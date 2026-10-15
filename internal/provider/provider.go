package provider

import (
	"context"
	"github.com/apiwat/apw-agent/internal/core"
)

// Provider is the common interface for model backends (API, Local, Browser/Argon)
type Provider interface {
	Name() string
	Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error)
	Close() error
}
