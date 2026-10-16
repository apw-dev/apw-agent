package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
)

func newTestProvider(t *testing.T, serverURL string) *Provider {
	t.Helper()
	t.Setenv("APW_TEST_ANTHROPIC_KEY", "sk-ant-test-secret")
	p, err := New(config.ProviderConfig{
		Type:       config.ProviderAPI,
		Endpoint:   serverURL,
		Model:      "claude-sonnet-4-20250514",
		APIKeyEnv:  "APW_TEST_ANTHROPIC_KEY",
		TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("New provider failed: %v", err)
	}
	p.BaseDelay = 5 * time.Millisecond
	return p
}

func TestProvider_Ask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant-test-secret" {
			t.Errorf("missing or wrong x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("wrong anthropic-version: %s", r.Header.Get("anthropic-version"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_01",
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"content": [
				{"type": "thinking", "text": "ignored"},
				{"type": "text", "text": "Hello "},
				{"type": "text", "text": "World"}
			],
			"usage": {
				"input_tokens": 25,
				"output_tokens": 10
			}
		}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "req-1",
		Prompt:    "Say hello",
	})
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if res.Content != "Hello World" {
		t.Errorf("Content = %q, want %q", res.Content, "Hello World")
	}
	if !res.Completed {
		t.Errorf("expected Completed = true")
	}
	if res.ModelObserved != "claude-sonnet-4-20250514" {
		t.Errorf("ModelObserved = %q", res.ModelObserved)
	}
	if res.Usage == nil || res.Usage.InputTokens != 25 || res.Usage.OutputTokens != 10 {
		t.Errorf("unexpected Usage: %+v", res.Usage)
	}
}

func TestProvider_Ask_429ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "end_turn",
			"content": [{"type": "text", "text": "recovered"}]
		}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "req-retry",
		Prompt:    "test retry",
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
	if res.Content != "recovered" {
		t.Errorf("Content = %q, want \"recovered\"", res.Content)
	}
	if res.Usage != nil {
		t.Errorf("expected nil Usage when omitted by server, got %+v", res.Usage)
	}
}

func TestProvider_Ask_401UnauthorizedNoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	_, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "req-401",
		Prompt:    "auth test",
	})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 401 not to retry (1 call), got %d calls", calls.Load())
	}
	if strings.Contains(err.Error(), "sk-ant-test-secret") {
		t.Errorf("error message leaked API key: %v", err)
	}
}

func TestProvider_Ask_MaxTokensIncomplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "claude-sonnet-4-20250514",
			"stop_reason": "max_tokens",
			"content": [{"type": "text", "text": "truncated..."}]
		}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv.URL)
	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "req-trunc",
		Prompt:    "long output",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Completed {
		t.Errorf("expected Completed = false when stop_reason is max_tokens")
	}
}

func TestProvider_Ask_ContextCancel(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer srv.Close()
	defer close(done)

	p := newTestProvider(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := p.Ask(ctx, core.ModelRequest{
		RequestID: "req-cancel",
		Prompt:    "cancel me",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestProvider_Ask_LiveOptIn(t *testing.T) {
	if os.Getenv("APW_LIVE_TEST") != "1" {
		t.Skip("skipping live API test; set APW_LIVE_TEST=1 to enable")
	}

	p, err := New(config.ProviderConfig{
		Type:       config.ProviderAPI,
		Model:      "claude-3-5-haiku-latest",
		APIKeyEnv:  "ANTHROPIC_API_KEY",
		TimeoutSec: 30,
	})
	if err != nil {
		t.Fatalf("live provider init failed: %v", err)
	}

	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "live-1",
		Prompt:    "Reply with the single word OK.",
	})
	if err != nil {
		t.Fatalf("live Ask failed: %v", err)
	}
	if res.Content == "" {
		t.Fatal("expected non-empty live response")
	}
}
