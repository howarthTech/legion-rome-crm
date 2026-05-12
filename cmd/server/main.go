// Post 5 CRM — admin tool for managing the SMS reminder list.
//
// Wires the store, Twilio client, and auth manager into an http.ServeMux,
// then serves on $PORT. Config comes from environment variables; see
// .env.example.
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/howarthTech/legion-rome-crm/internal/app"
	"github.com/howarthTech/legion-rome-crm/internal/auth"
	"github.com/howarthTech/legion-rome-crm/internal/handlers"
	"github.com/howarthTech/legion-rome-crm/internal/sms"
	"github.com/howarthTech/legion-rome-crm/internal/store"
)

//go:embed all:web/templates
var templatesFS embed.FS

//go:embed all:web/static
var staticFS embed.FS

func main() {
	cfg := loadConfig()

	// --- Store -----------------------------------------------------------
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	// --- Twilio ----------------------------------------------------------
	twilio := sms.NewClient(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioFromNumber)
	if twilio.DryRun {
		log.Println("⚠ Twilio in DRYRUN mode — sends will be logged to stdout, not transmitted. Set TWILIO_* env vars to enable real sends.")
	}

	// --- Auth ------------------------------------------------------------
	authMgr, err := auth.New(cfg.AdminUsername, cfg.AdminPasswordHash, cfg.SessionSecret)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	// --- App + routes ----------------------------------------------------
	a, err := app.New(st, twilio, authMgr, templatesFS, staticFS, cfg.PublicURL, cfg.OrgName)
	if err != nil {
		log.Fatalf("app: %v", err)
	}

	mux := http.NewServeMux()

	// Public — login + webhook
	mux.HandleFunc("GET /login", handlers.LoginGet(a))
	mux.HandleFunc("POST /login", handlers.LoginPost(a))
	mux.HandleFunc("POST /logout", handlers.Logout(a))
	mux.HandleFunc("POST /webhooks/twilio", handlers.TwilioInbound(a))

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", a.StaticFS))

	// Protected — admin only
	mux.HandleFunc("GET /{$}", authMgr.RequireAuth(handlers.Dashboard(a)))
	mux.HandleFunc("GET /members", authMgr.RequireAuth(handlers.MembersList(a)))
	mux.HandleFunc("GET /members/new", authMgr.RequireAuth(handlers.MembersNewGet(a)))
	mux.HandleFunc("POST /members", authMgr.RequireAuth(handlers.MembersNewPost(a)))
	mux.HandleFunc("GET /members/{id}", authMgr.RequireAuth(handlers.MembersView(a)))
	mux.HandleFunc("POST /members/{id}/resend-opt-in", authMgr.RequireAuth(handlers.MembersResendOptIn(a)))
	mux.HandleFunc("POST /members/{id}/opt-out", authMgr.RequireAuth(handlers.MembersOptOut(a)))
	mux.HandleFunc("POST /members/{id}/delete", authMgr.RequireAuth(handlers.MembersDelete(a)))

	// Healthcheck for the deploy script's polling
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "ok")
	})

	// --- Server ----------------------------------------------------------
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Listening on %s (public URL: %s)", cfg.Listen, cfg.PublicURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("Shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// securityHeaders wraps the mux with a few defensive headers.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "interest-cohort=()")
		// Robots tag: admin should never be indexed even if accidentally exposed.
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		h.ServeHTTP(w, r)
	})
}

// --- Config -----------------------------------------------------------------

type config struct {
	Listen            string
	DBPath            string
	PublicURL         string
	OrgName           string
	AdminUsername     string
	AdminPasswordHash string
	SessionSecret     string
	TwilioAccountSID  string
	TwilioAuthToken   string
	TwilioFromNumber  string
}

func loadConfig() config {
	c := config{
		Listen:            envOr("LISTEN_ADDR", "127.0.0.1:8081"),
		DBPath:            envOr("DB_PATH", "./data/crm.db"),
		PublicURL:         envOr("PUBLIC_URL", "http://localhost:8081"),
		OrgName:           envOr("ORG_NAME", "American Legion Post 5"),
		AdminUsername:     os.Getenv("ADMIN_USERNAME"),
		AdminPasswordHash: os.Getenv("ADMIN_PASSWORD_HASH"),
		SessionSecret:     os.Getenv("SESSION_SECRET"),
		TwilioAccountSID:  os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:   os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioFromNumber:  os.Getenv("TWILIO_FROM_NUMBER"),
	}
	return c
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
