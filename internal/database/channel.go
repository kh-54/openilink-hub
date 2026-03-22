package database

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
)

type FilterRule struct {
	UserIDs      []string `json:"user_ids,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	MessageTypes []string `json:"message_types,omitempty"` // "text","image","voice","file","video"
}

type Channel struct {
	ID         string     `json:"id"`
	BotID      string     `json:"bot_id"`
	Name       string     `json:"name"`
	Handle     string     `json:"handle"` // @mention handle for routing
	APIKey     string     `json:"api_key"`
	FilterRule FilterRule `json:"filter_rule"`
	Enabled    bool       `json:"enabled"`
	LastSeq    int64      `json:"last_seq"`
	CreatedAt  int64      `json:"created_at"`
	UpdatedAt  int64      `json:"updated_at"`
}

func generateAPIKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const channelSelectCols = `id, bot_id, name, handle, api_key, filter_rule, enabled, last_seq,
	EXTRACT(EPOCH FROM created_at)::BIGINT, EXTRACT(EPOCH FROM updated_at)::BIGINT`

func scanChannel(scanner interface{ Scan(...any) error }) (*Channel, error) {
	c := &Channel{}
	var filterJSON []byte
	err := scanner.Scan(&c.ID, &c.BotID, &c.Name, &c.Handle, &c.APIKey,
		&filterJSON, &c.Enabled, &c.LastSeq, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(filterJSON, &c.FilterRule)
	return c, nil
}

func (db *DB) CreateChannel(botID, name, handle string, filter *FilterRule) (*Channel, error) {
	id := uuid.New().String()
	apiKey := generateAPIKey()
	if filter == nil {
		filter = &FilterRule{}
	}
	filterJSON, _ := json.Marshal(filter)
	_, err := db.Exec(
		"INSERT INTO channels (id, bot_id, name, handle, api_key, filter_rule) VALUES ($1, $2, $3, $4, $5, $6)",
		id, botID, name, handle, apiKey, filterJSON,
	)
	if err != nil {
		return nil, err
	}
	return &Channel{ID: id, BotID: botID, Name: name, Handle: handle, APIKey: apiKey,
		FilterRule: *filter, Enabled: true}, nil
}

func (db *DB) GetChannel(id string) (*Channel, error) {
	return scanChannel(db.QueryRow("SELECT "+channelSelectCols+" FROM channels WHERE id = $1", id))
}

func (db *DB) GetChannelByAPIKey(apiKey string) (*Channel, error) {
	return scanChannel(db.QueryRow("SELECT "+channelSelectCols+" FROM channels WHERE api_key = $1", apiKey))
}

func (db *DB) ListChannelsByBot(botID string) ([]Channel, error) {
	rows, err := db.Query("SELECT "+channelSelectCols+" FROM channels WHERE bot_id = $1 AND enabled = TRUE", botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chs []Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		chs = append(chs, *c)
	}
	return chs, rows.Err()
}

func (db *DB) ListChannelsByBotIDs(botIDs []string) ([]Channel, error) {
	if len(botIDs) == 0 {
		return nil, nil
	}
	// Build query with ANY
	rows, err := db.Query(
		"SELECT "+channelSelectCols+" FROM channels WHERE bot_id = ANY($1) ORDER BY created_at",
		botIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chs []Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		chs = append(chs, *c)
	}
	return chs, rows.Err()
}

func (db *DB) UpdateChannel(id, name, handle string, filter *FilterRule, enabled bool) error {
	filterJSON, _ := json.Marshal(filter)
	_, err := db.Exec(
		"UPDATE channels SET name = $1, handle = $2, filter_rule = $3, enabled = $4, updated_at = NOW() WHERE id = $5",
		name, handle, filterJSON, enabled, id,
	)
	return err
}

func (db *DB) DeleteChannel(id string) error {
	_, err := db.Exec("DELETE FROM channels WHERE id = $1", id)
	return err
}

func (db *DB) RotateChannelKey(id string) (string, error) {
	newKey := generateAPIKey()
	_, err := db.Exec("UPDATE channels SET api_key = $1, updated_at = NOW() WHERE id = $2", newKey, id)
	return newKey, err
}

func (db *DB) UpdateChannelLastSeq(channelID string, seq int64) error {
	_, err := db.Exec("UPDATE channels SET last_seq = $1, updated_at = NOW() WHERE id = $2", seq, channelID)
	return err
}

func (db *DB) CountChannelsByBot(botID string) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM channels WHERE bot_id = $1", botID).Scan(&count)
	return count, err
}
