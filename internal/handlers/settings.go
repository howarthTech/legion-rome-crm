package handlers

import (
	"net/http"
	"net/url"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// SettingsGet shows the per-post settings screen.
func SettingsGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		useTitles, _ := a.Store.GetSettingBool(r.Context(), store.SettingUseMemberTitles, true)
		a.Render(w, r, "settings", "Settings", map[string]any{
			"UseTitles": useTitles,
		})
	}
}

// SettingsPost saves the settings form.
func SettingsPost(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		useTitles := r.PostForm.Get("useTitles") == "on"
		if err := a.Store.SetSettingBool(r.Context(), store.SettingUseMemberTitles, useTitles); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/settings?ok="+url.QueryEscape("Settings saved."), http.StatusSeeOther)
	}
}
