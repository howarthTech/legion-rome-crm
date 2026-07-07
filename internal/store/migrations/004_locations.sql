-- Known locations — reusable venues for events. Name is the human label
-- ("The Farm"), address is the checked street address. Events still store a
-- rendered "Name — Address" string, so deleting a location never breaks
-- existing events.
CREATE TABLE locations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    address    TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
