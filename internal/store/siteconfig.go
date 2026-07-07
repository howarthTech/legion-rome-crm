package store

import "context"

// SiteConfigKeys is the canonical set of per-post identity/contact/branding
// values the website reads. Editing screens iterate this so adding a key here
// surfaces it in the admin and the API automatically.
var SiteConfigKeys = []struct {
	Key   string
	Label string
	Help  string
	Long  bool // render as a textarea
}{
	{"postName", "Post name", "Formal name as on your charter.", false},
	{"postShortName", "Short name", "Conversational form used around the site.", false},
	{"charterYear", "Charter year", "", false},
	{"description", "One-line description", "Used in search results and page meta.", true},
	{"locality", "City / town", "", false},
	{"region", "State (2-letter)", "", false},
	{"regionLong", "State (full name)", "", false},
	{"serviceArea", "Service area", "e.g. Rome and Floyd County.", false},
	{"timezone", "Time zone (IANA)", "e.g. America/New_York.", false},
	{"email", "Public email", "Where the contact form and published address point.", false},
	{"phone", "Public phone", "", false},
	{"mailingAddress", "Mailing address", "", false},
	{"meetingLocation", "Meeting location", "", false},
	{"meetingSchedule", "Meeting schedule", "e.g. 2nd Monday at 6:00 PM.", false},
	{"facebookURL", "Facebook URL", "Your post's page or group.", false},
	{"heroTitle", "Homepage headline", "The big hero heading.", false},
	{"heroImageAlt", "Hero image description", "Alt text for the hero photo (accessibility).", true},
}

// GetConfig returns one config value ("" if unset).
func (s *Store) GetConfig(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM site_config WHERE key = ?", key).Scan(&v)
	if err != nil && err.Error() == "sql: no rows in result set" {
		return "", nil
	}
	return v, err
}

// AllConfig returns every stored config key/value as a map.
func (s *Store) AllConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM site_config")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetConfig upserts one config value.
func (s *Store) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO site_config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`, key, value)
	return err
}
