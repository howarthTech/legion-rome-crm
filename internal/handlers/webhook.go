package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

// TwilioInbound — POST /webhooks/twilio
//
// Twilio calls this URL every time someone replies to one of our messages.
// We parse the From + Body, match to a member, and update opt-in status.
//
// Twilio expects either an empty 200 or a TwiML response — we return TwiML
// to send a confirmation back to the user when their reply changes status.
//
// Security: Twilio signs every request with X-Twilio-Signature. We verify it
// before doing anything. Without verification, anyone could spoof opt-ins.
func TwilioInbound(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		// Reconstruct the full URL Twilio called (used as part of the HMAC).
		fullURL := a.PublicURL + r.URL.RequestURI()
		signature := r.Header.Get("X-Twilio-Signature")
		if !a.Twilio.VerifyWebhook(signature, fullURL, r.PostForm) {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}

		from := r.PostForm.Get("From")
		body := r.PostForm.Get("Body")
		sid := r.PostForm.Get("MessageSid")

		ctx := r.Context()
		var memberID int64
		if m, err := a.Store.GetMemberByPhone(ctx, from); err == nil {
			memberID = m.ID
		}
		// Log every inbound regardless of whether we recognize the sender —
		// useful if someone messages the Twilio number out of the blue.
		_ = a.Store.LogInbound(ctx, memberID, from, body, sid)

		// Decide what to do based on the body text. We accept any common opt-in
		// phrase and any of the keywords Twilio auto-handles for opt-out, plus
		// a couple of human variations.
		reply := ""
		switch classifyReply(body) {
		case replyYes:
			if memberID == 0 {
				// Unknown number sent YES; nothing to confirm.
				reply = ""
			} else {
				_ = a.Store.SetOptedIn(ctx, memberID)
				reply = fmt.Sprintf(
					"%s: You're confirmed on our event reminder list. We'll text you ahead of upcoming meetings. Reply STOP anytime to opt out.",
					a.OrgName)
				_ = a.Store.LogOutbound(ctx, memberID, from, reply, "", "queued", "")
			}
		case replyStop:
			if memberID > 0 {
				_ = a.Store.SetOptedOut(ctx, memberID)
			}
			// Twilio's "Advanced Opt-Out" auto-handles the STOP response for
			// us when enabled on the account — so we usually don't need to
			// reply here. Leave reply empty.
			reply = ""
		default:
			// Unknown reply — could be a real question. Don't auto-reply;
			// admin sees it in the dashboard and follows up.
			reply = ""
		}

		respondTwiML(w, reply)
	}
}

type replyKind int

const (
	replyUnknown replyKind = iota
	replyYes
	replyStop
)

func classifyReply(body string) replyKind {
	s := strings.ToUpper(strings.TrimSpace(body))
	// Strip punctuation so "YES!" matches.
	s = strings.TrimFunc(s, func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	})
	switch s {
	case "YES", "Y", "YEP", "YEAH", "CONFIRM", "OK", "OKAY", "JOIN":
		return replyYes
	case "STOP", "STOPALL", "UNSUBSCRIBE", "CANCEL", "END", "QUIT", "OPTOUT", "NO":
		return replyStop
	}
	return replyUnknown
}

// respondTwiML writes a minimal TwiML response. If body is empty, returns
// "<Response></Response>" which tells Twilio "noop, don't reply".
func respondTwiML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if body == "" {
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Response></Response>`)
		return
	}
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Response><Message>%s</Message></Response>`,
		xmlEscape(body))
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// Quiet the "unused import" warning during partial builds.
var _ = context.Background
var _ = store.OptInConfirmed
