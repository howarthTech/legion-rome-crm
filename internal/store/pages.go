package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PageDefs is the standard set of editable prose pages a post site has. The
// editor lists these; a row is created lazily on first save. Order is display
// order in the admin.
var PageDefs = []struct {
	Slug  string
	Title string
	Help  string
}{
	{"about", "About / welcome", "The introduction on your About page."},
	{"history", "Post history", "Who your post is named for, when it was founded, milestones."},
	{"membership", "Membership", "Eligibility and how to join (beyond the standard Legion text)."},
	{"rental", "Hall rental", "Pricing, capacity, amenities, and who to contact."},
	{"family", "Legion family", "What your Auxiliary, SAL, and Riders do locally."},
	{"gallery", "Gallery intro", "A short intro shown above the photo gallery."},
}

// Page is an editable prose page.
type Page struct {
	Slug  string
	Title string
	Body  string
}

// GetPage returns a stored page, or (nil, nil) if it hasn't been written yet.
func (s *Store) GetPage(ctx context.Context, slug string) (*Page, error) {
	var p Page
	err := s.db.QueryRowContext(ctx,
		"SELECT slug, title, body FROM pages WHERE slug = ?", slug).
		Scan(&p.Slug, &p.Title, &p.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPages returns every stored page keyed by slug.
func (s *Store) ListPages(ctx context.Context) (map[string]Page, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT slug, title, body FROM pages")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Page{}
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.Slug, &p.Title, &p.Body); err != nil {
			return nil, err
		}
		out[p.Slug] = p
	}
	return out, rows.Err()
}

// SavePage upserts a page's title + body.
func (s *Store) SavePage(ctx context.Context, slug, title, body string) error {
	if slug == "" {
		return fmt.Errorf("page slug required")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pages (slug, title, body) VALUES (?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET title = excluded.title, body = excluded.body,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`, slug, title, body)
	return err
}
