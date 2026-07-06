package events

import "time"

// QuietHours blocks SMS sends late at night / early morning. TCPA best
// practice (and many state laws) restrict messaging to ~8 AM–9 PM in the
// recipient's local time. We use the post's configured timezone as a proxy
// for the members' local time (members of a local post are in its timezone).
//
// Window: sends are allowed 09:00–21:00 inclusive of 9 AM, exclusive of 9 PM.
type QuietHours struct {
	loc        *time.Location
	startHour  int // first allowed hour (09)
	endHour    int // first disallowed hour (21)
}

// NewQuietHours builds a guard for the given IANA timezone (e.g.
// "America/New_York"). Falls back to UTC if the zone can't be loaded.
func NewQuietHours(tz string) *QuietHours {
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	return &QuietHours{loc: loc, startHour: 9, endHour: 21}
}

// Location exposes the post's timezone (used by the event form to interpret
// the admin's local date/time input).
func (q *QuietHours) Location() *time.Location { return q.loc }

// Allowed reports whether sending is permitted at instant t.
func (q *QuietHours) Allowed(t time.Time) bool {
	h := t.In(q.loc).Hour()
	return h >= q.startHour && h < q.endHour
}

// Window returns a human label like "9:00 AM–9:00 PM America/New_York".
func (q *QuietHours) Window() string {
	return "9:00 AM–9:00 PM " + q.loc.String()
}
