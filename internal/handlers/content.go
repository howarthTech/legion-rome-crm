package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// ContentHub is the landing page for editing the public website's content.
func ContentHub(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, r, "content_hub", "Website content", nil)
	}
}

// --- Post info (site_config) -----------------------------------------------

func ContentInfoGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := a.Store.AllConfig(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Render(w, r, "content_info", "Post info", map[string]any{
			"Keys":   store.SiteConfigKeys,
			"Values": cfg,
		})
	}
}

func ContentInfoPost(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		for _, k := range store.SiteConfigKeys {
			if err := a.Store.SetConfig(r.Context(), k.Key, strings.TrimSpace(r.PostForm.Get(k.Key))); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		a.Rebuild.Ping()
		redirect(w, r, "/content/info", "ok", "Post info saved."+rebuildNote(a))
	}
}

// --- Roster (officers + family) --------------------------------------------

func ContentRoster(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		officers, _ := a.Store.ListRoster(ctx, store.RosterOfficer)
		family, _ := a.Store.ListRoster(ctx, store.RosterFamily)
		a.Render(w, r, "content_roster", "Officers & family", map[string]any{
			"Officers": officers,
			"Family":   family,
		})
	}
}

func ContentRosterCreate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		group := r.PostForm.Get("group")
		if group != store.RosterOfficer && group != store.RosterFamily {
			redirect(w, r, "/content/roster", "err", "Unknown group.")
			return
		}
		m := store.RosterMember{
			Group: group,
			Role:  strings.TrimSpace(r.PostForm.Get("role")),
			Name:  strings.TrimSpace(r.PostForm.Get("name")),
			Phone: strings.TrimSpace(r.PostForm.Get("phone")),
			Email: strings.TrimSpace(r.PostForm.Get("email")),
		}
		if m.Role == "" || m.Name == "" {
			redirect(w, r, "/content/roster", "err", "Role and name are required.")
			return
		}
		if _, err := a.Store.CreateRosterMember(r.Context(), m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		redirect(w, r, "/content/roster", "ok", fmt.Sprintf("%s added.%s", m.Name, rebuildNote(a)))
	}
}

func ContentRosterUpdate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		m := store.RosterMember{
			Role:  strings.TrimSpace(r.PostForm.Get("role")),
			Name:  strings.TrimSpace(r.PostForm.Get("name")),
			Phone: strings.TrimSpace(r.PostForm.Get("phone")),
			Email: strings.TrimSpace(r.PostForm.Get("email")),
		}
		if m.Role == "" || m.Name == "" {
			redirect(w, r, "/content/roster", "err", "Role and name are required.")
			return
		}
		if err := a.Store.UpdateRosterMember(r.Context(), id, m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		redirect(w, r, "/content/roster", "ok", "Saved."+rebuildNote(a))
	}
}

func ContentRosterDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := a.Store.DeleteRosterMember(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		redirect(w, r, "/content/roster", "ok", "Removed."+rebuildNote(a))
	}
}

func ContentRosterMove(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		delta := 1
		if r.URL.Query().Get("dir") == "up" {
			delta = -1
		}
		if err := a.Store.MoveRosterMember(r.Context(), id, delta); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		http.Redirect(w, r, "/content/roster", http.StatusSeeOther)
	}
}

// --- Pages -----------------------------------------------------------------

func ContentPages(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stored, _ := a.Store.ListPages(r.Context())
		type row struct {
			Slug, Title, Help string
			Written           bool
		}
		var rows []row
		for _, d := range store.PageDefs {
			_, ok := stored[d.Slug]
			rows = append(rows, row{Slug: d.Slug, Title: d.Title, Help: d.Help, Written: ok})
		}
		a.Render(w, r, "content_pages", "Pages", map[string]any{"Pages": rows})
	}
}

func ContentPageEditGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var def *struct{ Slug, Title, Help string }
		for _, d := range store.PageDefs {
			if d.Slug == slug {
				dd := struct{ Slug, Title, Help string }{d.Slug, d.Title, d.Help}
				def = &dd
				break
			}
		}
		if def == nil {
			http.NotFound(w, r)
			return
		}
		page, _ := a.Store.GetPage(r.Context(), slug)
		title, body := def.Title, ""
		if page != nil {
			if page.Title != "" {
				title = page.Title
			}
			body = page.Body
		}
		a.Render(w, r, "content_page", "Edit: "+def.Title, map[string]any{
			"Slug": def.Slug, "DefTitle": def.Title, "Help": def.Help,
			"Title": title, "Body": body,
		})
	}
}

func ContentPageSave(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		known := false
		for _, d := range store.PageDefs {
			if d.Slug == slug {
				known = true
				break
			}
		}
		if !known {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(r.PostForm.Get("title"))
		body := r.PostForm.Get("body")
		if err := a.Store.SavePage(r.Context(), slug, title, body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		redirect(w, r, "/content/pages", "ok", fmt.Sprintf("%q saved.%s", title, rebuildNote(a)))
	}
}

// redirect is a small flash-redirect helper shared by the content handlers.
func redirect(w http.ResponseWriter, r *http.Request, path, key, msg string) {
	http.Redirect(w, r, path+"?"+key+"="+url.QueryEscape(msg), http.StatusSeeOther)
}
