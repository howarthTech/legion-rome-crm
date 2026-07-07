-- Site content authored in the CRM: the post's identity/contact config, the
-- officer + family roster, and editable prose pages. The public website builds
-- from these via the CRM's content API (same pattern as events). This makes
-- the CRM the single source of truth for the whole site and lets post officers
-- edit their own site.

-- Flat key/value config (identity, contact, branding). Keys are a known set
-- documented in internal/store/siteconfig.go.
CREATE TABLE site_config (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Officer + family-program roster. group_type separates the post's officers
-- from the Legion-family contacts (Auxiliary/SAL/Riders).
CREATE TABLE roster (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    group_type TEXT    NOT NULL DEFAULT 'officer' CHECK (group_type IN ('officer','family')),
    role       TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    phone      TEXT    NOT NULL DEFAULT '',
    email      TEXT    NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_roster_group ON roster(group_type, sort_order);

-- Editable prose pages (history, rental, family intro, membership, etc.).
-- slug is the stable page key; body is markdown.
CREATE TABLE pages (
    slug       TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
