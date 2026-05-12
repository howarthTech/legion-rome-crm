-- Initial schema for Post 5 CRM.
-- Run automatically on startup by internal/store/store.go.

-- Members opted into the SMS reminder list. The opt-in flow is:
--
--   1. Admin adds member  → row inserted with opt_in_status = 'PENDING'
--   2. Twilio sends opt-in SMS  → member sees the consent message
--   3. Member replies "YES"      → opt_in_status = 'OPTED_IN'
--   4. Member replies "STOP"     → opt_in_status = 'OPTED_OUT'
--   5. Admin manual opt-out      → opt_in_status = 'OPTED_OUT'
--
-- Only OPTED_IN members receive event reminders. This is a TCPA-compliance
-- requirement, not a UX choice.
CREATE TABLE IF NOT EXISTS members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    phone TEXT NOT NULL UNIQUE,                  -- E.164, e.g. +17065551234
    email TEXT,                                  -- optional
    opt_in_status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK(opt_in_status IN ('PENDING', 'OPTED_IN', 'OPTED_OUT')),
    opt_in_requested_at TEXT NOT NULL,
    opt_in_confirmed_at TEXT,
    opt_out_at TEXT,
    notes TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_members_phone  ON members(phone);
CREATE INDEX IF NOT EXISTS idx_members_status ON members(opt_in_status);

-- Audit log of every SMS sent or received. Useful for:
--   - Debugging delivery issues
--   - Demonstrating consent capture if challenged
--   - Letting the admin see a member's message history
CREATE TABLE IF NOT EXISTS messages_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER REFERENCES members(id) ON DELETE SET NULL,
    direction TEXT NOT NULL CHECK(direction IN ('OUTBOUND', 'INBOUND')),
    phone TEXT NOT NULL,
    body TEXT NOT NULL,
    twilio_sid TEXT,                             -- Twilio's MessageSid
    status TEXT,                                 -- queued / sent / delivered / failed / received
    error_msg TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_messages_member  ON messages_log(member_id);
CREATE INDEX IF NOT EXISTS idx_messages_created ON messages_log(created_at);
