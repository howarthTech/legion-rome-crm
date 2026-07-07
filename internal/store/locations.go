package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Location is a reusable venue. Events render it as "Name — Address" (or just
// the address when the name adds nothing), stored as plain text on the event,
// so locations can be edited or deleted without touching past events.
type Location struct {
	ID      int64
	Name    string
	Address string
}

// Display is the string an event stores and the website shows.
func (l Location) Display() string {
	name := strings.TrimSpace(l.Name)
	addr := strings.TrimSpace(l.Address)
	if name == "" || strings.EqualFold(name, addr) {
		return addr
	}
	if addr == "" {
		return name
	}
	return name + " — " + addr
}

// ListLocations returns every known location, alphabetical by name.
func (s *Store) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, address FROM locations ORDER BY name COLLATE NOCASE ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Location
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.Name, &l.Address); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetLocation fetches one location by id.
func (s *Store) GetLocation(ctx context.Context, id int64) (*Location, error) {
	var l Location
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, address FROM locations WHERE id = ?", id).
		Scan(&l.ID, &l.Name, &l.Address)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("location %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// CreateLocation inserts a location. Names are unique; a duplicate returns a
// caller-friendly error.
func (s *Store) CreateLocation(ctx context.Context, name, address string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO locations (name, address) VALUES (?, ?)",
		strings.TrimSpace(name), strings.TrimSpace(address))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, fmt.Errorf("a location named %q already exists", name)
		}
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteLocation removes a location. Existing events keep their stored
// location text.
func (s *Store) DeleteLocation(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM locations WHERE id = ?", id)
	return err
}
