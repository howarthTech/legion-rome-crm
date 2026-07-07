// Package app wires the dependencies (store, sms client, auth manager,
// templates) and exposes the HTTP handler. Handlers live in
// internal/handlers/ as methods on *App.
package app

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/auth"
	"github.com/howarthTech/legion-rome-crm/internal/events"
	"github.com/howarthTech/legion-rome-crm/internal/geocode"
	"github.com/howarthTech/legion-rome-crm/internal/rebuild"
	"github.com/howarthTech/legion-rome-crm/internal/sms"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// App holds everything handlers need.
type App struct {
	Store      *store.Store
	Twilio     *sms.Client
	Auth       *auth.Manager
	Quiet      *events.QuietHours
	Rebuild    *rebuild.Notifier
	Geocode    *geocode.Checker
	Templates  map[string]*template.Template // one set per page: layout + that page
	StaticFS   http.Handler
	PublicURL  string // canonical public URL (used for Twilio webhook signature verification)
	OrgName    string // post name — used in SMS bodies and page chrome
}

// Deps bundles the App's dependencies so New doesn't grow an unwieldy
// positional signature as the app gains features.
type Deps struct {
	Store     *store.Store
	Twilio    *sms.Client
	Auth      *auth.Manager
	Quiet     *events.QuietHours
	Rebuild   *rebuild.Notifier
	Geocode   *geocode.Checker
	TplFS     fs.FS
	StaticFS  embed.FS
	PublicURL string
	OrgName   string
}

// New builds an App. Caller is responsible for closing d.Store.
func New(d Deps) (*App, error) {
	tpl, err := loadTemplates(d.TplFS)
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	staticSub, err := fs.Sub(d.StaticFS, "web/static")
	if err != nil {
		return nil, err
	}
	return &App{
		Store:     d.Store,
		Twilio:    d.Twilio,
		Auth:      d.Auth,
		Quiet:     d.Quiet,
		Rebuild:   d.Rebuild,
		Geocode:   d.Geocode,
		Templates: tpl,
		StaticFS:  http.FileServer(http.FS(staticSub)),
		PublicURL: strings.TrimRight(d.PublicURL, "/"),
		OrgName:   d.OrgName,
	}, nil
}

// pageNames are the page templates under web/templates/. Each page is parsed
// into its OWN template set together with the layout. Parsing them all into
// one shared set is a bug: every page defines a block named "body", and in a
// single set the last file parsed silently overwrites all the others — every
// page then renders the final file's body (this shipped once; see
// TestPagesRenderTheirOwnBody).
var pageNames = []string{
	"login", "dashboard", "members_list", "members_new", "member_view", "reminders",
	"events_list", "events_form", "locations_list",
}

func loadTemplates(tplFS fs.FS) (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Local().Format("Jan 2, 2006 · 3:04 PM")
		},
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Local().Format("Jan 2, 2006")
		},
		"badgeClass": func(status store.OptInStatus) string {
			switch status {
			case store.OptInConfirmed:
				return "badge badge-ok"
			case store.OptInPending:
				return "badge badge-pending"
			case store.OptInOptedOut:
				return "badge badge-out"
			default:
				return "badge"
			}
		},
		"statusLabel": func(status store.OptInStatus) string {
			switch status {
			case store.OptInConfirmed:
				return "Opted in"
			case store.OptInPending:
				return "Awaiting consent"
			case store.OptInOptedOut:
				return "Opted out"
			default:
				return string(status)
			}
		},
	}
	sets := make(map[string]*template.Template, len(pageNames))
	for _, page := range pageNames {
		t, err := template.New(page).Funcs(funcs).ParseFS(tplFS,
			"web/templates/layout.html",
			"web/templates/"+page+".html",
		)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", page, err)
		}
		sets[page] = t
	}
	return sets, nil
}

// Render writes a template wrapped in the base layout. `data` is merged with
// the standard fields ({Title, User, FlashError, FlashOK, OrgName}) so every
// template can rely on them.
func (a *App) Render(w http.ResponseWriter, r *http.Request, name, title string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["Title"] = title
	data["OrgName"] = a.OrgName
	if u, err := a.Auth.ParseSession(r); err == nil {
		data["User"] = u
	}
	// Flash messages are passed in via query string (?ok=... or ?err=...).
	// Keeps the implementation cookie-free; sufficient for this app's needs.
	if e := r.URL.Query().Get("err"); e != "" {
		data["FlashError"] = e
	}
	if ok := r.URL.Query().Get("ok"); ok != "" {
		data["FlashOK"] = ok
	}
	tpl, ok := a.Templates[name]
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		fmt.Println("template error: no such page template:", name)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.ExecuteTemplate(w, name, data); err != nil {
		// Template execution is mid-response; we may already have started
		// writing. Best effort: log and abandon.
		fmt.Println("template error:", err)
	}
}

// NormalizePhone takes a user-entered phone string and returns it in E.164
// form (e.g. "+17065551234"). Strips spaces, dashes, parens, dots. Assumes
// US country code if input has 10 digits. Returns error for anything else.
func NormalizePhone(input string) (string, error) {
	stripped := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' { return r }
		if r == '+' { return r }
		return -1
	}, input)
	if strings.HasPrefix(stripped, "+") {
		// Already E.164-shaped. Require 11–15 digits after the plus.
		digits := stripped[1:]
		if len(digits) < 8 || len(digits) > 15 {
			return "", errors.New("phone: international number must be 8–15 digits after +")
		}
		return stripped, nil
	}
	switch len(stripped) {
	case 10:
		return "+1" + stripped, nil
	case 11:
		if stripped[0] != '1' {
			return "", errors.New("phone: 11-digit numbers must start with 1 (US country code)")
		}
		return "+" + stripped, nil
	default:
		return "", fmt.Errorf("phone: expected 10 digits (or +CC...); got %d", len(stripped))
	}
}
