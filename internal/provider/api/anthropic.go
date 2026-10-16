package api

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
	defaultEndpoint   = "https://api.anthropic.com"
	anthropicVersion  = "2023-06-01"
	defaultMaxTokens  = 8192
	defaultMaxRetries = 3
	defaultBaseDelay  = 200 * time.Millisecond
)

// ErrUnauthorized is returned when the API rejects credentials (HTTP 401 or 403).
var ErrUnauthorized = errors.New("api provider: unauthorized")

// Provider implements provider.Provider using the Anthropic Messages API over net/http.
type Provider struct {
	cfg        config.ProviderConfig
	apiKey     string
	endpoint   string
	timeout    time.Duration
	client     *http.Client
	MaxRetries int
	BaseDelay  time.Duration
}

// New creates an Anthropic API Provider from a validated ProviderConfig.
func New(cfg config.ProviderConfig) (*Provider, error) {
	key, err := cfg.APIKey()
	if err != nil {
		return nil, err
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
		apiKey:     key,
		endpoint:   endpoint,
		timeout:    timeout,
		client:     &http.Client{},
		MaxRetries: defaultMaxRetries,
		BaseDelay:  defaultBaseDelay,
	}, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "api"
}

type messagesRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Content    []contentBlock `json:"content"`
	Usage      *apiUsage      `json:"usage,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Ask sends a prompt to the Anthropic Messages API with bounded retries on transient errors.
func (p *Provider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.cfg.Model
	}

	payload, err := json.Marshal(messagesRequest{
		Model:     model,
		MaxTokens: defaultMaxTokens,
		Messages: []apiMessage{
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
		lastErr = p.sanitizeError(err)

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

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.endpoint+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, 0, false, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, false, ctx.Err()
		}
		if isTimeoutOrTemporary(err) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return nil, 0, true, fmt.Errorf("network timeout calling messages endpoint: %w", err)
		}
		return nil, 0, false, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, true, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, 0, false, fmt.Errorf("%w (status %d)", ErrUnauthorized, resp.StatusCode)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, retryAfter, true, fmt.Errorf("transient API status %d", resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, false, fmt.Errorf("unexpected API status %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBytes)))
	}

	var parsed messagesResponse
	if err := json.Unmarshal(rawBytes, &parsed); err != nil {
		return nil, 0, false, fmt.Errorf("decode API response: %w", err)
	}

	var textBuilder strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			textBuilder.WriteString(block.Text)
		}
	}

	var usage *core.Usage
	if parsed.Usage != nil {
		usage = &core.Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		}
	}

	completed := parsed.StopReason == "end_turn"
	if parsed.StopReason == "max_tokens" {
		completed = false
	}

	return &core.ModelResponse{
		RequestID:     requestID,
		Provider:      p.Name(),
		ModelObserved: parsed.Model,
		Content:       textBuilder.String(),
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

func isTimeoutOrTemporary(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func (p *Provider) sanitizeError(err error) error {
	if err == nil || p.apiKey == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, p.apiKey) {
		return err
	}
	redacted := strings.ReplaceAll(msg, p.apiKey, "[REDACTED]")
	if errors.Is(err, ErrUnauthorized) {
		return fmt.Errorf("%w: %s", ErrUnauthorized, redacted)
	}
	return errors.New(redacted)
}

// Close releases any idle connections held by the underlying HTTP client.
func (p *Provider) Close() error {
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	return nil
}
