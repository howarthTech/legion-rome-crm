-- Events authored in the CRM. The CRM is the source of truth for the post's
-- events: the public website builds its event pages from GET /api/events.json
-- and the reminder screen reads this table directly.
CREATE TABLE IF NOT EXISTS events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    slug           TEXT    NOT NULL UNIQUE,      -- URL slug on the site: /events/<slug>/
    title          TEXT    NOT NULL,
    starts_at      TEXT    NOT NULL,             -- RFC3339 with the post's UTC offset
    starts_at_unix INTEGER NOT NULL,             -- same instant, for ordering/filtering
    ends_at        TEXT    NOT NULL DEFAULT '',  -- RFC3339 or empty
    location       TEXT    NOT NULL DEFAULT '',
    contact_name   TEXT    NOT NULL DEFAULT '',
    contact_phone  TEXT    NOT NULL DEFAULT '',
    description    TEXT    NOT NULL DEFAULT '',  -- one-liner (cards, SMS, meta description)
    body           TEXT    NOT NULL DEFAULT '',  -- markdown page body
    created_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_events_starts_at_unix ON events(starts_at_unix);
