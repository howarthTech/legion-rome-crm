-- Member rank/title ("Commander", "SGT", "Dr.") — used to address the member
-- in communications when the per-post setting is on.
ALTER TABLE members ADD COLUMN title TEXT NOT NULL DEFAULT '';

-- Per-tenant settings, key/value. First key: use_member_titles ('1'/'0',
-- absent = on).
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
