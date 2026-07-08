package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

const maxPhotoBytes = 12 << 20 // 12 MB per photo

// allowedImage maps a detected content type to a file extension.
var allowedImage = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// GalleryAlbums lists albums with an add form.
func GalleryAlbums(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		albums, err := a.Store.ListAlbums(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Render(w, r, "gallery_albums", "Photo gallery", map[string]any{"Albums": albums})
	}
}

// GalleryAlbumCreate adds an album.
func GalleryAlbumCreate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(r.PostForm.Get("title"))
		if title == "" {
			redirect(w, r, "/content/gallery", "err", "Give the album a title.")
			return
		}
		id, err := a.Store.CreateAlbum(r.Context(), title,
			strings.TrimSpace(r.PostForm.Get("date")), strings.TrimSpace(r.PostForm.Get("description")))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		http.Redirect(w, r, fmt.Sprintf("/content/gallery/%d?ok=%s", id, "Album+created.+Now+add+photos."), http.StatusSeeOther)
	}
}

// GalleryAlbum shows one album: its details, photos, and an upload form.
func GalleryAlbum(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		album, err := albumFromPath(a, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		photos, _ := a.Store.ListPhotos(r.Context(), album.ID)
		a.Render(w, r, "gallery_album", album.Title, map[string]any{
			"Album":  album,
			"Photos": photos,
		})
	}
}

// GalleryAlbumUpdate saves album details.
func GalleryAlbumUpdate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		album, err := albumFromPath(a, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(r.PostForm.Get("title"))
		if title == "" {
			redirect(w, r, fmt.Sprintf("/content/gallery/%d", album.ID), "err", "Title is required.")
			return
		}
		_ = a.Store.UpdateAlbum(r.Context(), album.ID, title,
			strings.TrimSpace(r.PostForm.Get("date")), strings.TrimSpace(r.PostForm.Get("description")))
		a.Rebuild.Ping()
		redirect(w, r, fmt.Sprintf("/content/gallery/%d", album.ID), "ok", "Album saved."+rebuildNote(a))
	}
}

// GalleryAlbumDelete removes an album and its photo files.
func GalleryAlbumDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		album, err := albumFromPath(a, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		files, err := a.Store.DeleteAlbum(r.Context(), album.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, f := range files {
			_ = os.Remove(filepath.Join(a.MediaDir, filepath.Base(f)))
		}
		a.Rebuild.Ping()
		redirect(w, r, "/content/gallery", "ok", fmt.Sprintf("Album %q deleted.%s", album.Title, rebuildNote(a)))
	}
}

// GalleryUpload accepts one or more photos into an album.
func GalleryUpload(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		album, err := albumFromPath(a, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		back := fmt.Sprintf("/content/gallery/%d", album.ID)
		// Cap the whole request; several photos at once.
		r.Body = http.MaxBytesReader(w, r.Body, 8*maxPhotoBytes)
		if err := r.ParseMultipartForm(maxPhotoBytes); err != nil {
			redirect(w, r, back, "err", "That upload was too large. Add fewer or smaller photos at a time.")
			return
		}
		files := r.MultipartForm.File["photos"]
		if len(files) == 0 {
			redirect(w, r, back, "err", "Choose at least one photo.")
			return
		}
		if err := os.MkdirAll(a.MediaDir, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		saved, skipped := 0, 0
		for _, fh := range files {
			if fh.Size > maxPhotoBytes {
				skipped++
				continue
			}
			if err := saveOnePhoto(r, a, album.ID, fh); err != nil {
				skipped++
				continue
			}
			saved++
		}
		a.Rebuild.Ping()
		msg := fmt.Sprintf("%d photo%s added.", saved, plural(saved))
		if skipped > 0 {
			msg += fmt.Sprintf(" %d skipped (not an image, or over 12 MB).", skipped)
		}
		redirect(w, r, back, "ok", msg+rebuildNote(a))
	}
}

// saveOnePhoto validates the file is an image (by content sniff), writes it to
// MediaDir under a random name, and records the row.
func saveOnePhoto(r *http.Request, a *app.App, albumID int64, fh *multipart.FileHeader) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// Sniff the real content type from the first bytes (don't trust the name).
	head := make([]byte, 512)
	n, _ := io.ReadFull(src, head)
	ct := http.DetectContentType(head[:n])
	ext, ok := allowedImage[ct]
	if !ok {
		return fmt.Errorf("not an image: %s", ct)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}

	name := randHex(16) + ext
	dst, err := os.OpenFile(filepath.Join(a.MediaDir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, io.LimitReader(src, maxPhotoBytes)); err != nil {
		dst.Close()
		_ = os.Remove(dst.Name())
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	if _, err := a.Store.AddPhoto(r.Context(), albumID, name, ct); err != nil {
		_ = os.Remove(filepath.Join(a.MediaDir, name))
		return err
	}
	return nil
}

// GalleryPhotoCaption sets a photo caption.
func GalleryPhotoCaption(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, albumID, ok := photoAndAlbum(a, r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		_ = a.Store.SetPhotoCaption(r.Context(), id, strings.TrimSpace(r.PostForm.Get("caption")))
		a.Rebuild.Ping()
		http.Redirect(w, r, fmt.Sprintf("/content/gallery/%d", albumID), http.StatusSeeOther)
	}
}

// GalleryPhotoDelete removes a photo and its file.
func GalleryPhotoDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, albumID, ok := photoAndAlbum(a, r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if fn, err := a.Store.DeletePhoto(r.Context(), id); err == nil && fn != "" {
			_ = os.Remove(filepath.Join(a.MediaDir, filepath.Base(fn)))
		}
		a.Rebuild.Ping()
		http.Redirect(w, r, fmt.Sprintf("/content/gallery/%d", albumID), http.StatusSeeOther)
	}
}

// GalleryPhotoMove reorders a photo within its album.
func GalleryPhotoMove(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, albumID, ok := photoAndAlbum(a, r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		delta := 1
		if r.URL.Query().Get("dir") == "up" {
			delta = -1
		}
		_ = a.Store.MovePhoto(r.Context(), id, delta)
		a.Rebuild.Ping()
		http.Redirect(w, r, fmt.Sprintf("/content/gallery/%d", albumID), http.StatusSeeOther)
	}
}

// MediaServe serves an uploaded photo. Public (photos are public on the site);
// the filename is base-sanitized to prevent path traversal.
func MediaServe(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.PathValue("name"))
		if name == "" || name == "." || strings.ContainsAny(name, `/\`) {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(a.MediaDir, name)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, full)
	}
}

// GalleryAPI is the public gallery feed the website builds from.
func GalleryAPI(a *app.App) http.HandlerFunc {
	type apiPhoto struct {
		URL     string `json:"url"`
		Caption string `json:"caption,omitempty"`
	}
	type apiAlbum struct {
		Slug        string     `json:"slug"`
		Title       string     `json:"title"`
		Date        string     `json:"date,omitempty"`
		Description string     `json:"description,omitempty"`
		Photos      []apiPhoto `json:"photos"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		albums, byAlbum, err := a.Store.AllAlbumsWithPhotos(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		base := strings.TrimRight(a.PublicURL, "/")
		out := struct {
			Generated string     `json:"generated"`
			Albums    []apiAlbum `json:"albums"`
		}{Generated: time.Now().UTC().Format(time.RFC3339), Albums: make([]apiAlbum, 0, len(albums))}
		for _, al := range albums {
			photos := make([]apiPhoto, 0)
			for _, p := range byAlbum[al.ID] {
				photos = append(photos, apiPhoto{URL: base + "/media/" + p.Filename, Caption: p.Caption})
			}
			out.Albums = append(out.Albums, apiAlbum{
				Slug: al.Slug, Title: al.Title, Date: al.Date, Description: al.Description, Photos: photos,
			})
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// --- helpers ---------------------------------------------------------------

func albumFromPath(a *app.App, r *http.Request) (*store.Album, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, err
	}
	return a.Store.GetAlbum(r.Context(), id)
}

func photoAndAlbum(a *app.App, r *http.Request) (photoID, albumID int64, ok bool) {
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	p, err := a.Store.GetPhoto(r.Context(), pid)
	if err != nil {
		return 0, 0, false
	}
	return p.ID, p.AlbumID, true
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
