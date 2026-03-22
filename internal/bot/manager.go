package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/openilink/openilink-hub/internal/database"
	"github.com/openilink/openilink-hub/internal/provider"
	"github.com/openilink/openilink-hub/internal/relay"
	"github.com/openilink/openilink-hub/internal/sink"
	"github.com/openilink/openilink-hub/internal/storage"
)

var mentionRe = regexp.MustCompile(`@(\S+)`)

func parseMentions(text string) []string {
	matches := mentionRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var handles []string
	for _, m := range matches {
		handles = append(handles, m[1])
	}
	return handles
}

// Manager manages all active bot instances.
type Manager struct {
	mu        sync.RWMutex
	instances map[string]*Instance
	db        *database.DB
	hub       *relay.Hub
	sinks     []sink.Sink
	store     *storage.Storage // optional, for media files
}

func NewManager(db *database.DB, hub *relay.Hub, sinks []sink.Sink, store *storage.Storage) *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		db:        db,
		hub:       hub,
		sinks:     sinks,
		store:     store,
	}
}

func (m *Manager) StartAll(ctx context.Context) {
	bots, err := m.db.GetAllBots()
	if err != nil {
		slog.Error("failed to load bots", "err", err)
		return
	}
	for _, b := range bots {
		if len(b.Credentials) == 0 || string(b.Credentials) == "{}" {
			continue
		}
		if err := m.StartBot(ctx, &b); err != nil {
			slog.Error("failed to start bot", "bot", b.ID, "err", err)
		}
	}
	slog.Info("started all bots", "count", len(bots))
}

func (m *Manager) StartBot(ctx context.Context, bot *database.Bot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.instances[bot.ID]; ok {
		old.Stop()
	}

	factory, ok := provider.Get(bot.Provider)
	if !ok {
		slog.Error("unknown provider", "provider", bot.Provider, "bot", bot.ID)
		return nil
	}

	p := factory()
	inst := NewInstance(bot.ID, p)

	err := p.Start(ctx, provider.StartOptions{
		Credentials: bot.Credentials,
		SyncState:   bot.SyncState,
		OnMessage: func(msg provider.InboundMessage) {
			m.onInbound(inst, msg)
		},
		OnStatus: func(status string) {
			_ = m.db.UpdateBotStatus(bot.ID, status)
			m.onStatusChange(inst, status)
		},
		OnSyncUpdate: func(state json.RawMessage) {
			_ = m.db.UpdateBotSyncState(bot.ID, state)
		},
	})
	if err != nil {
		return err
	}

	m.instances[bot.ID] = inst
	slog.Info("bot started", "bot", bot.ID, "provider", bot.Provider)
	return nil
}

func (m *Manager) StopBot(botDBID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.instances[botDBID]; ok {
		inst.Stop()
		delete(m.instances, botDBID)
	}
}

func (m *Manager) GetInstance(botDBID string) (*Instance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.instances[botDBID]
	return inst, ok
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range m.instances {
		inst.Stop()
	}
	m.instances = make(map[string]*Instance)
}

func (m *Manager) onStatusChange(inst *Instance, status string) {
	env := relay.NewEnvelope("bot_status", relay.BotStatusData{
		BotID:  inst.DBID,
		Status: status,
	})
	m.hub.Broadcast(inst.DBID, env)
}

// onInbound routes an inbound message with filtering, then delivers to sinks.
func (m *Manager) onInbound(inst *Instance, msg provider.InboundMessage) {
	// Determine primary msg type and content for storage
	msgType := "text"
	content := ""
	for _, item := range msg.Items {
		switch item.Type {
		case "text":
			content = item.Text
		case "image", "voice", "file", "video":
			msgType = item.Type
			if content == "" {
				if item.Text != "" {
					content = item.Text
				} else if item.FileName != "" {
					content = item.FileName
				} else {
					content = "[" + item.Type + "]"
				}
			}
		}
	}

	_ = m.db.IncrBotMsgCount(inst.DBID)

	// Download and store media files
	if m.store != nil {
		m.processMedia(inst, &msg)
	}

	// Build payload with media URLs
	payloadMap := map[string]any{"content": content}
	if msg.GroupID != "" {
		payloadMap["group_id"] = msg.GroupID
	}
	if msg.ContextToken != "" {
		payloadMap["context_token"] = msg.ContextToken
	}
	// Include media URLs in payload
	for _, item := range msg.Items {
		if item.Media != nil && item.Media.URL != "" {
			payloadMap["media_url"] = item.Media.URL
			payloadMap["media_type"] = item.Media.MediaType
			break
		}
	}
	payload, _ := json.Marshal(payloadMap)

	items := make([]relay.MessageItem, len(msg.Items))
	for i, item := range msg.Items {
		items[i] = convertRelayItem(item)
	}

	// Load channels and route — match first channel only
	channels, err := m.db.ListChannelsByBot(inst.DBID)
	if err != nil {
		slog.Error("load channels failed", "bot", inst.DBID, "err", err)
		return
	}

	var target *database.Channel
	mentioned := parseMentions(content)
	if len(mentioned) > 0 {
		// First @mention match
		first := strings.ToLower(mentioned[0])
		for _, ch := range channels {
			if ch.Handle != "" && strings.ToLower(ch.Handle) == first {
				target = &ch
				break
			}
		}
	} else {
		// First filter match
		for _, ch := range channels {
			if matchFilter(ch.FilterRule, msg.Sender, content, msgType) {
				target = &ch
				break
			}
		}
	}

	if target == nil {
		// No channel matched — store without channel_id
		m.db.SaveMessage(&database.Message{
			BotID: inst.DBID, Direction: "inbound", Sender: msg.Sender,
			Recipient: msg.Recipient, MsgType: msgType, Payload: payload,
		})
		return
	}

	// Store inbound with channel_id
	chID := target.ID
	seqID, _ := m.db.SaveMessage(&database.Message{
		BotID: inst.DBID, ChannelID: &chID, Direction: "inbound",
		Sender: msg.Sender, Recipient: msg.Recipient, MsgType: msgType, Payload: payload,
	})
	_ = m.db.UpdateChannelLastSeq(target.ID, seqID)

	env := relay.NewEnvelope("message", relay.MessageData{
		SeqID: seqID, ExternalID: msg.ExternalID,
		Sender: msg.Sender, Recipient: msg.Recipient, GroupID: msg.GroupID,
		Timestamp: msg.Timestamp, MessageState: msg.MessageState,
		Items: items, ContextToken: msg.ContextToken, SessionID: msg.SessionID,
	})

	d := sink.Delivery{
		BotDBID: inst.DBID, Provider: inst.Provider, Channel: *target,
		Message: msg, Envelope: env, SeqID: seqID, MsgType: msgType, Content: content,
	}
	for _, s := range m.sinks {
		go s.Handle(d)
	}
}

// matchFilter checks if a message passes the channel's filter rule.
// processMedia downloads media items and stores them, replacing URLs.
func (m *Manager) processMedia(inst *Instance, msg *provider.InboundMessage) {
	ctx := context.Background()
	for i := range msg.Items {
		item := &msg.Items[i]
		if item.Media == nil || item.Media.EncryptQueryParam == "" {
			continue
		}
		data, err := inst.Provider.DownloadMedia(ctx, item.Media.EncryptQueryParam, item.Media.AESKey)
		if err != nil {
			slog.Error("media download failed", "bot", inst.DBID, "type", item.Type, "err", err)
			continue
		}
		ext := mediaExt(item.Type)
		contentType := mediaContentType(item.Type)
		key := fmt.Sprintf("media/%s/%s/%d%s", inst.DBID, msg.ExternalID, i, ext)
		url, err := m.store.Put(ctx, key, contentType, data)
		if err != nil {
			slog.Error("media store failed", "bot", inst.DBID, "key", key, "err", err)
			continue
		}
		item.Media.URL = url
		item.Media.FileSize = int64(len(data))
	}
}

func mediaExt(itemType string) string {
	switch itemType {
	case "image":
		return ".jpg"
	case "voice":
		return ".silk"
	case "video":
		return ".mp4"
	default:
		return ""
	}
}

func mediaContentType(itemType string) string {
	switch itemType {
	case "image":
		return "image/jpeg"
	case "voice":
		return "audio/silk"
	case "video":
		return "video/mp4"
	case "file":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}

func matchFilter(rule database.FilterRule, sender, text, msgType string) bool {
	if len(rule.UserIDs) > 0 {
		found := false
		for _, uid := range rule.UserIDs {
			if uid == sender {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(rule.MessageTypes) > 0 {
		found := false
		for _, mt := range rule.MessageTypes {
			if mt == msgType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(rule.Keywords) > 0 {
		found := false
		lower := strings.ToLower(text)
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func convertRelayItem(item provider.MessageItem) relay.MessageItem {
	ri := relay.MessageItem{
		Type:     item.Type,
		Text:     item.Text,
		FileName: item.FileName,
	}
	if item.Media != nil {
		ri.Media = &relay.Media{
			URL:         item.Media.URL,
			AESKey:      item.Media.AESKey,
			FileSize:    item.Media.FileSize,
			MediaType:   item.Media.MediaType,
			PlayTime:    item.Media.PlayTime,
			PlayLength:  item.Media.PlayLength,
			ThumbWidth:  item.Media.ThumbWidth,
			ThumbHeight: item.Media.ThumbHeight,
		}
	}
	if item.RefMsg != nil {
		refItem := convertRelayItem(item.RefMsg.Item)
		ri.RefMsg = &relay.RefMsg{
			Title: item.RefMsg.Title,
			Item:  refItem,
		}
	}
	return ri
}
