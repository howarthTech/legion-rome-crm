-- Dues tracking. Each member carries a current dues status (paid up vs due) and
-- the membership year they're paid through; every individual payment (how, when,
-- how much, notes) is logged in dues_payments so the post keeps a full history.
-- Existing members get status 'DUE' by the column default.
ALTER TABLE members ADD COLUMN dues_status TEXT NOT NULL DEFAULT 'DUE';
ALTER TABLE members ADD COLUMN dues_paid_through TEXT NOT NULL DEFAULT '';

CREATE TABLE dues_payments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id       INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    paid_on         TEXT    NOT NULL,             -- YYYY-MM-DD the dues were paid
    amount_cents    INTEGER,                      -- optional; NULL = amount not recorded
    method          TEXT    NOT NULL DEFAULT '',  -- Cash / Check / Card / Money order / ...
    membership_year TEXT    NOT NULL DEFAULT '',  -- e.g. "2026" — the year these dues cover
    notes           TEXT    NOT NULL DEFAULT '',
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_dues_payments_member ON dues_payments(member_id, paid_on, id);
