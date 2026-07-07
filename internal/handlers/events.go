package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// EventsList shows every event with edit/delete actions.
func EventsList(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		evs, err := a.Store.ListEvents(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		now := time.Now()
		var upcoming, past []store.Event
		for _, e := range evs {
			if e.IsPast(now) {
				past = append(past, e)
			} else {
				upcoming = append(upcoming, e)
			}
		}
		// Past events read best newest-first.
		for i, j := 0, len(past)-1; i < j; i, j = i+1, j-1 {
			past[i], past[j] = past[j], past[i]
		}
		a.Render(w, r, "events_list", "Events", map[string]any{
			"Upcoming": upcoming,
			"Past":     past,
		})
	}
}

// EventsNewGet renders an empty event form.
func EventsNewGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		locs, _ := a.Store.ListLocations(r.Context())
		members, _ := a.Store.ListMembers(r.Context())
		a.Render(w, r, "events_form", "Add an event", map[string]any{
			"Action":    "/events",
			"Legend":    "Add an event",
			"Form":      map[string]string{},
			"Locations": locs,
			"Members":   members,
		})
	}
}

// EventsCreate handles the new-event POST.
func EventsCreate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ev, formErr := eventFromForm(a, r, nil)
		if formErr != "" {
			locs, _ := a.Store.ListLocations(r.Context())
			members, _ := a.Store.ListMembers(r.Context())
			a.Render(w, r, "events_form", "Add an event", map[string]any{
				"Action": "/events", "Legend": "Add an event",
				"Error": formErr, "Form": formEcho(r), "Locations": locs, "Members": members,
			})
			return
		}
		if _, err := a.Store.CreateEvent(r.Context(), ev); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		redirectEvents(w, r, "ok", fmt.Sprintf("Event %q added.%s", ev.Title, rebuildNote(a)))
	}
}

// EventsEditGet renders the form pre-filled for an existing event.
func EventsEditGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ev, err := getEventFromPath(a, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		form := map[string]string{
			"title":        ev.Title,
			"eventType":    ev.Type,
			"date":         ev.StartsAt.Format("2006-01-02"),
			"start":        ev.StartsAt.Format("15:04"),
			"end":          "",
			"contactName":  ev.ContactName,
			"contactPhone": ev.ContactPhone,
			"description":  ev.Description,
			"body":         ev.Body,
		}
		if ev.EndsAtRaw != "" {
			if t, err := time.Parse(time.RFC3339, ev.EndsAtRaw); err == nil {
				form["end"] = t.Format("15:04")
			}
		}
		locs, _ := a.Store.ListLocations(r.Context())
		members, _ := a.Store.ListMembers(r.Context())
		a.Render(w, r, "events_form", "Edit event", map[string]any{
			"Action":    fmt.Sprintf("/events/%d", ev.ID),
			"Legend":    "Edit event",
			"Form":      form,
			"Event":     ev,
			"Locations": locs,
			"Members":   members,
		})
	}
}

// EventsUpdate handles the edit POST.
func EventsUpdate(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, err := getEventFromPath(a, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ev, formErr := eventFromForm(a, r, existing)
		if formErr != "" {
			locs, _ := a.Store.ListLocations(r.Context())
			members, _ := a.Store.ListMembers(r.Context())
			a.Render(w, r, "events_form", "Edit event", map[string]any{
				"Action": fmt.Sprintf("/events/%d", existing.ID), "Legend": "Edit event",
				"Error": formErr, "Form": formEcho(r), "Event": existing, "Locations": locs, "Members": members,
			})
			return
		}
		if err := a.Store.UpdateEvent(r.Context(), existing.ID, ev); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		redirectEvents(w, r, "ok", fmt.Sprintf("Event %q updated.%s", ev.Title, rebuildNote(a)))
	}
}

// EventsDelete removes an event.
func EventsDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ev, err := getEventFromPath(a, r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := a.Store.DeleteEvent(r.Context(), ev.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Rebuild.Ping()
		redirectEvents(w, r, "ok", fmt.Sprintf("Event %q deleted.%s", ev.Title, rebuildNote(a)))
	}
}

// EventsAPI serves the public read-only feed the website builds from.
// Everything here already appears on the public site; no auth required.
func EventsAPI(a *app.App) http.HandlerFunc {
	type apiEvent struct {
		Slug         string `json:"slug"`
		Title        string `json:"title"`
		Type         string `json:"type"`
		StartsAt     string `json:"startsAt"`
		EndsAt       string `json:"endsAt,omitempty"`
		Location     string `json:"location,omitempty"`
		ContactName  string `json:"contactName,omitempty"`
		ContactPhone string `json:"contactPhone,omitempty"`
		Description  string `json:"description,omitempty"`
		Body         string `json:"body,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		evs, err := a.Store.ListEvents(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := struct {
			Generated string     `json:"generated"`
			OrgName   string     `json:"orgName"`
			Events    []apiEvent `json:"events"`
		}{
			Generated: time.Now().UTC().Format(time.RFC3339),
			OrgName:   a.OrgName,
			Events:    make([]apiEvent, 0, len(evs)),
		}
		for _, e := range evs {
			out.Events = append(out.Events, apiEvent{
				Slug: e.Slug, Title: e.Title, Type: e.Type, StartsAt: e.StartsAtRaw, EndsAt: e.EndsAtRaw,
				Location: e.Location, ContactName: e.ContactName, ContactPhone: e.ContactPhone,
				Description: e.Description, Body: e.Body,
			})
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// --- helpers -----------------------------------------------------------

func getEventFromPath(a *app.App, r *http.Request) (*store.Event, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, err
	}
	return a.Store.GetEvent(r.Context(), id)
}

// eventFromForm validates the posted form and assembles a store.Event.
// existing is non-nil on edit (enables the "keep current location" choice).
// Returns a user-facing error string when invalid.
func eventFromForm(a *app.App, r *http.Request, existing *store.Event) (store.Event, string) {
	if err := r.ParseForm(); err != nil {
		return store.Event{}, "That form didn't come through right — please try again."
	}
	f := func(k string) string { return strings.TrimSpace(r.PostForm.Get(k)) }

	title, date, start := f("title"), f("date"), f("start")
	if title == "" || date == "" || start == "" {
		return store.Event{}, "Title, date, and start time are required."
	}
	eventType := f("eventType")
	if eventType != store.EventTypePost && eventType != store.EventTypeCommunity {
		return store.Event{}, "Choose an event type."
	}
	loc := a.Quiet.Location()
	startsAt, err := time.ParseInLocation("2006-01-02 15:04", date+" "+start, loc)
	if err != nil {
		return store.Event{}, "Couldn't read the date/start time — use the date picker and HH:MM."
	}

	locationText, locErr := resolveEventLocation(a, r, existing)
	if locErr != "" {
		return store.Event{}, locErr
	}

	ev := store.Event{
		Title:        title,
		Type:         eventType,
		StartsAt:     startsAt,
		Location:     locationText,
		ContactName:  f("contactName"),
		ContactPhone: f("contactPhone"),
		Description:  f("description"),
		Body:         r.PostForm.Get("body"),
	}
	if end := f("end"); end != "" {
		endsAt, err := time.ParseInLocation("2006-01-02 15:04", date+" "+end, loc)
		if err != nil {
			return store.Event{}, "Couldn't read the end time — use HH:MM, or leave it blank."
		}
		if !endsAt.After(startsAt) {
			return store.Event{}, "End time must be after the start time."
		}
		ev.EndsAtRaw = endsAt.Format(time.RFC3339)
	}
	return ev, ""
}

// resolveEventLocation turns the form's location choice into the text stored
// on the event. Choices: "" (no location), "keep" (edit only — keep the
// current text), "loc:<id>" (a known location), or "new" (check the address,
// name it, save it as a known location, use it).
func resolveEventLocation(a *app.App, r *http.Request, existing *store.Event) (string, string) {
	choice := strings.TrimSpace(r.PostForm.Get("locationChoice"))
	switch {
	case choice == "" || choice == "none":
		return "", ""
	case choice == "keep":
		if existing == nil {
			return "", "Choose a location."
		}
		return existing.Location, ""
	case strings.HasPrefix(choice, "loc:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(choice, "loc:"), 10, 64)
		if err != nil {
			return "", "Choose a location."
		}
		loc, err := a.Store.GetLocation(r.Context(), id)
		if err != nil {
			return "", "That location no longer exists — pick another."
		}
		return loc.Display(), ""
	case choice == "new":
		_, display, errMsg := resolveNewLocation(a, r,
			strings.TrimSpace(r.PostForm.Get("newLocationAddress")),
			strings.TrimSpace(r.PostForm.Get("newLocationName")),
			r.PostForm.Get("skipCheck") == "on")
		return display, errMsg
	default:
		return "", "Choose a location."
	}
}

func formEcho(r *http.Request) map[string]string {
	m := map[string]string{}
	for _, k := range []string{"title", "eventType", "date", "start", "end", "locationChoice", "newLocationAddress", "newLocationName", "contactName", "contactPhone", "description", "body"} {
		m[k] = r.PostForm.Get(k)
	}
	return m
}

func rebuildNote(a *app.App) string {
	if a.Rebuild.Enabled() {
		return " Your website will show these changes in a couple of minutes."
	}
	return " Your website will show these changes in the next hour."
}

func redirectEvents(w http.ResponseWriter, r *http.Request, key, msg string) {
	http.Redirect(w, r, "/events?"+key+"="+url.QueryEscape(msg), http.StatusSeeOther)
}
