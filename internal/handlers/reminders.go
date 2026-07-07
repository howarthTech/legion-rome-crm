package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// RemindersGet shows the send-reminder screen: upcoming events (authored in
// this CRM), the opted-in member count, and the quiet-hours status.
func RemindersGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()

		data := map[string]any{
			"QuietWindow": a.Quiet.Window(),
			"SendAllowed": a.Quiet.Allowed(now),
		}

		// Opted-in count (the only members who can receive a reminder).
		optedIn, err := a.Store.ListOptedIn(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data["OptedInCount"] = len(optedIn)

		// Post events only — community events are never SMS-reminded.
		upcoming, err := a.Store.UpcomingEvents(ctx, now, store.EventTypePost)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data["Events"] = upcoming

		a.Render(w, r, "reminders", "Send a reminder", data)
	}
}

// RemindersSend sends an SMS reminder for the chosen event to every OPTED_IN
// member. Guarded by quiet hours. Each message includes STOP instructions
// (TCPA) and is audit-logged.
func RemindersSend(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		slug := strings.TrimSpace(r.PostForm.Get("event"))
		if slug == "" {
			redirectReminders(w, r, "err", "Choose an event to send a reminder for.")
			return
		}

		// Quiet-hours hard guard.
		if !a.Quiet.Allowed(now) {
			redirectReminders(w, r, "err",
				fmt.Sprintf("It's currently outside sending hours (%s). Please try again during the day.", a.Quiet.Window()))
			return
		}

		ev, err := a.Store.GetEventBySlug(ctx, slug)
		if err != nil {
			redirectReminders(w, r, "err", "That event doesn't exist anymore.")
			return
		}

		optedIn, err := a.Store.ListOptedIn(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(optedIn) == 0 {
			redirectReminders(w, r, "err", "No opted-in members to send to yet.")
			return
		}

		body := reminderBody(a.OrgName, ev)

		sent, failed := 0, 0
		for _, m := range optedIn {
			res, err := a.Twilio.Send(ctx, m.Phone, body)
			if err != nil {
				failed++
				_ = a.Store.LogOutbound(ctx, m.ID, m.Phone, body, "", "failed", err.Error())
				continue
			}
			sent++
			_ = a.Store.LogOutbound(ctx, m.ID, m.Phone, body, res.SID, res.Status, "")
		}

		msg := fmt.Sprintf("Reminder for %q sent to %d member%s.", ev.Title, sent, plural(sent))
		if failed > 0 {
			msg += fmt.Sprintf(" %d failed (see member message logs).", failed)
		}
		redirectReminders(w, r, "ok", msg)
	}
}

// reminderBody composes the SMS. Kept short (SMS segments) and includes the
// required opt-out line. Example:
//
//	American Legion Post 5: Reminder — Post 5 Monthly Meeting on Mon, Jul 13
//	at 6:00 PM, The Farm. Reply STOP to opt out.
func reminderBody(orgName string, ev *store.Event) string {
	when := ""
	if t := ev.StartsAt; !t.IsZero() {
		when = t.Format("Mon, Jan 2 at 3:04 PM")
	}
	loc := ev.Location
	// Trim an overly long location to keep the message compact.
	if i := strings.Index(loc, " — "); i > 0 {
		loc = loc[:i]
	}
	parts := []string{fmt.Sprintf("%s: Reminder — %s", orgName, ev.Title)}
	if when != "" {
		parts = append(parts, "on "+when)
	}
	if loc != "" {
		parts = append(parts, "at "+loc)
	}
	return strings.Join(parts, " ") + ". Reply STOP to opt out."
}

func redirectReminders(w http.ResponseWriter, r *http.Request, key, msg string) {
	http.Redirect(w, r, "/reminders?"+key+"="+url.QueryEscape(msg), http.StatusSeeOther)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
