package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Album is a photo gallery album.
type Album struct {
	ID          int64
	Slug        string
	Title       string
	Date        string
	Description string
	Sort        int
	PhotoCount  int
}

// Photo is one image within an album.
type Photo struct {
	ID          int64
	AlbumID     int64
	Filename    string
	Caption     string
	ContentType string
	Sort        int
}

// ListAlbums returns albums with their photo counts, newest album date first.
func (s *Store) ListAlbums(ctx context.Context) ([]Album, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.slug, a.title, a.album_date, a.description, a.sort_order,
		       (SELECT COUNT(*) FROM gallery_photos p WHERE p.album_id = a.id)
		FROM gallery_albums a
		ORDER BY a.album_date DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Album
	for rows.Next() {
		var a Album
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Date, &a.Description, &a.Sort, &a.PhotoCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAlbum by id.
func (s *Store) GetAlbum(ctx context.Context, id int64) (*Album, error) {
	var a Album
	err := s.db.QueryRowContext(ctx,
		"SELECT id, slug, title, album_date, description, sort_order FROM gallery_albums WHERE id = ?", id).
		Scan(&a.ID, &a.Slug, &a.Title, &a.Date, &a.Description, &a.Sort)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("album %d not found", id)
	}
	return &a, err
}

// CreateAlbum inserts an album with a slug derived from the title.
func (s *Store) CreateAlbum(ctx context.Context, title, date, description string) (int64, error) {
	slug, err := s.uniqueSlugInTable(ctx, "gallery_albums", slugify(title))
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO gallery_albums (slug, title, album_date, description) VALUES (?, ?, ?, ?)",
		slug, title, date, description)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAlbum rewrites an album's editable fields (slug is immutable).
func (s *Store) UpdateAlbum(ctx context.Context, id int64, title, date, description string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE gallery_albums SET title=?, album_date=?, description=? WHERE id=?",
		title, date, description, id)
	return err
}

// DeleteAlbum removes an album and returns the filenames of its photos so the
// caller can delete the files from disk (the DB cascade removes photo rows).
func (s *Store) DeleteAlbum(ctx context.Context, id int64) ([]string, error) {
	files, err := s.albumFilenames(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM gallery_albums WHERE id=?", id); err != nil {
		return nil, err
	}
	return files, nil
}

func (s *Store) albumFilenames(ctx context.Context, albumID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT filename FROM gallery_photos WHERE album_id=?", albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// ListPhotos returns an album's photos in order.
func (s *Store) ListPhotos(ctx context.Context, albumID int64) ([]Photo, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, album_id, filename, caption, content_type, sort_order FROM gallery_photos WHERE album_id=? ORDER BY sort_order, id",
		albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Photo
	for rows.Next() {
		var p Photo
		if err := rows.Scan(&p.ID, &p.AlbumID, &p.Filename, &p.Caption, &p.ContentType, &p.Sort); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddPhoto records an uploaded photo (appended to the end of its album).
func (s *Store) AddPhoto(ctx context.Context, albumID int64, filename, contentType string) (int64, error) {
	var maxSort sql.NullInt64
	_ = s.db.QueryRowContext(ctx, "SELECT MAX(sort_order) FROM gallery_photos WHERE album_id=?", albumID).Scan(&maxSort)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO gallery_photos (album_id, filename, content_type, sort_order) VALUES (?, ?, ?, ?)",
		albumID, filename, contentType, int(maxSort.Int64)+10)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetPhoto by id.
func (s *Store) GetPhoto(ctx context.Context, id int64) (*Photo, error) {
	var p Photo
	err := s.db.QueryRowContext(ctx,
		"SELECT id, album_id, filename, caption, content_type, sort_order FROM gallery_photos WHERE id=?", id).
		Scan(&p.ID, &p.AlbumID, &p.Filename, &p.Caption, &p.ContentType, &p.Sort)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("photo %d not found", id)
	}
	return &p, err
}

// SetPhotoCaption updates a photo's caption.
func (s *Store) SetPhotoCaption(ctx context.Context, id int64, caption string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE gallery_photos SET caption=? WHERE id=?", caption, id)
	return err
}

// DeletePhoto removes a photo row and returns its filename to delete from disk.
func (s *Store) DeletePhoto(ctx context.Context, id int64) (string, error) {
	p, err := s.GetPhoto(ctx, id)
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM gallery_photos WHERE id=?", id); err != nil {
		return "", err
	}
	return p.Filename, nil
}

// MovePhoto swaps a photo with its neighbor within the album (delta -1/+1).
func (s *Store) MovePhoto(ctx context.Context, id int64, delta int) error {
	p, err := s.GetPhoto(ctx, id)
	if err != nil {
		return err
	}
	photos, err := s.ListPhotos(ctx, p.AlbumID)
	if err != nil {
		return err
	}
	idx := -1
	for i, x := range photos {
		if x.ID == id {
			idx = i
			break
		}
	}
	swap := idx + delta
	if idx < 0 || swap < 0 || swap >= len(photos) {
		return nil
	}
	a, b := photos[idx], photos[swap]
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE gallery_photos SET sort_order=? WHERE id=?", b.Sort, a.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE gallery_photos SET sort_order=? WHERE id=?", a.Sort, b.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// AllAlbumsWithPhotos returns every album with its photos, for the public API.
func (s *Store) AllAlbumsWithPhotos(ctx context.Context) ([]Album, map[int64][]Photo, error) {
	albums, err := s.ListAlbums(ctx)
	if err != nil {
		return nil, nil, err
	}
	byAlbum := map[int64][]Photo{}
	for _, a := range albums {
		ph, err := s.ListPhotos(ctx, a.ID)
		if err != nil {
			return nil, nil, err
		}
		byAlbum[a.ID] = ph
	}
	return albums, byAlbum, nil
}

// uniqueSlugInTable de-duplicates a slug against the given table's slug column.
func (s *Store) uniqueSlugInTable(ctx context.Context, table, base string) (string, error) {
	if base == "" {
		base = "album"
	}
	slug := base
	for i := 2; ; i++ {
		var n int
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+table+" WHERE slug = ?", slug).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}
