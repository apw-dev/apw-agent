package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
)

const (
	defaultEndpoint   = "http://127.0.0.1:11434"
	defaultMaxRetries = 3
	defaultBaseDelay  = 200 * time.Millisecond
)

// Provider implements provider.Provider using an OpenAI-compatible /v1/chat/completions endpoint.
type Provider struct {
	cfg        config.ProviderConfig
	endpoint   string
	timeout    time.Duration
	client     *http.Client
	MaxRetries int
	BaseDelay  time.Duration
}

// New creates a Local Provider from a validated ProviderConfig.
func New(cfg config.ProviderConfig) (*Provider, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("local provider: model is required")
	}

	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	return &Provider{
		cfg:        cfg,
		endpoint:   endpoint,
		timeout:    timeout,
		client:     &http.Client{},
		MaxRetries: defaultMaxRetries,
		BaseDelay:  defaultBaseDelay,
	}, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "local"
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   *chatUsage   `json:"usage,omitempty"`
}

type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Ask sends a prompt to the local OpenAI-compatible chat completions endpoint.
func (p *Provider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.cfg.Model
	}

	payload, err := json.Marshal(chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	maxRetries := p.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		res, retryDelay, shouldRetry, err := p.doOnce(ctx, req.RequestID, payload)
		if err == nil {
			return res, nil
		}
		lastErr = err

		if !shouldRetry || attempt == maxRetries {
			return nil, lastErr
		}

		if retryDelay <= 0 {
			retryDelay = p.backoffDelay(attempt)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	return nil, lastErr
}

func (p *Provider) doOnce(ctx context.Context, requestID string, payload []byte) (*core.ModelResponse, time.Duration, bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.endpoint+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, 0, false, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, false, ctx.Err()
		}
		if isTimeout(err) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return nil, 0, true, fmt.Errorf("timeout calling local endpoint: %w", err)
		}
		return nil, 0, false, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, true, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, retryAfter, true, fmt.Errorf("transient local status %d", resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, false, fmt.Errorf("unexpected local status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBytes)))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(rawBytes, &parsed); err != nil {
		return nil, 0, false, fmt.Errorf("decode local response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, 0, false, errors.New("local provider: response contained no choices")
	}

	choice := parsed.Choices[0]
	completed := choice.FinishReason != "length"

	var usage *core.Usage
	if parsed.Usage != nil {
		usage = &core.Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		}
	}

	return &core.ModelResponse{
		RequestID:     requestID,
		Provider:      p.Name(),
		ModelObserved: parsed.Model,
		Content:       choice.Message.Content,
		Completed:     completed,
		Usage:         usage,
		RawResponse:   string(rawBytes),
		Timestamp:     time.Now(),
	}, 0, false, nil
}

func (p *Provider) backoffDelay(attempt int) time.Duration {
	base := p.BaseDelay
	if base <= 0 {
		base = defaultBaseDelay
	}
	delay := base * (1 << attempt)
	jitter := time.Duration(rand.Int64N(int64(base)/2 + 1))
	return delay + jitter
}

func parseRetryAfter(val string) time.Duration {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	if secs, err := strconv.Atoi(val); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

// Close releases any idle connections held by the underlying HTTP client.
func (p *Provider) Close() error {
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	return nil
}
