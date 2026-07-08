package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Dues status values stored on members.dues_status.
const (
	DuesPaid = "PAID"
	DuesDue  = "DUE"
)

// DuesPayment is one recorded dues payment for a member.
type DuesPayment struct {
	ID             int64
	MemberID       int64
	PaidOn         string // YYYY-MM-DD
	AmountCents    sql.NullInt64
	Method         string
	MembershipYear string
	Notes          string
	CreatedAt      time.Time
}

// RecordDuesPayment logs a payment and marks the member paid up. If a
// membership year is given it becomes the member's "paid through" year. The
// insert and the member update happen in one transaction. Returns
// ErrMemberNotFound if the member id is unknown.
func (s *Store) RecordDuesPayment(ctx context.Context, memberID int64, paidOn string, amount sql.NullInt64, method, membershipYear, notes string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE members
		 SET dues_status = 'PAID',
		     dues_paid_through = CASE WHEN ? <> '' THEN ? ELSE dues_paid_through END,
		     updated_at = ?
		 WHERE id = ?`,
		membershipYear, membershipYear, now, memberID)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrMemberNotFound
	}

	ins, err := tx.ExecContext(ctx,
		`INSERT INTO dues_payments (member_id, paid_on, amount_cents, method, membership_year, notes)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		memberID, paidOn, amount, method, membershipYear, notes)
	if err != nil {
		return 0, fmt.Errorf("insert dues payment: %w", err)
	}
	id, _ := ins.LastInsertId()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// SetDuesStatus flips a member's dues status without recording a payment — used
// by "Mark dues due" (e.g. at the start of a new membership year). status must
// be DuesPaid or DuesDue. Returns ErrMemberNotFound if the id is unknown.
func (s *Store) SetDuesStatus(ctx context.Context, memberID int64, status string) error {
	if status != DuesPaid && status != DuesDue {
		return fmt.Errorf("invalid dues status %q", status)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE members SET dues_status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339), memberID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// ListDuesPayments returns a member's payments, most recent first.
func (s *Store) ListDuesPayments(ctx context.Context, memberID int64) ([]DuesPayment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, member_id, paid_on, amount_cents, method, membership_year, notes, created_at
		 FROM dues_payments WHERE member_id = ? ORDER BY paid_on DESC, id DESC`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DuesPayment
	for rows.Next() {
		var p DuesPayment
		var createdAt string
		if err := rows.Scan(&p.ID, &p.MemberID, &p.PaidOn, &p.AmountCents,
			&p.Method, &p.MembershipYear, &p.Notes, &createdAt); err != nil {
			return nil, err
		}
		p.CreatedAt = parseRFC3339(createdAt)
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteDuesPayment removes one payment record (to correct a mistake). Scoped to
// the member id so a stray id can't delete another member's record. Does not
// change the member's dues status — the admin adjusts that separately.
func (s *Store) DeleteDuesPayment(ctx context.Context, memberID, paymentID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM dues_payments WHERE id = ? AND member_id = ?`, paymentID, memberID)
	return err
}
