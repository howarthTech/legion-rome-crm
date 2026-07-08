package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestDuesLifecycle(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ctx := t.Context()

	id, err := st.InsertMember(ctx, "Al Hollis", "", "+17065550101", "", "")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// New members start owing dues.
	m, _ := st.GetMember(ctx, id)
	if m.DuesStatus != DuesDue {
		t.Fatalf("new member dues status = %q, want DUE", m.DuesStatus)
	}

	// Record a payment → marks paid through the given year.
	amount := sql.NullInt64{Int64: 3000, Valid: true}
	pid, err := st.RecordDuesPayment(ctx, id, "2026-07-08", amount, "Check", "2026", "check #1043")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	m, _ = st.GetMember(ctx, id)
	if m.DuesStatus != DuesPaid || m.DuesPaidThrough != "2026" {
		t.Errorf("after payment: status=%q paidThrough=%q, want PAID/2026", m.DuesStatus, m.DuesPaidThrough)
	}

	pays, err := st.ListDuesPayments(ctx, id)
	if err != nil || len(pays) != 1 {
		t.Fatalf("list payments: %v (n=%d)", err, len(pays))
	}
	if pays[0].Method != "Check" || !pays[0].AmountCents.Valid || pays[0].AmountCents.Int64 != 3000 || pays[0].Notes != "check #1043" {
		t.Errorf("payment fields wrong: %+v", pays[0])
	}

	// Mark due again (new year) — history is preserved.
	if err := st.SetDuesStatus(ctx, id, DuesDue); err != nil {
		t.Fatalf("mark due: %v", err)
	}
	m, _ = st.GetMember(ctx, id)
	if m.DuesStatus != DuesDue {
		t.Errorf("status after mark-due = %q, want DUE", m.DuesStatus)
	}
	if pays, _ := st.ListDuesPayments(ctx, id); len(pays) != 1 {
		t.Errorf("mark-due should not delete history; got %d payments", len(pays))
	}

	// Delete the payment record.
	if err := st.DeleteDuesPayment(ctx, id, pid); err != nil {
		t.Fatalf("delete payment: %v", err)
	}
	if pays, _ := st.ListDuesPayments(ctx, id); len(pays) != 0 {
		t.Errorf("expected 0 payments after delete, got %d", len(pays))
	}

	// Unknown member.
	if _, err := st.RecordDuesPayment(ctx, 99999, "2026-07-08", sql.NullInt64{}, "Cash", "", ""); !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("record for unknown member: got %v, want ErrMemberNotFound", err)
	}
	if err := st.SetDuesStatus(ctx, 99999, DuesPaid); !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("set status unknown member: got %v, want ErrMemberNotFound", err)
	}

	// Optional amount: NULL round-trips as invalid.
	if _, err := st.RecordDuesPayment(ctx, id, "2026-07-09", sql.NullInt64{}, "Cash", "2026", ""); err != nil {
		t.Fatalf("record no-amount: %v", err)
	}
	pays, _ = st.ListDuesPayments(ctx, id)
	if len(pays) != 1 || pays[0].AmountCents.Valid {
		t.Errorf("expected 1 payment with NULL amount, got %+v", pays)
	}

	// Cascade: deleting the member removes their payments.
	if err := st.DeleteMember(ctx, id); err != nil {
		t.Fatalf("delete member: %v", err)
	}
	if pays, _ := st.ListDuesPayments(ctx, id); len(pays) != 0 {
		t.Errorf("payments should cascade-delete with member; got %d", len(pays))
	}
}
