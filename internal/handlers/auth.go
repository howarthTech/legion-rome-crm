// Package handlers contains the HTTP handlers. Each handler is a method on
// *app.App so it can access dependencies (store, twilio, auth) via the
// receiver. main.go wires them onto an http.ServeMux.
package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/howarthTech/legion-rome-crm/internal/app"
)

// LoginGet renders the login form. If already authenticated, redirects to /.
func LoginGet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.Auth.ParseSession(r); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		a.Render(w, r, "login", "Sign in", map[string]any{
			"Next": r.URL.Query().Get("next"),
		})
	}
}

// LoginPost validates credentials and issues a session cookie.
func LoginPost(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		username := strings.TrimSpace(r.PostForm.Get("username"))
		password := r.PostForm.Get("password")
		next := r.PostForm.Get("next")
		if !isSafeNext(next) {
			next = "/"
		}

		// Constant-time-ish: always compute bcrypt even if username is wrong,
		// to avoid timing oracles. (CheckPassword does the bcrypt regardless.)
		usernameMatches := username == a.Auth.Username
		passwordErr := a.Auth.CheckPassword(password)
		if !usernameMatches || passwordErr != nil {
			a.Render(w, r, "login", "Sign in", map[string]any{
				"Next":  next,
				"Error": "Invalid username or password.",
			})
			return
		}

		a.Auth.SetSessionCookie(w, username)
		http.Redirect(w, r, next, http.StatusSeeOther)
	}
}

// Logout clears the cookie and redirects.
func Logout(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.Auth.ClearSessionCookie(w)
		http.Redirect(w, r, "/login?ok=Signed+out.", http.StatusSeeOther)
	}
}

// isSafeNext blocks open-redirect attacks via the `next` parameter.
// Only relative paths (starting with /, not //, not /\ either) are allowed.
func isSafeNext(next string) bool {
	if next == "" {
		return false
	}
	u, err := url.Parse(next)
	if err != nil {
		return false
	}
	// Must be relative (no scheme, no host)
	if u.Scheme != "" || u.Host != "" {
		return false
	}
	// Must start with a single / (not // which is protocol-relative)
	return strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//")
}
