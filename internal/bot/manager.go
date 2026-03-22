package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"
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
	baseURL   string           // Hub origin for proxy URLs
}

func NewManager(db *database.DB, hub *relay.Hub, sinks []sink.Sink, store *storage.Storage, baseURL string) *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		db:        db,
		hub:       hub,
		sinks:     sinks,
		store:     store,
		baseURL:   baseURL,
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
		// Don't auto-start bots with expired sessions — need manual re-bind
		if b.Status == "session_expired" {
			slog.Info("skip expired bot", "bot", b.ID)
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

// RetryMediaDownload retries downloading media for a failed message.
func (m *Manager) RetryMediaDownload(msgID int64) error {
	msg, err := m.db.GetMessage(msgID)
	if err != nil {
		return err
	}
	var payload map[string]any
	json.Unmarshal(msg.Payload, &payload)

	cdn, ok := payload["media_cdn"].(map[string]any)
	if !ok {
		return fmt.Errorf("no media_cdn in message")
	}
	eqp, _ := cdn["eqp"].(string)
	aes, _ := cdn["aes"].(string)
	mediaType, _ := payload["media_type"].(string)
	if eqp == "" || aes == "" {
		return fmt.Errorf("missing cdn params")
	}

	inst, ok2 := m.GetInstance(msg.BotID)
	if !ok2 {
		return fmt.Errorf("bot not connected")
	}

	// Mark as downloading
	payload["media_status"] = "downloading"
	p, _ := json.Marshal(payload)
	m.db.UpdateMessagePayload(msgID, p)

	go func() {
		item := provider.MessageItem{
			Type: mediaType,
			Media: &provider.Media{
				EncryptQueryParam: eqp,
				AESKey:            aes,
				MediaType:         mediaType,
			},
		}
		fakeMsg := provider.InboundMessage{
			ExternalID: fmt.Sprintf("retry-%d", msgID),
			Items:      []provider.MessageItem{item},
		}
		m.processMedia(inst, &fakeMsg)

		if fakeMsg.Items[0].Media.StorageKey != "" {
			payload["media_key"] = fakeMsg.Items[0].Media.StorageKey
			payload["media_status"] = "ready"
		} else if fakeMsg.Items[0].Media.URL != "" {
			payload["media_status"] = "ready"
		} else {
			payload["media_status"] = "failed"
		}
		updated, _ := json.Marshal(payload)
		m.db.UpdateMessagePayload(msgID, updated)
	}()
	return nil
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

	// Build payload — save immediately, media downloads async
	hasMedia := false
	payloadMap := map[string]any{"content": content}
	if msg.GroupID != "" {
		payloadMap["group_id"] = msg.GroupID
	}
	if msg.ContextToken != "" {
		payloadMap["context_token"] = msg.ContextToken
	}
	for _, item := range msg.Items {
		if item.Media != nil && item.Media.EncryptQueryParam != "" {
			payloadMap["media_cdn"] = map[string]string{
				"eqp": item.Media.EncryptQueryParam,
				"aes": item.Media.AESKey,
			}
			payloadMap["media_type"] = item.Media.MediaType
			payloadMap["media_status"] = "downloading"
			hasMedia = true
			break
		}
	}
	payload, _ := json.Marshal(payloadMap)

	items := make([]relay.MessageItem, len(msg.Items))
	for i, item := range msg.Items {
		items[i] = convertRelayItem(item)
	}

	// Load channels and route
	channels, err := m.db.ListChannelsByBot(inst.DBID)
	if err != nil {
		slog.Error("load channels failed", "bot", inst.DBID, "err", err)
		return
	}

	// Match channels:
	// - No handle → receives ALL messages
	// - Has handle → only receives messages with @handle mention
	var matched []database.Channel
	mentioned := parseMentions(content)
	mentionMatched := make(map[string]bool) // track which @handles already matched

	for _, ch := range channels {
		if ch.Handle == "" {
			// No handle — always receives (if filter matches)
			if matchFilter(ch.FilterRule, msg.Sender, content, msgType) {
				matched = append(matched, ch)
			}
		} else if len(mentioned) > 0 && !mentionMatched[strings.ToLower(ch.Handle)] {
			// Has handle — check if @mentioned (first match only per handle)
			for _, m := range mentioned {
				if strings.EqualFold(m, ch.Handle) {
					matched = append(matched, ch)
					mentionMatched[strings.ToLower(ch.Handle)] = true
					break
				}
			}
		}
	}

	if len(matched) == 0 {
		// No channel matched — store without channel_id
		m.db.SaveMessage(&database.Message{
			BotID: inst.DBID, Direction: "inbound", Sender: msg.Sender,
			Recipient: msg.Recipient, MsgType: msgType, Payload: payload,
		})
		return
	}

	// Deliver to each matched channel
	var savedMsgIDs []int64
	for _, ch := range matched {
		chID := ch.ID
		seqID, _ := m.db.SaveMessage(&database.Message{
			BotID: inst.DBID, ChannelID: &chID, Direction: "inbound",
			Sender: msg.Sender, Recipient: msg.Recipient, MsgType: msgType, Payload: payload,
		})
		savedMsgIDs = append(savedMsgIDs, seqID)
		_ = m.db.UpdateChannelLastSeq(ch.ID, seqID)

		env := relay.NewEnvelope("message", relay.MessageData{
			SeqID: seqID, ExternalID: msg.ExternalID,
			Sender: msg.Sender, Recipient: msg.Recipient, GroupID: msg.GroupID,
			Timestamp: msg.Timestamp, MessageState: msg.MessageState,
			Items: items, ContextToken: msg.ContextToken, SessionID: msg.SessionID,
		})

		d := sink.Delivery{
			BotDBID: inst.DBID, Provider: inst.Provider, Channel: ch,
			Message: msg, Envelope: env, SeqID: seqID, MsgType: msgType, Content: content,
		}
		for _, s := range m.sinks {
			go s.Handle(d)
		}
	}

	// Async media download — update message payloads when done
	if hasMedia {
		go m.downloadAndUpdateMedia(inst, msg, payloadMap, savedMsgIDs)
	}
}

// downloadAndUpdateMedia downloads media files and updates all saved message payloads.
func (m *Manager) downloadAndUpdateMedia(inst *Instance, msg provider.InboundMessage, payloadMap map[string]any, msgIDs []int64) {
	m.processMedia(inst, &msg)

	// Check if any media failed
	allOk := true
	for _, item := range msg.Items {
		if item.Media != nil && item.Media.EncryptQueryParam != "" {
			if item.Media.StorageKey != "" {
				payloadMap["media_key"] = item.Media.StorageKey
			} else if item.Media.URL != "" {
				// CDN proxy fallback
			} else {
				allOk = false
			}
		}
	}
	if allOk {
		payloadMap["media_status"] = "ready"
	} else {
		payloadMap["media_status"] = "failed"
	}

	updated, _ := json.Marshal(payloadMap)
	for _, id := range msgIDs {
		m.db.UpdateMessagePayload(id, updated)
	}
}

// matchFilter checks if a message passes the channel's filter rule.
// processMedia handles media items:
// - With MinIO: download → store → set URL to MinIO
// - Without MinIO: set URL to Hub proxy endpoint
func (m *Manager) processMedia(inst *Instance, msg *provider.InboundMessage) {
	ctx := context.Background()
	for i := range msg.Items {
		item := &msg.Items[i]
		if item.Media == nil || item.Media.EncryptQueryParam == "" {
			continue
		}

		if m.store != nil {
			// Download media — voice gets SILK→WAV conversion
			var data []byte
			var err error
			if item.Type == "voice" {
				sampleRate := 24000
				if item.Media.PlayTime > 0 {
					sampleRate = 24000 // default
				}
				data, err = inst.Provider.DownloadVoice(ctx, item.Media.EncryptQueryParam, item.Media.AESKey, sampleRate)
			} else {
				data, err = inst.Provider.DownloadMedia(ctx, item.Media.EncryptQueryParam, item.Media.AESKey)
			}
			if err != nil {
				slog.Error("media download failed", "bot", inst.DBID, "type", item.Type, "err", err)
				continue
			}
			ext := mediaExt(item.Type)
			ct := mediaContentType(item.Type)
			now := time.Now()
			key := fmt.Sprintf("%s/%d/%02d/%02d/%s_%d%s",
				inst.DBID, now.Year(), now.Month(), now.Day(),
				msg.ExternalID, i, ext)
			url, err := m.store.Put(ctx, key, ct, data)
			if err != nil {
				slog.Error("media store failed", "bot", inst.DBID, "key", key, "err", err)
				continue
			}
			item.Media.URL = url
			item.Media.StorageKey = key
			item.Media.FileSize = int64(len(data))
		} else {
			// Fallback: proxy URL via Hub
			item.Media.URL = fmt.Sprintf("%s/api/v1/channels/media?eqp=%s&aes=%s&ct=%s",
				m.baseURL, item.Media.EncryptQueryParam, item.Media.AESKey,
				mediaContentType(item.Type))
		}
	}
}

func mediaExt(itemType string) string {
	switch itemType {
	case "image":
		return ".jpg"
	case "voice":
		return ".wav"
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
		return "audio/wav"
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
