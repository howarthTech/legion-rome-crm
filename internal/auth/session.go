// Package auth implements a minimal session-cookie scheme:
//
//   - One configured admin user (ADMIN_USERNAME + ADMIN_PASSWORD_HASH env vars).
//   - On login, we set a signed cookie with the username and expiry.
//   - Middleware verifies the cookie's HMAC and expiry on protected routes.
//
// No external dependencies. ~100 lines. For a single-user admin tool, this is
// plenty — full OIDC / OAuth would be overkill for 20–50 members managed by
// one officer.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName    = "rome_admin_session"
	sessionTTL    = 12 * time.Hour
	contextUserKey = "session_user"
)

// Manager holds the credentials + signing key needed to issue and verify
// sessions. Construct once with New() and pass to handlers.
type Manager struct {
	Username     string
	PasswordHash []byte // bcrypt hash
	SecretKey    []byte // HMAC signing key (random, kept in env)
}

// New builds a Manager from the typical env triple.
func New(username, passwordHash, secret string) (*Manager, error) {
	if username == "" || passwordHash == "" || secret == "" {
		return nil, errors.New("auth: ADMIN_USERNAME, ADMIN_PASSWORD_HASH, and SESSION_SECRET are required")
	}
	if len(secret) < 32 {
		return nil, errors.New("auth: SESSION_SECRET must be at least 32 characters")
	}
	return &Manager{
		Username:     username,
		PasswordHash: []byte(passwordHash),
		SecretKey:    []byte(secret),
	}, nil
}

// CheckPassword returns nil if the password matches the configured hash.
// Uses bcrypt's constant-time comparison.
func (m *Manager) CheckPassword(password string) error {
	return bcrypt.CompareHashAndPassword(m.PasswordHash, []byte(password))
}

// SetSessionCookie writes a fresh signed session cookie to the response.
func (m *Manager) SetSessionCookie(w http.ResponseWriter, username string) {
	expiry := time.Now().Add(sessionTTL)
	payload := username + "|" + strconv.FormatInt(expiry.Unix(), 10)
	signature := m.sign(payload)
	value := base64.URLEncoding.EncodeToString([]byte(payload + "|" + signature))

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		Secure:   true, // safe for our deployment (TLS at the Caddy layer)
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie nukes the session cookie. Called on /logout.
func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ParseSession reads the cookie from a request, verifies its HMAC, and
// returns the username if valid and not expired. Returns "" + error otherwise.
func (m *Manager) ParseSession(r *http.Request) (string, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", err
	}
	raw, err := base64.URLEncoding.DecodeString(c.Value)
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 {
		return "", errors.New("malformed session")
	}
	username, expiryStr, signature := parts[0], parts[1], parts[2]

	// Constant-time HMAC comparison
	expected := m.sign(username + "|" + expiryStr)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return "", errors.New("invalid session signature")
	}

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil || time.Unix(expiry, 0).Before(time.Now()) {
		return "", errors.New("session expired")
	}
	return username, nil
}

func (m *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.SecretKey)
	mac.Write([]byte(payload))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

// RequireAuth is HTTP middleware that redirects unauthenticated requests to
// /login. Wrap any handler that should be admin-only.
func (m *Manager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := m.ParseSession(r); err != nil {
			// Preserve the originally requested URL so we can come back after login.
			next := r.URL.RequestURI()
			http.Redirect(w, r, "/login?next="+next, http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
