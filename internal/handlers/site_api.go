package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// SiteAPI serves the public site-content feed the website builds from:
// identity/contact config, the officer + family roster, and prose pages.
// Everything here is already public on the site; no auth by design.
func SiteAPI(a *app.App) http.HandlerFunc {
	type apiPerson struct {
		Role  string `json:"role"`
		Name  string `json:"name"`
		Phone string `json:"phone,omitempty"`
		Email string `json:"email,omitempty"`
	}
	type apiPage struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		config, err := a.Store.AllConfig(ctx)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// ORG_NAME is authoritative for the post name even if config is unset.
		if config == nil {
			config = map[string]string{}
		}
		if config["postName"] == "" {
			config["postName"] = a.OrgName
		}

		officers, _ := a.Store.ListRoster(ctx, store.RosterOfficer)
		family, _ := a.Store.ListRoster(ctx, store.RosterFamily)
		pages, _ := a.Store.ListPages(ctx)

		toPeople := func(ms []store.RosterMember) []apiPerson {
			out := make([]apiPerson, 0, len(ms))
			for _, m := range ms {
				out = append(out, apiPerson{Role: m.Role, Name: m.Name, Phone: m.Phone, Email: m.Email})
			}
			return out
		}
		apiPages := make([]apiPage, 0, len(pages))
		for _, p := range pages {
			apiPages = append(apiPages, apiPage{Slug: p.Slug, Title: p.Title, Body: p.Body})
		}

		out := struct {
			Generated string            `json:"generated"`
			Config    map[string]string `json:"config"`
			Officers  []apiPerson       `json:"officers"`
			Family    []apiPerson       `json:"family"`
			Pages     []apiPage         `json:"pages"`
		}{
			Generated: time.Now().UTC().Format(time.RFC3339),
			Config:    config,
			Officers:  toPeople(officers),
			Family:    toPeople(family),
			Pages:     apiPages,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(out)
	}
}
