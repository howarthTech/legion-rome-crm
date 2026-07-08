package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

		// First-run setup checklist: show until complete (or dismissed).
		ob, _ := a.Store.OnboardingStatus(ctx)
		dismissed, _ := a.Store.GetSettingBool(ctx, store.SettingOnboardingDismissed, false)

		a.Render(w, r, "dashboard", "Dashboard", map[string]any{
			"Total":          len(members),
			"OptedIn":        counts[store.OptInConfirmed],
			"Pending":        counts[store.OptInPending],
			"OptedOut":       counts[store.OptInOptedOut],
			"Onboarding":     ob,
			"ShowOnboarding": ob != nil && !ob.AllDone && !dismissed,
		})
	}
}

// OnboardingDismiss hides the setup checklist even if incomplete.
func OnboardingDismiss(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = a.Store.SetSettingBool(r.Context(), store.SettingOnboardingDismissed, true)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// validOnboardingStep guards the step key from arbitrary setting writes.
var validOnboardingStep = map[string]bool{
	"info": true, "roster": true, "pages": true, "events": true, "members": true,
}

// OnboardingSkip defers a step ("skip for now"); OnboardingUnskip brings it
// back. Both take ?step=<key> and return to the dashboard.
func OnboardingSkip(a *app.App, skip bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		step := r.URL.Query().Get("step")
		if validOnboardingStep[step] {
			_ = a.Store.SetOnboardingSkip(r.Context(), step, skip)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		useTitles, _ := a.Store.GetSettingBool(r.Context(), store.SettingUseMemberTitles, true)
		a.Render(w, r, "members_new", "Add member", map[string]any{
			"UseTitles": useTitles,
		})
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
		title := strings.TrimSpace(r.PostForm.Get("title"))
		rawPhone := strings.TrimSpace(r.PostForm.Get("phone"))
		email := strings.TrimSpace(r.PostForm.Get("email"))
		notes := strings.TrimSpace(r.PostForm.Get("notes"))

		useTitles, _ := a.Store.GetSettingBool(ctx, store.SettingUseMemberTitles, true)
		if !useTitles {
			title = "" // setting off → never store or use a title
		}
		echo := func(errMsg string) map[string]any {
			return map[string]any{
				"Error": errMsg, "UseTitles": useTitles,
				"FormName": name, "FormTitle": title, "FormPhone": rawPhone,
				"FormEmail": email, "FormNotes": notes,
			}
		}

		if name == "" {
			a.Render(w, r, "members_new", "Add member", echo("Name is required."))
			return
		}

		phone, err := app.NormalizePhone(rawPhone)
		if err != nil {
			a.Render(w, r, "members_new", "Add member", echo("Phone: "+err.Error()))
			return
		}

		id, err := a.Store.InsertMember(ctx, name, title, phone, email, notes)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, store.ErrPhoneExists) {
				msg = "A member with that phone number is already on the list."
			}
			a.Render(w, r, "members_new", "Add member", echo(msg))
			return
		}

		// Send the opt-in consent SMS. If this fails, the member row stays —
		// admin can re-send from the member detail page.
		_ = sendOptInSMS(ctx, a, id, phone, name, title)

		http.Redirect(w, r,
			fmt.Sprintf("/members/%d?ok=%s", id, url.QueryEscape("Member added. Opt-in SMS sent.")),
			http.StatusSeeOther)
	}
}

// MembersEditGet renders the edit form pre-filled with the member's details.
func MembersEditGet(a *app.App) http.HandlerFunc {
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
		useTitles, _ := a.Store.GetSettingBool(r.Context(), store.SettingUseMemberTitles, true)
		a.Render(w, r, "members_edit", "Edit "+m.Name, map[string]any{
			"UseTitles": useTitles,
			"MemberID":  m.ID,
			"FormName":  m.Name, "FormTitle": m.Title, "FormPhone": displayPhone(m.Phone),
			"FormEmail": m.Email, "FormNotes": m.Notes,
		})
	}
}

// MembersEditPost validates and saves edited member details. Opt-in status is
// untouched (see store.UpdateMember) — this is a details edit, not a consent
// change. Correcting a typo'd phone keeps the member's consent; if the number
// belongs to a different person, the admin should remove and re-add so they get
// a fresh opt-in (the form says so).
func MembersEditPost(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.PostForm.Get("name"))
		title := strings.TrimSpace(r.PostForm.Get("title"))
		rawPhone := strings.TrimSpace(r.PostForm.Get("phone"))
		email := strings.TrimSpace(r.PostForm.Get("email"))
		notes := strings.TrimSpace(r.PostForm.Get("notes"))

		useTitles, _ := a.Store.GetSettingBool(ctx, store.SettingUseMemberTitles, true)
		if !useTitles {
			title = "" // setting off → never store or use a title
		}
		echo := func(errMsg string) map[string]any {
			return map[string]any{
				"Error": errMsg, "UseTitles": useTitles, "MemberID": id,
				"FormName": name, "FormTitle": title, "FormPhone": rawPhone,
				"FormEmail": email, "FormNotes": notes,
			}
		}

		if name == "" {
			a.Render(w, r, "members_edit", "Edit member", echo("Name is required."))
			return
		}
		phone, err := app.NormalizePhone(rawPhone)
		if err != nil {
			a.Render(w, r, "members_edit", "Edit member", echo("Phone: "+err.Error()))
			return
		}

		if err := a.Store.UpdateMember(ctx, id, name, title, phone, email, notes); err != nil {
			switch {
			case errors.Is(err, store.ErrPhoneExists):
				a.Render(w, r, "members_edit", "Edit member", echo("Another member already has that phone number."))
			case errors.Is(err, store.ErrMemberNotFound):
				http.NotFound(w, r)
			default:
				a.Render(w, r, "members_edit", "Edit member", echo(err.Error()))
			}
			return
		}
		http.Redirect(w, r,
			fmt.Sprintf("/members/%d?ok=%s", id, url.QueryEscape("Member details updated.")),
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
		payments, _ := a.Store.ListDuesPayments(r.Context(), id)
		now := time.Now()
		a.Render(w, r, "member_view", m.Name, map[string]any{
			"Member":       m,
			"Messages":     msgs,
			"DuesPayments": payments,
			"Today":        now.Format("2006-01-02"),
			"CurrentYear":  now.Format("2006"),
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
		if err := sendOptInSMS(r.Context(), a, m.ID, m.Phone, m.Name, m.Title); err != nil {
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
func sendOptInSMS(ctx context.Context, a *app.App, memberID int64, phone, name, title string) error {
	useTitles, _ := a.Store.GetSettingBool(ctx, store.SettingUseMemberTitles, true)
	body := fmt.Sprintf(
		"%s: Hi %s, you've been added to our event reminder list. Reply YES to confirm and receive SMS reminders about meetings and events. Reply STOP to opt out. Msg & data rates may apply.",
		a.OrgName, salutation(name, title, useTitles),
	)
	res, err := a.Twilio.Send(ctx, phone, body)
	if err != nil {
		_ = a.Store.LogOutbound(ctx, memberID, phone, body, "", "failed", err.Error())
		return err
	}
	return a.Store.LogOutbound(ctx, memberID, phone, body, res.SID, res.Status, "")
}

// salutation is how communications address a member: with titles enabled and
// present, rank/title + last name ("Commander Hollis"); otherwise first name.
func salutation(name, title string, useTitles bool) string {
	if useTitles && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title) + " " + lastName(name)
	}
	return firstName(name)
}

// displayPhone renders a stored E.164 number in a friendly form for pre-filling
// the edit form: "+17065551234" → "(706) 555-1234". Non-US (or unexpected)
// numbers are shown as-is. Either way NormalizePhone re-parses it on submit.
func displayPhone(e164 string) string {
	if len(e164) == 12 && strings.HasPrefix(e164, "+1") {
		d := e164[2:]
		return "(" + d[0:3] + ") " + d[3:6] + "-" + d[6:10]
	}
	return e164
}

func firstName(full string) string {
	full = strings.TrimSpace(full)
	if i := strings.IndexAny(full, " "); i > 0 {
		return full[:i]
	}
	return full
}

func lastName(full string) string {
	full = strings.TrimSpace(full)
	if i := strings.LastIndexAny(full, " "); i >= 0 && i < len(full)-1 {
		return full[i+1:]
	}
	return full
}
