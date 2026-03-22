package database

import "encoding/json"

type Message struct {
	ID        int64           `json:"id"`
	BotID     string          `json:"bot_id"`
	ChannelID *string         `json:"channel_id,omitempty"`
	Direction string          `json:"direction"`
	Sender    string          `json:"sender"`
	Recipient string          `json:"recipient,omitempty"`
	MsgType   string          `json:"msg_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt int64           `json:"created_at"`
}

func (db *DB) SaveMessage(m *Message) (int64, error) {
	if m.Payload == nil {
		m.Payload = json.RawMessage(`{}`)
	}
	var id int64
	err := db.QueryRow(`
		INSERT INTO messages (bot_id, channel_id, direction, sender, recipient, msg_type, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		m.BotID, m.ChannelID, m.Direction, m.Sender, m.Recipient, m.MsgType, m.Payload,
	).Scan(&id)
	return id, err
}

func (db *DB) ListMessages(botID string, limit int, beforeID int64) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows interface {
		Close() error
		Next() bool
		Scan(...any) error
		Err() error
	}
	var err error
	if beforeID > 0 {
		rows, err = db.Query(`
			SELECT id, bot_id, channel_id, direction, sender, recipient, msg_type, payload,
			       EXTRACT(EPOCH FROM created_at)::BIGINT
			FROM messages WHERE bot_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3`,
			botID, beforeID, limit,
		)
	} else {
		rows, err = db.Query(`
			SELECT id, bot_id, channel_id, direction, sender, recipient, msg_type, payload,
			       EXTRACT(EPOCH FROM created_at)::BIGINT
			FROM messages WHERE bot_id = $1 ORDER BY id DESC LIMIT $2`,
			botID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.BotID, &m.ChannelID, &m.Direction,
			&m.Sender, &m.Recipient, &m.MsgType, &m.Payload, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (db *DB) ListMessagesBySender(botID, sender string, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, bot_id, channel_id, direction, sender, recipient, msg_type, payload,
		       EXTRACT(EPOCH FROM created_at)::BIGINT
		FROM messages WHERE bot_id = $1 AND (sender = $2 OR recipient = $2)
		ORDER BY id DESC LIMIT $3`,
		botID, sender, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.BotID, &m.ChannelID, &m.Direction,
			&m.Sender, &m.Recipient, &m.MsgType, &m.Payload, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// ListChannelMessages returns conversation history for a sender within a channel.
func (db *DB) ListChannelMessages(channelID, sender string, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, bot_id, channel_id, direction, sender, recipient, msg_type, payload,
		       EXTRACT(EPOCH FROM created_at)::BIGINT
		FROM messages
		WHERE channel_id = $1 AND (sender = $2 OR recipient = $2)
		ORDER BY id DESC LIMIT $3`,
		channelID, sender, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.BotID, &m.ChannelID, &m.Direction,
			&m.Sender, &m.Recipient, &m.MsgType, &m.Payload, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (db *DB) GetMessagesSince(botID string, afterSeq int64, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(`
		SELECT id, bot_id, channel_id, direction, sender, recipient, msg_type, payload,
		       EXTRACT(EPOCH FROM created_at)::BIGINT
		FROM messages WHERE bot_id = $1 AND id > $2 ORDER BY id ASC LIMIT $3`,
		botID, afterSeq, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.BotID, &m.ChannelID, &m.Direction,
			&m.Sender, &m.Recipient, &m.MsgType, &m.Payload, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (db *DB) PruneMessages(maxAgeDays int) (int64, error) {
	result, err := db.Exec("DELETE FROM messages WHERE created_at < NOW() - INTERVAL '1 day' * $1", maxAgeDays)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
