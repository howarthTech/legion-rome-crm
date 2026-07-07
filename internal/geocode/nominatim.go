// Package geocode checks addresses against OpenStreetMap's Nominatim service.
//
// Why Nominatim: free, no API key, and a Legion post adds a venue a handful
// of times a year — far inside the usage policy (max 1 req/s, identifying
// User-Agent required; see https://operations.osmfoundation.org/policies/nominatim/).
// The CRM uses it only when an admin saves a new location, never on page views.
package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Result is what an address check returns.
type Result struct {
	Found         bool
	SuggestedName string // the venue/POI name OSM knows, if any ("The Farm", "Brookdale…")
	DisplayName   string // OSM's full formatted place string (shown as confirmation)
}

type Checker struct {
	baseURL string
	hc      *http.Client
}

// New returns a Checker against the public Nominatim instance.
func New() *Checker {
	return &Checker{
		baseURL: "https://nominatim.openstreetmap.org",
		hc:      &http.Client{Timeout: 10 * time.Second},
	}
}

// Check looks the address up. A nil error with Found=false means the service
// answered but knows no such place; a non-nil error means the service could
// not be reached (caller may offer to skip the check).
func (c *Checker) Check(ctx context.Context, address string) (Result, error) {
	q := url.Values{}
	q.Set("q", address)
	q.Set("format", "jsonv2")
	q.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/search?"+q.Encode(), nil)
	if err != nil {
		return Result{}, err
	}
	// Nominatim policy: identify the application.
	req.Header.Set("User-Agent", "legion-post-crm/1.0 (Legion Post Platform; howarth.tech)")

	resp, err := c.hc.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("address check unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("address check returned HTTP %d", resp.StatusCode)
	}

	var hits []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&hits); err != nil {
		return Result{}, fmt.Errorf("address check: bad response: %w", err)
	}
	if len(hits) == 0 {
		return Result{Found: false}, nil
	}

	name := strings.TrimSpace(hits[0].Name)
	if name == "" {
		// Plain street addresses have no POI name; suggest the first segment
		// of the formatted place ("493, Jones Bend Road Northeast" style).
		if i := strings.Index(hits[0].DisplayName, ","); i > 0 {
			name = strings.TrimSpace(hits[0].DisplayName[:i])
		}
	}
	return Result{Found: true, SuggestedName: name, DisplayName: hits[0].DisplayName}, nil
}
