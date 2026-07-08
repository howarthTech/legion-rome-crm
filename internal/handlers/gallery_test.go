package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// TestGalleryAPIUsesRequestHost is the regression guard for the DNS-cutover
// trap: the site may fetch the gallery feed on a different hostname than the
// CRM's configured PUBLIC_URL (e.g. the preview host before production DNS is
// live). Photo URLs in the feed must be same-origin as the request that Caddy
// forwarded, not PUBLIC_URL — otherwise Hugo fetches photos from an unresolved
// host and the site build fails.
func TestGalleryAPIUsesRequestHost(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	id, err := st.CreateAlbum(t.Context(), "Memorial Day", "2026-05-25", "desc")
	if err != nil {
		t.Fatalf("create album: %v", err)
	}
	if _, err := st.AddPhoto(t.Context(), id, "abc123.jpg", "image/jpeg"); err != nil {
		t.Fatalf("add photo: %v", err)
	}

	a := &app.App{Store: st, PublicURL: "https://admin.romelegion.org"}
	req := httptest.NewRequest("GET", "/api/gallery.json", nil)
	// Simulate Caddy reverse-proxying the preview host over loopback HTTP.
	req.Header.Set("X-Forwarded-Host", "legion-admin-preview.howarth.tech")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	GalleryAPI(a)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Albums []struct {
			Photos []struct {
				URL string `json:"url"`
			} `json:"photos"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Albums) != 1 || len(got.Albums[0].Photos) != 1 {
		t.Fatalf("expected 1 album with 1 photo, got %+v", got)
	}
	want := "https://legion-admin-preview.howarth.tech/media/abc123.jpg"
	if u := got.Albums[0].Photos[0].URL; u != want {
		t.Errorf("photo URL = %q, want %q (must follow the forwarded host, not PUBLIC_URL)", u, want)
	}
}

func TestPublicBaseFallsBackToPublicURL(t *testing.T) {
	a := &app.App{PublicURL: "https://admin.romelegion.org/"}
	req := httptest.NewRequest("GET", "/api/gallery.json", nil)
	req.Host = "" // no host, no forwarded headers → fall back to PublicURL (trimmed)
	if got := publicBase(a, req); got != "https://admin.romelegion.org" {
		t.Errorf("publicBase fallback = %q, want trimmed PublicURL", got)
	}
}
