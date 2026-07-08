package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestUpdateMember(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ctx := t.Context()

	id, err := st.InsertMember(ctx, "Al Hollis", "Commander", "+17065550101", "al@example.com", "founder")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Confirm opt-in so we can assert the edit doesn't disturb consent state.
	if err := st.SetOptedIn(ctx, id); err != nil {
		t.Fatalf("opt in: %v", err)
	}

	// Edit every field.
	if err := st.UpdateMember(ctx, id, "Albert Hollis", "Adjutant", "+17065550102", "albert@example.com", "founder, moved"); err != nil {
		t.Fatalf("update: %v", err)
	}
	m, err := st.GetMember(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.Name != "Albert Hollis" || m.Title != "Adjutant" || m.Phone != "+17065550102" ||
		m.Email != "albert@example.com" || m.Notes != "founder, moved" {
		t.Errorf("fields not updated: %+v", m)
	}
	if m.OptInStatus != OptInConfirmed {
		t.Errorf("opt-in status changed by an edit: got %q, want OPTED_IN", m.OptInStatus)
	}

	// A second member; updating member 1 onto its phone must collide.
	other, err := st.InsertMember(ctx, "Jane Adams", "", "+17065550200", "", "")
	if err != nil {
		t.Fatalf("insert other: %v", err)
	}
	if err := st.UpdateMember(ctx, id, "Albert Hollis", "Adjutant", "+17065550200", "", ""); !errors.Is(err, ErrPhoneExists) {
		t.Errorf("expected ErrPhoneExists on phone collision, got %v", err)
	}
	_ = other

	// Unknown id.
	if err := st.UpdateMember(ctx, 99999, "Nobody", "", "+17065559999", "", ""); !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}
