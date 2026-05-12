package store

import (
	"context"
	"database/sql"
	"time"
)

// MessageDirection corresponds to messages_log.direction.
type MessageDirection string

const (
	MessageOutbound MessageDirection = "OUTBOUND"
	MessageInbound  MessageDirection = "INBOUND"
)

// MessageLog is one row in messages_log.
type MessageLog struct {
	ID         int64
	MemberID   sql.NullInt64 // null if Twilio sent from an unknown number
	Direction  MessageDirection
	Phone      string
	Body       string
	TwilioSID  string
	Status     string
	ErrorMsg   string
	CreatedAt  time.Time
}

// LogOutbound records a sent SMS attempt. Call after the Twilio API responds —
// pass the returned SID on success, or the error string on failure.
func (s *Store) LogOutbound(ctx context.Context, memberID int64, phone, body, twilioSID, status, errMsg string) error {
	const q = `INSERT INTO messages_log (member_id, direction, phone, body, twilio_sid, status, error_msg)
	            VALUES (?, 'OUTBOUND', ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`
	_, err := s.db.ExecContext(ctx, q, nullableID(memberID), phone, body, twilioSID, status, errMsg)
	return err
}

// LogInbound records a received SMS. memberID may be 0 if the sender isn't in
// the members table — we still log the message for audit purposes.
func (s *Store) LogInbound(ctx context.Context, memberID int64, phone, body, twilioSID string) error {
	const q = `INSERT INTO messages_log (member_id, direction, phone, body, twilio_sid, status)
	            VALUES (?, 'INBOUND', ?, ?, NULLIF(?, ''), 'received')`
	_, err := s.db.ExecContext(ctx, q, nullableID(memberID), phone, body, twilioSID)
	return err
}

// MessagesForMember returns the most recent N messages for a member, newest first.
func (s *Store) MessagesForMember(ctx context.Context, memberID int64, limit int) ([]MessageLog, error) {
	const q = `SELECT id, member_id, direction, phone, body,
	                  COALESCE(twilio_sid,''), COALESCE(status,''), COALESCE(error_msg,''),
	                  created_at
	             FROM messages_log
	            WHERE member_id = ?
	            ORDER BY created_at DESC
	            LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, memberID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessageLog
	for rows.Next() {
		var m MessageLog
		var createdAt string
		if err := rows.Scan(&m.ID, &m.MemberID, &m.Direction, &m.Phone, &m.Body,
			&m.TwilioSID, &m.Status, &m.ErrorMsg, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = parseRFC3339(createdAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullableID(id int64) sql.NullInt64 {
	if id == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: id, Valid: true}
}
