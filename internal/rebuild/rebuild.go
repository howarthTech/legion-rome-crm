// Package rebuild pings the website's GitHub Actions deploy workflow after
// event changes so the public site rebuilds with the new data. Optional: with
// no token configured, Ping is a no-op and the site catches up on its next
// scheduled build instead.
package rebuild

import (
	"bytes"
	"log"
	"net/http"
	"time"
)

type Notifier struct {
	token string // GitHub token with permission to dispatch workflows on repo
	repo  string // e.g. "howarthTech/legion-rome"
	hc    *http.Client
}

// New builds a Notifier. Either value empty → notifier is disabled.
func New(token, repo string) *Notifier {
	return &Notifier{token: token, repo: repo, hc: &http.Client{Timeout: 15 * time.Second}}
}

// Enabled reports whether pings will actually be sent.
func (n *Notifier) Enabled() bool { return n.token != "" && n.repo != "" }

// Ping asynchronously triggers the site's "Build & Deploy" workflow. Fire and
// forget: a failed ping only delays the site update until the next scheduled
// build, so it logs rather than surfacing to the admin.
func (n *Notifier) Ping() {
	if !n.Enabled() {
		return
	}
	go func() {
		url := "https://api.github.com/repos/" + n.repo + "/actions/workflows/deploy.yml/dispatches"
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(`{"ref":"main"}`)))
		if err != nil {
			log.Println("rebuild ping:", err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+n.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := n.hc.Do(req)
		if err != nil {
			log.Println("rebuild ping:", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			log.Println("rebuild ping: GitHub returned", resp.Status)
		}
	}()
}
