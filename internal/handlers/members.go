package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// Dashboard is the post-login landing page. Shows quick stats + a link to the
// members list.
func Dashboard(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		members, err := a.Store.ListMembers(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		counts := map[store.OptInStatus]int{}
		for _, m := range members {
			counts[m.OptInStatus]++
		}
		a.Render(w, r, "dashboard", "Dashboard", map[string]any{
			"Total":   len(members),
			"OptedIn": counts[store.OptInConfirmed],
			"Pending": counts[store.OptInPending],
			"OptedOut": counts[store.OptInOptedOut],
		})
	}
}

// MembersList shows all members in a table.
func MembersList(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members, err := a.Store.ListMembers(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.Render(w, r, "members_list", "Members", map[string]any{
			"Members": members,
		})
	}
}

// MembersNewGet renders the "add member" form.
func MembersNewGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, r, "members_new", "Add member", nil)
	}
}

// MembersNewPost validates input, inserts the row, and triggers the opt-in SMS.
// The opt-in SMS body is fixed and includes STOP instructions — TCPA requires
// every commercial / informational message include opt-out information.
func MembersNewPost(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.PostForm.Get("name"))
		rawPhone := strings.TrimSpace(r.PostForm.Get("phone"))
		email := strings.TrimSpace(r.PostForm.Get("email"))
		notes := strings.TrimSpace(r.PostForm.Get("notes"))

		if name == "" {
			a.Render(w, r, "members_new", "Add member", map[string]any{
				"Error": "Name is required.",
				"FormName": name, "FormPhone": rawPhone, "FormEmail": email, "FormNotes": notes,
			})
			return
		}

		phone, err := app.NormalizePhone(rawPhone)
		if err != nil {
			a.Render(w, r, "members_new", "Add member", map[string]any{
				"Error": "Phone: " + err.Error(),
				"FormName": name, "FormPhone": rawPhone, "FormEmail": email, "FormNotes": notes,
			})
			return
		}

		id, err := a.Store.InsertMember(ctx, name, phone, email, notes)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, store.ErrPhoneExists) {
				msg = "A member with that phone number is already on the list."
			}
			a.Render(w, r, "members_new", "Add member", map[string]any{
				"Error": msg,
				"FormName": name, "FormPhone": rawPhone, "FormEmail": email, "FormNotes": notes,
			})
			return
		}

		// Send the opt-in consent SMS. If this fails, the member row stays —
		// admin can re-send from the member detail page.
		_ = sendOptInSMS(ctx, a, id, phone, name)

		http.Redirect(w, r,
			fmt.Sprintf("/members/%d?ok=%s", id, url.QueryEscape("Member added. Opt-in SMS sent.")),
			http.StatusSeeOther)
	}
}

// MembersView shows a single member: details, opt-in status, and message log.
func MembersView(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		m, err := a.Store.GetMember(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrMemberNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		msgs, _ := a.Store.MessagesForMember(r.Context(), id, 50)
		a.Render(w, r, "member_view", m.Name, map[string]any{
			"Member":   m,
			"Messages": msgs,
		})
	}
}

// MembersResendOptIn — admin can re-trigger the consent SMS for a PENDING member.
func MembersResendOptIn(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		m, err := a.Store.GetMember(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := sendOptInSMS(r.Context(), a, m.ID, m.Phone, m.Name); err != nil {
			http.Redirect(w, r, fmt.Sprintf("/members/%d?err=%s", id, url.QueryEscape(err.Error())), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/members/%d?ok=%s", id, url.QueryEscape("Opt-in SMS resent.")), http.StatusSeeOther)
	}
}

// MembersOptOut — admin manually marks the member as opted out (e.g. they
// asked verbally or by email).
func MembersOptOut(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := a.Store.SetOptedOut(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/members/%d?ok=%s", id, url.QueryEscape("Marked opted out.")), http.StatusSeeOther)
	}
}

// MembersDelete removes the row.
func MembersDelete(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := a.Store.DeleteMember(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/members?ok="+url.QueryEscape("Member deleted."), http.StatusSeeOther)
	}
}

// sendOptInSMS — fires the consent message and logs it. Used by both new-add
// and re-send-opt-in flows.
func sendOptInSMS(ctx context.Context, a *app.App, memberID int64, phone, name string) error {
	first := firstName(name)
	body := fmt.Sprintf(
		"%s: Hi %s, you've been added to our event reminder list. Reply YES to confirm and receive SMS reminders about meetings and events. Reply STOP to opt out. Msg & data rates may apply.",
		a.OrgName, first,
	)
	res, err := a.Twilio.Send(ctx, phone, body)
	if err != nil {
		_ = a.Store.LogOutbound(ctx, memberID, phone, body, "", "failed", err.Error())
		return err
	}
	return a.Store.LogOutbound(ctx, memberID, phone, body, res.SID, res.Status, "")
}

func firstName(full string) string {
	full = strings.TrimSpace(full)
	if i := strings.IndexAny(full, " "); i > 0 {
		return full[:i]
	}
	return full
}
