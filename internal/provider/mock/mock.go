package mock

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/openilink/openilink-hub/internal/provider"
)

func init() {
	provider.Register("mock", func() provider.Provider {
		return New()
	})
}

// Provider is a mock provider for testing.
type Provider struct {
	mu        sync.Mutex
	status    string
	onMsg     func(provider.InboundMessage)
	onStatus  func(string)
	sent      []provider.OutboundMessage
}

func New() *Provider {
	return &Provider{status: "disconnected"}
}

func (p *Provider) Name() string  { return "mock" }
func (p *Provider) Status() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

func (p *Provider) Start(_ context.Context, opts provider.StartOptions) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = "connected"
	p.onMsg = opts.OnMessage
	p.onStatus = opts.OnStatus
	return nil
}

func (p *Provider) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = "disconnected"
}

func (p *Provider) Send(_ context.Context, msg provider.OutboundMessage) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, msg)
	return "mock-client-id", nil
}

// SimulateInbound injects a fake inbound message for testing.
func (p *Provider) SimulateInbound(msg provider.InboundMessage) {
	p.mu.Lock()
	cb := p.onMsg
	p.mu.Unlock()
	if cb != nil {
		cb(msg)
	}
}

// SentMessages returns all outbound messages sent through this provider.
func (p *Provider) SentMessages() []provider.OutboundMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]provider.OutboundMessage, len(p.sent))
	copy(out, p.sent)
	return out
}

// Credentials returns mock credentials JSON.
func Credentials() json.RawMessage {
	data, _ := json.Marshal(map[string]string{"mock": "true"})
	return data
}
