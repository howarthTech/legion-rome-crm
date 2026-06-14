// Package events fetches the post's public events feed (the static site's
// /events/events.json) so the CRM can offer the admin a list of upcoming
// events to send reminders for — without duplicating event data.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// Event mirrors one entry in the site's events.json feed.
type Event struct {
	Slug         string `json:"slug"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	StartsAt     string `json:"startsAt"` // RFC3339 with offset
	EndsAt       string `json:"endsAt"`
	Location     string `json:"location"`
	Description  string `json:"description"`
	ContactName  string `json:"contactName"`
	ContactPhone string `json:"contactPhone"`
	IsPast       bool   `json:"isPast"`
	ICSURL       string `json:"icsURL"`
}

// Start parses StartsAt; zero time on error.
func (e Event) Start() time.Time {
	t, err := time.Parse(time.RFC3339, e.StartsAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

type feed struct {
	Generated string  `json:"generated"`
	SiteName  string  `json:"siteName"`
	Events    []Event `json:"events"`
}

// Client fetches and caches the feed. Construct with NewClient.
type Client struct {
	feedURL    string
	httpClient *http.Client

	// small cache so repeated dashboard loads don't hammer the site
	cached    []Event
	cachedAt  time.Time
	cacheTTL  time.Duration
}

// NewClient returns a feed client. feedURL is the site's events.json
// (e.g. https://romelegion.org/events/events.json). Empty feedURL → Upcoming
// always returns an empty list (feature simply unavailable).
func NewClient(feedURL string) *Client {
	return &Client{
		feedURL:    feedURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cacheTTL:   2 * time.Minute,
	}
}

// Configured reports whether a feed URL was provided.
func (c *Client) Configured() bool { return c.feedURL != "" }

// Upcoming returns future events soonest-first. Uses a short cache.
func (c *Client) Upcoming(ctx context.Context, now time.Time) ([]Event, error) {
	all, err := c.fetch(ctx, now)
	if err != nil {
		return nil, err
	}
	var up []Event
	for _, e := range all {
		if !e.IsPast && e.Start().After(now) {
			up = append(up, e)
		}
	}
	sort.Slice(up, func(i, j int) bool { return up[i].Start().Before(up[j].Start()) })
	return up, nil
}

// Find returns the event with the given slug from the feed.
func (c *Client) Find(ctx context.Context, slug string, now time.Time) (*Event, error) {
	all, err := c.fetch(ctx, now)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Slug == slug {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("event %q not found in feed", slug)
}

func (c *Client) fetch(ctx context.Context, now time.Time) ([]Event, error) {
	if !c.Configured() {
		return nil, nil
	}
	if c.cached != nil && now.Sub(c.cachedAt) < c.cacheTTL {
		return c.cached, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.feedURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch events feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("events feed returned %d", resp.StatusCode)
	}
	var f feed
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return nil, fmt.Errorf("decode events feed: %w", err)
	}
	c.cached = f.Events
	c.cachedAt = now
	return f.Events, nil
}
