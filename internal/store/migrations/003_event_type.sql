-- Two kinds of events:
--   'post'      — the post's own events (meetings, parties). Eligible for
--                 SMS reminders to opted-in members.
--   'community' — public events the post takes part in (parades, ceremonies,
--                 town events). Shown on the website, never SMS-reminded.
ALTER TABLE events ADD COLUMN event_type TEXT NOT NULL DEFAULT 'post'
    CHECK (event_type IN ('post', 'community'));
