package app

import (
	"bytes"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// TestMemberViewRendersDues renders the member page with a fully-populated
// member and a dues payment, guarding the dues section's runtime template
// wiring — the custom funcs (centsUSD, duesMethods), the nested with/range, and
// the $.Member cross-references. A parse-only check wouldn't catch these.
func TestMemberViewRendersDues(t *testing.T) {
	tpl, err := loadTemplates(os.DirFS("../../cmd/server"))
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}
	m := &store.Member{
		ID: 7, Name: "Al Hollis", Phone: "+17065550101",
		OptInStatus: store.OptInConfirmed,
		DuesStatus:  store.DuesPaid, DuesPaidThrough: "2026",
	}
	pays := []store.DuesPayment{{
		ID: 1, MemberID: 7, PaidOn: "2026-07-08",
		AmountCents: sql.NullInt64{Int64: 3000, Valid: true},
		Method:      "Check", MembershipYear: "2026", Notes: "check #1043",
	}}
	data := map[string]any{
		"Title": "t", "OrgName": "Post 5", "Member": m,
		"DuesPayments": pays, "Today": "2026-07-08", "CurrentYear": "2026",
	}
	var buf bytes.Buffer
	if err := tpl["member_view"].ExecuteTemplate(&buf, "member_view", data); err != nil {
		t.Fatalf("execute member_view: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Paid up", "$30.00", "Check", "check #1043", "Record a payment", "Mark dues due"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered member page missing %q", want)
		}
	}
}
