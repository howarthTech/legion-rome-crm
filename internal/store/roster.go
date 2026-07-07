package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Roster group types.
const (
	RosterOfficer = "officer"
	RosterFamily  = "family"
)

// RosterMember is one officer or family-program contact.
type RosterMember struct {
	ID    int64
	Group string // RosterOfficer or RosterFamily
	Role  string
	Name  string
	Phone string
	Email string
	Sort  int
}

// ListRoster returns members of one group, in sort order.
func (s *Store) ListRoster(ctx context.Context, group string) ([]RosterMember, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, group_type, role, name, phone, email, sort_order FROM roster WHERE group_type = ? ORDER BY sort_order, id",
		group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RosterMember
	for rows.Next() {
		var m RosterMember
		if err := rows.Scan(&m.ID, &m.Group, &m.Role, &m.Name, &m.Phone, &m.Email, &m.Sort); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetRosterMember by id.
func (s *Store) GetRosterMember(ctx context.Context, id int64) (*RosterMember, error) {
	var m RosterMember
	err := s.db.QueryRowContext(ctx,
		"SELECT id, group_type, role, name, phone, email, sort_order FROM roster WHERE id = ?", id).
		Scan(&m.ID, &m.Group, &m.Role, &m.Name, &m.Phone, &m.Email, &m.Sort)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("roster member %d not found", id)
	}
	return &m, err
}

// CreateRosterMember appends a member to the end of its group.
func (s *Store) CreateRosterMember(ctx context.Context, m RosterMember) (int64, error) {
	var maxSort sql.NullInt64
	_ = s.db.QueryRowContext(ctx,
		"SELECT MAX(sort_order) FROM roster WHERE group_type = ?", m.Group).Scan(&maxSort)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO roster (group_type, role, name, phone, email, sort_order) VALUES (?, ?, ?, ?, ?, ?)",
		m.Group, m.Role, m.Name, m.Phone, m.Email, int(maxSort.Int64)+10)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateRosterMember rewrites a member's fields (not the group).
func (s *Store) UpdateRosterMember(ctx context.Context, id int64, m RosterMember) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE roster SET role=?, name=?, phone=?, email=? WHERE id=?",
		m.Role, m.Name, m.Phone, m.Email, id)
	return err
}

// DeleteRosterMember removes a member.
func (s *Store) DeleteRosterMember(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM roster WHERE id=?", id)
	return err
}

// MoveRosterMember shifts a member up or down within its group by swapping
// sort_order with its neighbor. delta is -1 (up) or +1 (down).
func (s *Store) MoveRosterMember(ctx context.Context, id int64, delta int) error {
	m, err := s.GetRosterMember(ctx, id)
	if err != nil {
		return err
	}
	members, err := s.ListRoster(ctx, m.Group)
	if err != nil {
		return err
	}
	idx := -1
	for i, x := range members {
		if x.ID == id {
			idx = i
			break
		}
	}
	swap := idx + delta
	if idx < 0 || swap < 0 || swap >= len(members) {
		return nil // already at an end; no-op
	}
	a, b := members[idx], members[swap]
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE roster SET sort_order=? WHERE id=?", b.Sort, a.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE roster SET sort_order=? WHERE id=?", a.Sort, b.ID); err != nil {
		return err
	}
	return tx.Commit()
}
