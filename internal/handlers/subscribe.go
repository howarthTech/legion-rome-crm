package handlers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// The public self-serve opt-in page. This is the documented, verifiable opt-in
// path A2P 10DLC reviewers require: a page anyone can visit with an UNCHECKED
// consent checkbox and the full disclosures. Submitting creates the member as
// PENDING and fires the same reply-YES confirmation as the admin add flow — the
// checkbox is the consent, the reply-YES is the confirmation (double opt-in).
//
// It is public (no auth) and triggers a paid SMS, so it is defended with a
// honeypot field, a per-IP rate limit, a required consent box (enforced
// server-side), and de-duplication against existing members.

const (
	subWindow  = 10 * time.Minute
	subMaxHits = 5
)

type subRateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

var subLimiter = &subRateLimiter{hits: map[string][]time.Time{}}

// allow records a hit for ip and reports whether it is within the limit.
func (l *subRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-subWindow)
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= subMaxHits {
		l.hits[ip] = kept
		return false
	}
	l.hits[ip] = append(kept, time.Now())
	return true
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// SubscribeGet renders the public opt-in form.
func SubscribeGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Render(w, r, "subscribe", "Text reminders", map[string]any{})
	}
}

// SubscribePost validates the opt-in and starts the double opt-in.
func SubscribePost(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		render := func(data map[string]any) {
			a.Render(w, r, "subscribe", "Text reminders", data)
		}

		// Honeypot: real users leave the hidden "company" field empty. Bots
		// fill it — show the success page but do nothing.
		if strings.TrimSpace(r.PostForm.Get("company")) != "" {
			render(map[string]any{"Done": true})
			return
		}

		name := strings.TrimSpace(r.PostForm.Get("name"))
		rawPhone := strings.TrimSpace(r.PostForm.Get("phone"))
		consent := r.PostForm.Get("consent")
		echo := func(msg string) {
			render(map[string]any{"Error": msg, "FormName": name, "FormPhone": rawPhone})
		}

		if !subLimiter.allow(clientIP(r)) {
			echo("Too many attempts from your network. Please try again in a few minutes.")
			return
		}
		if name == "" {
			echo("Please enter your name.")
			return
		}
		if consent != "on" && consent != "yes" && consent != "1" {
			echo("Please check the box to agree to receive text reminders.")
			return
		}
		phone, err := app.NormalizePhone(rawPhone)
		if err != nil {
			echo("Please enter a valid US mobile number, e.g. (706) 555-1234.")
			return
		}

		ctx := r.Context()
		existing, _ := a.Store.GetMemberByPhone(ctx, phone)
		switch {
		case existing == nil:
			id, err := a.Store.InsertMember(ctx, name, "", phone, "", "Web opt-in "+time.Now().Format("2006-01-02"))
			if err == nil {
				_ = sendOptInSMS(ctx, a, id, phone, name, "")
			}
		case existing.OptInStatus == store.OptInPending:
			// Already invited, not yet confirmed — resend the confirmation.
			_ = sendOptInSMS(ctx, a, existing.ID, phone, existing.Name, existing.Title)
		case existing.OptInStatus == store.OptInConfirmed:
			// Already subscribed — nothing to send.
		default:
			// Previously opted out: respect it; do not re-invite from the web.
		}

		render(map[string]any{"Done": true, "FormName": name, "MaskedPhone": maskPhone(phone)})
	}
}

// maskPhone renders "+17065551234" as "ending in 1234" for the confirmation page.
func maskPhone(e164 string) string {
	if len(e164) >= 4 {
		return "ending in " + e164[len(e164)-4:]
	}
	return ""
}
