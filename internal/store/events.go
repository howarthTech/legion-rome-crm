package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The two event kinds. Post events are the post's own (meetings, parties)
// and are eligible for SMS reminders; community events are public events the
// post takes part in — website-only, never SMS-reminded.
const (
	EventTypePost      = "post"
	EventTypeCommunity = "community"
)

// Event is an event authored in the CRM. The website's event pages and the
// reminder screen both read from this table — single source of truth.
type Event struct {
	ID           int64
	Slug         string
	Title        string
	Type         string    // EventTypePost or EventTypeCommunity
	StartsAt     time.Time // parsed from the stored RFC3339 string
	StartsAtRaw  string    // RFC3339 as entered (post's local offset preserved)
	EndsAtRaw    string    // RFC3339 or ""
	Location     string
	ContactName  string
	ContactPhone string
	Description  string
	Body         string
}

// IsCommunity reports whether this is a community (non-SMS) event.
func (e Event) IsCommunity() bool { return e.Type == EventTypeCommunity }

// IsPast reports whether the event started before now.
func (e Event) IsPast(now time.Time) bool { return e.StartsAt.Before(now) }

const eventCols = "id, slug, title, event_type, starts_at, starts_at_unix, ends_at, location, contact_name, contact_phone, description, body"

func scanEvent(row interface{ Scan(...any) error }) (Event, error) {
	var e Event
	var unix int64
	if err := row.Scan(&e.ID, &e.Slug, &e.Title, &e.Type, &e.StartsAtRaw, &unix, &e.EndsAtRaw,
		&e.Location, &e.ContactName, &e.ContactPhone, &e.Description, &e.Body); err != nil {
		return Event{}, err
	}
	if t, err := time.Parse(time.RFC3339, e.StartsAtRaw); err == nil {
		e.StartsAt = t
	} else {
		e.StartsAt = time.Unix(unix, 0)
	}
	return e, nil
}

// ListEvents returns every event, soonest-starting first.
func (s *Store) ListEvents(ctx context.Context) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+eventCols+" FROM events ORDER BY starts_at_unix ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpcomingEvents returns events starting after now, soonest first. typeFilter
// narrows to one event kind ("" = all kinds). The reminder screen passes
// EventTypePost — community events are never SMS-reminded.
func (s *Store) UpcomingEvents(ctx context.Context, now time.Time, typeFilter string) ([]Event, error) {
	q := "SELECT " + eventCols + " FROM events WHERE starts_at_unix > ?"
	args := []any{now.Unix()}
	if typeFilter != "" {
		q += " AND event_type = ?"
		args = append(args, typeFilter)
	}
	rows, err := s.db.QueryContext(ctx, q+" ORDER BY starts_at_unix ASC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEvent fetches one event by id.
func (s *Store) GetEvent(ctx context.Context, id int64) (*Event, error) {
	e, err := scanEvent(s.db.QueryRowContext(ctx,
		"SELECT "+eventCols+" FROM events WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("event %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetEventBySlug fetches one event by its site slug.
func (s *Store) GetEventBySlug(ctx context.Context, slug string) (*Event, error) {
	e, err := scanEvent(s.db.QueryRowContext(ctx,
		"SELECT "+eventCols+" FROM events WHERE slug = ?", slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("event %q not found", slug)
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateEvent inserts a new event. The slug is derived from the title and
// start month (matching the site's historical convention, e.g.
// "post-5-monthly-meeting-2026-07") and de-duplicated with a numeric suffix.
// The caller provides startsAt already in the post's timezone.
func (s *Store) CreateEvent(ctx context.Context, e Event) (int64, error) {
	slug, err := s.uniqueSlug(ctx, slugify(e.Title)+"-"+e.StartsAt.Format("2006-01"))
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO events (slug, title, event_type, starts_at, starts_at_unix, ends_at, location,
		                    contact_name, contact_phone, description, body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, e.Title, eventTypeOrDefault(e.Type), e.StartsAt.Format(time.RFC3339), e.StartsAt.Unix(), e.EndsAtRaw,
		e.Location, e.ContactName, e.ContactPhone, e.Description, e.Body)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func eventTypeOrDefault(t string) string {
	if t == EventTypeCommunity {
		return EventTypeCommunity
	}
	return EventTypePost
}

// SeedEvent inserts an event with a caller-chosen slug, used for migrating
// pre-existing site events into the CRM without changing their URLs. No-op
// if the slug already exists.
func (s *Store) SeedEvent(ctx context.Context, slug string, e Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (slug, title, event_type, starts_at, starts_at_unix, ends_at, location,
		                    contact_name, contact_phone, description, body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO NOTHING`,
		slug, e.Title, eventTypeOrDefault(e.Type), e.StartsAt.Format(time.RFC3339), e.StartsAt.Unix(), e.EndsAtRaw,
		e.Location, e.ContactName, e.ContactPhone, e.Description, e.Body)
	return err
}

// UpdateEvent rewrites an event's editable fields. The slug is intentionally
// immutable so published URLs and calendar entries never break.
func (s *Store) UpdateEvent(ctx context.Context, id int64, e Event) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE events SET title=?, event_type=?, starts_at=?, starts_at_unix=?, ends_at=?, location=?,
		       contact_name=?, contact_phone=?, description=?, body=?,
		       updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=?`,
		e.Title, eventTypeOrDefault(e.Type), e.StartsAt.Format(time.RFC3339), e.StartsAt.Unix(), e.EndsAtRaw,
		e.Location, e.ContactName, e.ContactPhone, e.Description, e.Body, id)
	return err
}

// DeleteEvent removes an event.
func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id=?", id)
	return err
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func (s *Store) uniqueSlug(ctx context.Context, base string) (string, error) {
	slug := base
	for i := 2; ; i++ {
		var n int
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM events WHERE slug = ?", slug).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}
