package ilink

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	ilink "github.com/openilink/openilink-sdk-go"
	"github.com/openilink/openilink-hub/internal/provider"
)

func init() {
	provider.Register("ilink", func() provider.Provider {
		return &Provider{}
	})
}

// Credentials stored as JSONB in bots.credentials.
type Credentials struct {
	BotID       string `json:"bot_id"`
	BotToken    string `json:"bot_token"`
	BaseURL     string `json:"base_url,omitempty"`
	ILinkUserID string `json:"ilink_user_id,omitempty"`
}

type syncState struct {
	SyncBuf string `json:"sync_buf"`
}

type Provider struct {
	client *ilink.Client
	creds  Credentials
	cancel context.CancelFunc
	status atomic.Value
	mu     sync.Mutex
}

func (p *Provider) Name() string { return "ilink" }

func (p *Provider) Status() string {
	v := p.status.Load()
	if v == nil {
		return "disconnected"
	}
	return v.(string)
}

func (p *Provider) Start(ctx context.Context, opts provider.StartOptions) error {
	var creds Credentials
	if err := json.Unmarshal(opts.Credentials, &creds); err != nil {
		return err
	}
	p.creds = creds

	clientOpts := []ilink.Option{}
	if creds.BaseURL != "" {
		clientOpts = append(clientOpts, ilink.WithBaseURL(creds.BaseURL))
	}
	p.client = ilink.NewClient(creds.BotToken, clientOpts...)

	var ss syncState
	if opts.SyncState != nil {
		json.Unmarshal(opts.SyncState, &ss)
	}

	ctx, p.cancel = context.WithCancel(ctx)
	p.status.Store("connected")
	if opts.OnStatus != nil {
		opts.OnStatus("connected")
	}

	go func() {
		err := p.client.Monitor(ctx, func(msg ilink.WeixinMessage) {
			if opts.OnMessage != nil {
				opts.OnMessage(convertInbound(msg))
			}
		}, &ilink.MonitorOptions{
			InitialBuf: ss.SyncBuf,
			OnBufUpdate: func(buf string) {
				if opts.OnSyncUpdate != nil {
					data, _ := json.Marshal(syncState{SyncBuf: buf})
					opts.OnSyncUpdate(data)
				}
			},
			OnError: func(err error) {
				slog.Warn("ilink monitor error", "err", err)
			},
			OnSessionExpired: func() {
				slog.Error("ilink session expired")
				p.status.Store("session_expired")
				if opts.OnStatus != nil {
					opts.OnStatus("session_expired")
				}
			},
		})

		var newStatus string
		if err != nil && err != context.Canceled {
			slog.Error("ilink monitor stopped", "err", err)
			newStatus = "error"
		} else {
			newStatus = "disconnected"
		}
		p.status.Store(newStatus)
		if opts.OnStatus != nil {
			opts.OnStatus(newStatus)
		}
	}()

	return nil
}

func (p *Provider) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *Provider) Send(ctx context.Context, msg provider.OutboundMessage) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	recipient := msg.Recipient
	if recipient == "" {
		recipient = p.creds.ILinkUserID
	}
	return p.client.Push(ctx, recipient, msg.Text)
}

func convertInbound(msg ilink.WeixinMessage) provider.InboundMessage {
	var items []provider.MessageItem
	for _, item := range msg.ItemList {
		mi := convertItem(item)
		if mi != nil {
			items = append(items, *mi)
		}
	}

	return provider.InboundMessage{
		ExternalID:   fmt.Sprintf("%d", msg.MessageID),
		Sender:       msg.FromUserID,
		Recipient:    msg.ToUserID,
		GroupID:      msg.GroupID,
		Timestamp:    msg.CreateTimeMs,
		MessageState: int(msg.MessageState),
		Items:        items,
		ContextToken: msg.ContextToken,
		SessionID:    msg.SessionID,
	}
}

func convertItem(item ilink.MessageItem) *provider.MessageItem {
	mi := &provider.MessageItem{}

	switch item.Type {
	case ilink.ItemText:
		if item.TextItem == nil {
			return nil
		}
		mi.Type = "text"
		mi.Text = item.TextItem.Text

	case ilink.ItemImage:
		mi.Type = "image"
		if item.ImageItem != nil {
			mi.Media = convertCDNMedia(item.ImageItem.Media, "image")
			if mi.Media != nil {
				if item.ImageItem.URL != "" {
					mi.Media.URL = item.ImageItem.URL
				}
				mi.Media.ThumbWidth = item.ImageItem.ThumbWidth
				mi.Media.ThumbHeight = item.ImageItem.ThumbHeight
			}
		}

	case ilink.ItemVoice:
		mi.Type = "voice"
		if item.VoiceItem != nil {
			mi.Text = item.VoiceItem.Text
			mi.Media = convertCDNMedia(item.VoiceItem.Media, "voice")
			if mi.Media != nil {
				mi.Media.PlayTime = item.VoiceItem.PlayTime
			}
		}

	case ilink.ItemFile:
		mi.Type = "file"
		if item.FileItem != nil {
			mi.FileName = item.FileItem.FileName
			mi.Media = convertCDNMedia(item.FileItem.Media, "file")
		}

	case ilink.ItemVideo:
		mi.Type = "video"
		if item.VideoItem != nil {
			mi.Media = convertCDNMedia(item.VideoItem.Media, "video")
			if mi.Media != nil {
				mi.Media.FileSize = item.VideoItem.VideoSize
				mi.Media.PlayLength = item.VideoItem.PlayLength
				mi.Media.ThumbWidth = item.VideoItem.ThumbWidth
				mi.Media.ThumbHeight = item.VideoItem.ThumbHeight
			}
		}

	default:
		return nil
	}

	// Convert referenced/quoted message
	if item.RefMsg != nil && item.RefMsg.MessageItem != nil {
		refItem := convertItem(*item.RefMsg.MessageItem)
		if refItem != nil {
			mi.RefMsg = &provider.RefMsg{
				Title: item.RefMsg.Title,
				Item:  *refItem,
			}
		}
	}

	return mi
}

func convertCDNMedia(m *ilink.CDNMedia, mediaType string) *provider.Media {
	if m == nil {
		return nil
	}
	return &provider.Media{
		AESKey:    m.AESKey,
		MediaType: mediaType,
	}
}
