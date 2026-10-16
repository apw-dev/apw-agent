package mock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/core"
)

// ErrNoMoreResponses is returned when the mock provider runs out of queued responses.
var ErrNoMoreResponses = errors.New("mock provider: no more responses queued")

// Provider is a deterministic in-memory model provider for testing.
type Provider struct {
	mu        sync.Mutex
	idx       int
	Responses []string            // returned in order; returns ErrNoMoreResponses when exhausted
	Requests  []core.ModelRequest // records every request received
}

// New creates a mock Provider with the given canned responses.
func New(responses ...string) *Provider {
	return &Provider{
		Responses: responses,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "mock"
}

// Ask records the request and returns the next queued response.
func (p *Provider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.Requests = append(p.Requests, req)
	if p.idx >= len(p.Responses) {
		return nil, ErrNoMoreResponses
	}
	content := p.Responses[p.idx]
	p.idx++

	modelObserved := req.Model
	if modelObserved == "" {
		modelObserved = "mock-model"
	}

	return &core.ModelResponse{
		RequestID:     req.RequestID,
		Provider:      p.Name(),
		ModelObserved: modelObserved,
		Content:       content,
		Completed:     true,
		Timestamp:     time.Now(),
	}, nil
}

// Close is a no-op for the mock provider.
func (p *Provider) Close() error {
	return nil
}
