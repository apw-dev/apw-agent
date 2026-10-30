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
	defaultEndpoint      = "https://api.anthropic.com"
	anthropicVersion     = "2023-06-01"
	defaultMaxTokens     = 16384
	defaultMaxRetries    = 3
	defaultBaseDelay     = 200 * time.Millisecond
	cacheBreakpointBytes = 4096
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

	var timeout time.Duration
	if cfg.TimeoutSec > 0 && cfg.TimeoutSec != 60 && cfg.TimeoutSec != 120 && cfg.TimeoutSec != 180 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
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

type cacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type textBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type outputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type messagesRequest struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	System       []textBlock     `json:"system,omitempty"`
	Messages     []apiMessage    `json:"messages"`
	Thinking     *thinkingConfig `json:"thinking,omitempty"`
	OutputConfig *outputConfig   `json:"output_config,omitempty"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
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
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

func makeCacheControl(opts map[string]string) *cacheControl {
	if opts != nil && opts["cache"] == "off" {
		return nil
	}
	cc := &cacheControl{Type: "ephemeral"}
	if opts != nil {
		if ttl := strings.TrimSpace(opts["cache_ttl"]); ttl == "5m" || ttl == "1h" {
			cc.TTL = ttl
		}
	}
	return cc
}

func splitCachePrefix(req core.ModelRequest) (string, string) {
	if req.CachePrefix != "" && req.Prompt == "" {
		return "", ""
	}
	if req.CachePrefix != "" && req.Prompt != "" {
		if strings.HasPrefix(req.Prompt, req.CachePrefix) {
			rest := strings.TrimPrefix(req.Prompt[len(req.CachePrefix):], "\n")
			if rest != "" {
				return req.CachePrefix, rest
			}
			return "", ""
		}
		return req.CachePrefix, req.Prompt
	}
	return "", ""
}

func buildMessagesRequest(model string, req core.ModelRequest) messagesRequest {
	maxTokens := defaultMaxTokens
	if req.Options != nil {
		if mt, err := strconv.Atoi(strings.TrimSpace(req.Options["max_tokens"])); err == nil && mt > 0 {
			maxTokens = mt
		}
	}

	out := messagesRequest{
		Model:     model,
		MaxTokens: maxTokens,
	}

	cc := makeCacheControl(req.Options)

	if sys := strings.TrimSpace(req.System); sys != "" {
		sb := textBlock{Type: "text", Text: sys}
		if len(sys) >= 1024 && cc != nil {
			sb.CacheControl = cc
		}
		out.System = []textBlock{sb}
	}

	prefix, rest := splitCachePrefix(req)
	switch {
	case prefix != "" && rest != "":
		var prefixCC *cacheControl
		if len(prefix) >= 1024 {
			prefixCC = cc
		}
		out.Messages = []apiMessage{
			{
				Role: "user",
				Content: []textBlock{
					{Type: "text", Text: prefix, CacheControl: prefixCC},
					{Type: "text", Text: rest},
				},
			},
		}
	default:
		full := req.EffectivePrompt()
		if len(full) >= cacheBreakpointBytes && cc != nil {
			out.Messages = []apiMessage{
				{
					Role: "user",
					Content: []textBlock{
						{Type: "text", Text: full, CacheControl: cc},
					},
				},
			}
		} else {
			out.Messages = []apiMessage{
				{
					Role:    "user",
					Content: full,
				},
			}
		}
	}

	if req.Options != nil {
		if effort := strings.TrimSpace(req.Options["effort"]); effort != "" {
			out.OutputConfig = &outputConfig{Effort: effort}
		}
		switch th := strings.TrimSpace(req.Options["thinking"]); th {
		case "adaptive":
			out.Thinking = &thinkingConfig{Type: "adaptive"}
		case "disabled":
			out.Thinking = &thinkingConfig{Type: "disabled"}
		default:
			if budget, err := strconv.Atoi(th); err == nil && budget >= 1024 {
				out.Thinking = &thinkingConfig{Type: "enabled", BudgetTokens: budget}
				if out.MaxTokens <= budget {
					out.MaxTokens = budget + defaultMaxTokens
				}
			}
		}
	}

	return out
}

// Ask sends a prompt to the Anthropic Messages API with bounded retries on transient errors.
func (p *Provider) Ask(ctx context.Context, req core.ModelRequest) (*core.ModelResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.cfg.Model
	}

	payload, err := json.Marshal(buildMessagesRequest(model, req))
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

		if !shouldRetry || attempt == maxRetries || retryDelay > 15*time.Second {
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
	reqCtx := ctx
	var cancel context.CancelFunc = func() {}
	if p.timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, p.timeout)
	}
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

	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if len(rawBytes) > 2*1024*1024 {
		return nil, 0, false, errors.New("provider response exceeds size limit")
	}
	if err != nil {
		return nil, 0, true, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, 0, false, fmt.Errorf("%w (status %d)", ErrUnauthorized, resp.StatusCode)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		retryAfter := parseRateLimitWait(resp.Header)
		detail := extractAPIErrorMessage(rawBytes)
		if resp.StatusCode == http.StatusTooManyRequests {
			resetHint := ""
			if retryAfter > 0 {
				resetHint = fmt.Sprintf(" (resets in %s)", retryAfter.Round(time.Second))
			}
			if detail != "" {
				return nil, retryAfter, true, fmt.Errorf("transient API status 429 rate limit / quota exceeded%s: %s", resetHint, detail)
			}
			return nil, retryAfter, true, fmt.Errorf("transient API status 429 rate limit / quota exceeded%s", resetHint)
		}
		if detail != "" {
			return nil, retryAfter, true, fmt.Errorf("transient API status %d: %s", resp.StatusCode, detail)
		}
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
			InputTokens:              parsed.Usage.InputTokens,
			OutputTokens:             parsed.Usage.OutputTokens,
			CacheCreationInputTokens: parsed.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     parsed.Usage.CacheReadInputTokens,
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
	if ts, err := http.ParseTime(val); err == nil {
		if d := time.Until(ts); d > 0 {
			return d
		}
	}
	return 0
}

// parseRateLimitWait inspects Retry-After and Anthropic's unified/standard rate-limit reset headers
// (as observed in Claude Code API traffic: anthropic-ratelimit-unified-5h-reset, 7d-reset, and
// anthropic-ratelimit-*-reset) to compute an accurate cooldown window.
func parseRateLimitWait(h http.Header) time.Duration {
	if h == nil {
		return 0
	}
	if raw := strings.TrimSpace(h.Get("Retry-After")); raw != "" {
		return parseRetryAfter(raw)
	}
	now := time.Now()
	for _, key := range []string{
		"Anthropic-Ratelimit-Unified-5h-Reset",
		"Anthropic-Ratelimit-Unified-7d-Reset",
		"Anthropic-Ratelimit-Tokens-Reset",
		"Anthropic-Ratelimit-Requests-Reset",
	} {
		val := strings.TrimSpace(h.Get(key))
		if val == "" {
			continue
		}
		if unixSec, err := strconv.ParseInt(val, 10, 64); err == nil && unixSec > now.Unix() {
			return time.Until(time.Unix(unixSec, 0))
		}
		if ts, err := time.Parse(time.RFC3339, val); err == nil && ts.After(now) {
			return time.Until(ts)
		}
	}
	return 0
}

func extractAPIErrorMessage(raw []byte) string {
	var env struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err == nil && strings.TrimSpace(env.Error.Message) != "" {
		if env.Error.Type != "" {
			return env.Error.Type + ": " + strings.TrimSpace(env.Error.Message)
		}
		return strings.TrimSpace(env.Error.Message)
	}
	return ""
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
