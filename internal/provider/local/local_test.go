package local

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
)

func newTestLocalProvider(t *testing.T, serverURL string) *Provider {
	t.Helper()
	p, err := New(config.ProviderConfig{
		Type:       config.ProviderLocal,
		Endpoint:   serverURL,
		Model:      "qwen2.5-coder:7b",
		TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("New local provider failed: %v", err)
	}
	p.BaseDelay = 5 * time.Millisecond
	return p
}

func TestProvider_Ask_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "qwen2.5-coder:7b",
			"choices": [
				{
					"message": {"role": "assistant", "content": "package main"},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 18,
				"completion_tokens": 4
			}
		}`))
	}))
	defer srv.Close()

	p := newTestLocalProvider(t, srv.URL)
	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "req-local-1",
		Prompt:    "Write package main",
	})
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if res.Content != "package main" {
		t.Errorf("Content = %q, want %q", res.Content, "package main")
	}
	if !res.Completed {
		t.Errorf("expected Completed = true")
	}
	if res.ModelObserved != "qwen2.5-coder:7b" {
		t.Errorf("ModelObserved = %q", res.ModelObserved)
	}
	if res.Usage == nil || res.Usage.InputTokens != 18 || res.Usage.OutputTokens != 4 {
		t.Errorf("unexpected Usage: %+v", res.Usage)
	}
}

func TestProvider_Ask_503ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "qwen2.5-coder:7b",
			"choices": [{"message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}]
		}`))
	}))
	defer srv.Close()

	p := newTestLocalProvider(t, srv.URL)
	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "req-retry",
		Prompt:    "ping",
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if calls.Load() != 2 || res.Content != "ok" {
		t.Errorf("calls = %d, content = %q", calls.Load(), res.Content)
	}
}

func TestProvider_Ask_LengthIncomplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "qwen2.5-coder:7b",
			"choices": [{"message": {"role": "assistant", "content": "partial"}, "finish_reason": "length"}]
		}`))
	}))
	defer srv.Close()

	p := newTestLocalProvider(t, srv.URL)
	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "req-len",
		Prompt:    "long",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Completed {
		t.Errorf("expected Completed = false when finish_reason == length")
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

	p := newTestLocalProvider(t, srv.URL)
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
		t.Skip("skipping live local provider test; set APW_LIVE_TEST=1 to enable")
	}

	p, err := New(config.ProviderConfig{
		Type:       config.ProviderLocal,
		Model:      "qwen2.5-coder:7b",
		TimeoutSec: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Ask(context.Background(), core.ModelRequest{
		RequestID: "live-local-1",
		Prompt:    "Reply with OK",
	})
	if err != nil {
		t.Fatalf("live local Ask failed: %v", err)
	}
	if res.Content == "" {
		t.Fatal("expected non-empty live local response")
	}
}

func TestProviderRequiresExplicitCompletedResponse(t *testing.T) {
	for _, reason := range []string{"", "length", "tool_calls", "function_call", "content_filter", "stop"} {
		t.Run(reason, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "partial"}, "finish_reason": reason}}})
			}))
			defer server.Close()
			provider := newTestLocalProvider(t, server.URL)
			response, err := provider.Ask(context.Background(), core.ModelRequest{Prompt: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if response.Completed != (reason == "stop") {
				t.Fatalf("finish_reason=%q was accepted as completed", reason)
			}
		})
	}
}
