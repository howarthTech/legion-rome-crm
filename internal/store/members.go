package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OptInStatus mirrors the CHECK constraint in the members table.
type OptInStatus string

const (
	OptInPending  OptInStatus = "PENDING"
	OptInConfirmed OptInStatus = "OPTED_IN"
	OptInOptedOut OptInStatus = "OPTED_OUT"
)

// Member is the in-memory representation of a row in the members table.
type Member struct {
	ID                int64
	Name              string
	Phone             string // E.164 (+17065551234)
	Email             string
	OptInStatus       OptInStatus
	OptInRequestedAt  time.Time
	OptInConfirmedAt  sql.NullTime
	OptOutAt          sql.NullTime
	Notes             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ErrPhoneExists is returned by InsertMember when the phone column's UNIQUE
// constraint fires. Callers can use errors.Is to detect.
var ErrPhoneExists = errors.New("a member with that phone number already exists")

// ErrMemberNotFound is returned when a Get/Update operation has no target.
var ErrMemberNotFound = errors.New("member not found")

// InsertMember creates a row with status=PENDING and returns the assigned ID.
// Phone must already be in E.164 form (caller normalizes).
func (s *Store) InsertMember(ctx context.Context, name, phone, email, notes string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO members (name, phone, email, opt_in_status, opt_in_requested_at, notes, created_at, updated_at)
	            VALUES (?, ?, NULLIF(?, ''), 'PENDING', ?, NULLIF(?, ''), ?, ?)`
	r, err := s.db.ExecContext(ctx, q, name, phone, email, now, notes, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, ErrPhoneExists
		}
		return 0, fmt.Errorf("insert member: %w", err)
	}
	return r.LastInsertId()
}

// GetMember by ID. Returns ErrMemberNotFound if no row.
func (s *Store) GetMember(ctx context.Context, id int64) (*Member, error) {
	return s.scanMember(s.db.QueryRowContext(ctx, memberSelectByID, id))
}

// GetMemberByPhone for the webhook handler: incoming SMS gives us only a phone.
func (s *Store) GetMemberByPhone(ctx context.Context, phone string) (*Member, error) {
	return s.scanMember(s.db.QueryRowContext(ctx, memberSelectByPhone, phone))
}

// ListMembers returns all members, most recent first.
func (s *Store) ListMembers(ctx context.Context) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, memberSelectAll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		m, err := s.scanMemberRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// ListOptedIn — members eligible to receive event reminders.
// This is the only query the reminder-sender should use; anything else risks
// sending to PENDING or OPTED_OUT members and violating TCPA.
func (s *Store) ListOptedIn(ctx context.Context) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, memberSelectOptedIn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		m, err := s.scanMemberRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// SetOptedIn flips the status to OPTED_IN and stamps confirmed_at. Idempotent.
func (s *Store) SetOptedIn(ctx context.Context, memberID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE members SET opt_in_status = 'OPTED_IN', opt_in_confirmed_at = ?, updated_at = ? WHERE id = ?`,
		now, now, memberID)
	return err
}

// SetOptedOut flips the status to OPTED_OUT and stamps opt_out_at. Idempotent.
func (s *Store) SetOptedOut(ctx context.Context, memberID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE members SET opt_in_status = 'OPTED_OUT', opt_out_at = ?, updated_at = ? WHERE id = ?`,
		now, now, memberID)
	return err
}

// DeleteMember removes the row entirely (and cascades to NULL on messages_log).
// Note: deleting a member doesn't recall messages already sent — it just means
// they no longer appear in admin lists. Prefer SetOptedOut for "stop messaging
// this person" semantics.
func (s *Store) DeleteMember(ctx context.Context, id int64) error {
	r, err := s.db.ExecContext(ctx, `DELETE FROM members WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// --- query strings & helpers ----------------------------------------------

const memberColumns = `id, name, phone, COALESCE(email, ''), opt_in_status,
		opt_in_requested_at, opt_in_confirmed_at, opt_out_at,
		COALESCE(notes, ''), created_at, updated_at`

var (
	memberSelectByID    = `SELECT ` + memberColumns + ` FROM members WHERE id = ?`
	memberSelectByPhone = `SELECT ` + memberColumns + ` FROM members WHERE phone = ?`
	memberSelectAll     = `SELECT ` + memberColumns + ` FROM members ORDER BY created_at DESC`
	memberSelectOptedIn = `SELECT ` + memberColumns + ` FROM members WHERE opt_in_status = 'OPTED_IN' ORDER BY name`
)

// scanner abstracts *sql.Row and *sql.Rows so we can reuse scan code.
type scanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanMember(row *sql.Row) (*Member, error) {
	m, err := scanMemberFrom(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMemberNotFound
	}
	return m, err
}

func (s *Store) scanMemberRow(rows *sql.Rows) (*Member, error) {
	return scanMemberFrom(rows)
}

func scanMemberFrom(src scanner) (*Member, error) {
	var (
		m           Member
		reqAt       string
		confirmedAt sql.NullString
		optOutAt    sql.NullString
		createdAt   string
		updatedAt   string
	)
	if err := src.Scan(
		&m.ID, &m.Name, &m.Phone, &m.Email, &m.OptInStatus,
		&reqAt, &confirmedAt, &optOutAt,
		&m.Notes, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	m.OptInRequestedAt = parseRFC3339(reqAt)
	m.OptInConfirmedAt = parseNullableRFC3339(confirmedAt)
	m.OptOutAt = parseNullableRFC3339(optOutAt)
	m.CreatedAt = parseRFC3339(createdAt)
	m.UpdatedAt = parseRFC3339(updatedAt)
	return &m, nil
}

func parseRFC3339(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// SQLite's CURRENT_TIMESTAMP uses "2006-01-02 15:04:05" — fall back.
		t, _ = time.Parse("2006-01-02 15:04:05", s)
	}
	return t.UTC()
}

func parseNullableRFC3339(n sql.NullString) sql.NullTime {
	if !n.Valid {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: parseRFC3339(n.String), Valid: true}
}
