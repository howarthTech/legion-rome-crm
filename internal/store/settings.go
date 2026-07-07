package store

import (
	"context"
	"database/sql"
	"errors"
)

// Setting keys. Values are stored as "1"/"0" for booleans.
const (
	// SettingUseMemberTitles: collect a rank/title per member and use it when
	// addressing them in communications. Default on.
	SettingUseMemberTitles = "use_member_titles"
)

// GetSettingBool reads a boolean setting, returning def when unset.
func (s *Store) GetSettingBool(ctx context.Context, key string, def bool) (bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	return v == "1", nil
}

// SetSettingBool writes a boolean setting.
func (s *Store) SetSettingBool(ctx context.Context, key string, val bool) error {
	v := "0"
	if val {
		v = "1"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`, key, v)
	return err
}
