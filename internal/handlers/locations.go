package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/howarthTech/legion-rome-crm/internal/app"
)

// LocationsList shows known locations with an add form and delete actions.
func LocationsList(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		locs, err := a.Store.ListLocations(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Render(w, r, "locations_list", "Locations", map[string]any{
			"Locations": locs,
			"Form":      map[string]string{},
		})
	}
}

// LocationsCreate adds a location from the manage screen: checks the address,
// uses the custom name or falls back to the checked suggestion.
func LocationsCreate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		name, _, errMsg := resolveNewLocation(a, r,
			strings.TrimSpace(r.PostForm.Get("address")),
			strings.TrimSpace(r.PostForm.Get("name")),
			r.PostForm.Get("skipCheck") == "on")
		if errMsg != "" {
			locs, _ := a.Store.ListLocations(r.Context())
			a.Render(w, r, "locations_list", "Locations", map[string]any{
				"Locations": locs,
				"Error":     errMsg,
				"Form": map[string]string{
					"name":    r.PostForm.Get("name"),
					"address": r.PostForm.Get("address"),
				},
			})
			return
		}
		http.Redirect(w, r, "/locations?ok="+url.QueryEscape(fmt.Sprintf("Location %q added.", name)), http.StatusSeeOther)
	}
}

// LocationsDelete removes a known location (existing events keep their text).
func LocationsDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		loc, err := a.Store.GetLocation(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := a.Store.DeleteLocation(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/locations?ok="+url.QueryEscape(fmt.Sprintf("Location %q removed. Events that used it are unchanged.", loc.Name)), http.StatusSeeOther)
	}
}

// LocationsCheck is the small JSON endpoint the event/location forms call to
// preview an address check ("did we find it, and what would we call it?").
func LocationsCheck(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address := strings.TrimSpace(r.URL.Query().Get("address"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if address == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "empty address"})
			return
		}
		res, err := a.Geocode.Check(r.Context(), address)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "checker unreachable"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":            true,
			"found":         res.Found,
			"suggestedName": res.SuggestedName,
			"matched":       res.DisplayName,
		})
	}
}

// resolveNewLocation verifies an address (unless skipped), settles on a name
// (custom wins, otherwise the check's suggestion), and creates the location.
// Returns the final name and Display text, or a user-facing error message.
func resolveNewLocation(a *app.App, r *http.Request, address, customName string, skipCheck bool) (name, display, errMsg string) {
	if address == "" {
		return "", "", "Enter the location's address."
	}
	suggested := ""
	if !skipCheck {
		res, err := a.Geocode.Check(r.Context(), address)
		if err != nil {
			return "", "", "The address checker couldn't be reached. Try again, or tick “skip the address check.”"
		}
		if !res.Found {
			return "", "", "That address wasn't found. Double-check it, or tick “skip the address check” if it's correct anyway."
		}
		suggested = res.SuggestedName
	}
	name = customName
	if name == "" {
		name = suggested
	}
	if name == "" {
		return "", "", "Give this location a name (the address check didn't suggest one)."
	}
	id, err := a.Store.CreateLocation(r.Context(), name, address)
	if err != nil {
		return "", "", err.Error()
	}
	loc, err := a.Store.GetLocation(r.Context(), id)
	if err != nil {
		return "", "", err.Error()
	}
	return loc.Name, loc.Display(), ""
}
