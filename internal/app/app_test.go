package app

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestPagesRenderTheirOwnBody guards against the shared-"body" template
// collision: when every page template was parsed into one set, each page's
// {{define "body"}} overwrote the previous one, and every page in the app —
// including /login — rendered the LAST parsed page (reminders). This test
// renders each page and asserts a marker unique to that page is present,
// and that the reminders marker only appears on the reminders page.
func TestPagesRenderTheirOwnBody(t *testing.T) {
	// Real templates live in the server package; loadTemplates expects
	// paths rooted at web/templates/.
	tpl, err := loadTemplates(os.DirFS("../../cmd/server"))
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	markers := map[string]string{
		"login":     `name="password"`,
		"reminders": "Send a reminder",
	}
	const remindersMarker = "Send a reminder"

	for page, marker := range markers {
		set, ok := tpl[page]
		if !ok {
			t.Fatalf("no template set for page %q", page)
		}
		var buf bytes.Buffer
		if err := set.ExecuteTemplate(&buf, page, map[string]any{"Title": "t"}); err != nil {
			t.Fatalf("execute %q: %v", page, err)
		}
		out := buf.String()
		if !strings.Contains(out, marker) {
			t.Errorf("page %q: expected marker %q in output", page, marker)
		}
		if page != "reminders" && strings.Contains(out, remindersMarker) {
			t.Errorf("page %q rendered the reminders body — template sets are colliding again", page)
		}
	}

	// Every page the app renders must have a set.
	for _, page := range pageNames {
		if _, ok := tpl[page]; !ok {
			t.Errorf("pageNames lists %q but loadTemplates produced no set for it", page)
		}
	}
}
