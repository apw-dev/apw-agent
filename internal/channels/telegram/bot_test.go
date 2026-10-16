package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestBot_OffsetAllowlistAndShutdown(t *testing.T) {
	var (
		mu          sync.Mutex
		seenOffsets []string
		sentReplies []sendMessageRequest
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bottest-token/getUpdates":
			offset := r.URL.Query().Get("offset")
			mu.Lock()
			seenOffsets = append(seenOffsets, offset)
			callNum := len(seenOffsets)
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			if callNum == 1 {
				// Return 3 updates:
				// 100: unauthorized user (999) -> must be ignored silently, but offset still advances
				// 101: authorized user (111) sends "/status" -> replies "apw 0.1.0-alpha ok"
				// 102: authorized user (222) sends "hello world" -> echoes "hello world"
				_, _ = w.Write([]byte(`{
					"ok": true,
					"result": [
						{
							"update_id": 100,
							"message": {"message_id": 1, "from": {"id": 999}, "chat": {"id": 999}, "text": "/status"}
						},
						{
							"update_id": 101,
							"message": {"message_id": 2, "from": {"id": 111}, "chat": {"id": 111}, "text": "/status"}
						},
						{
							"update_id": 102,
							"message": {"message_id": 3, "from": {"id": 222}, "chat": {"id": 222}, "text": "hello world"}
						}
					]
				}`))
				return
			}

			// Second poll should have offset=103; trigger shutdown
			cancel()
			select {
			case <-r.Context().Done():
			case <-done:
			}

		case "/bottest-token/sendMessage":
			var req sendMessageRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			sentReplies = append(sentReplies, req)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok": true}`))

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	defer close(done)

	bot, err := New("test-token", []int64{111, 222}, "0.1.0-alpha")
	if err != nil {
		t.Fatalf("New bot failed: %v", err)
	}
	bot.BaseURL = srv.URL
	bot.PollTimeout = 1

	err = bot.Run(ctx)
	if err != nil {
		t.Fatalf("bot.Run returned error on shutdown: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(seenOffsets) < 2 {
		t.Fatalf("expected at least 2 getUpdates calls, got %d", len(seenOffsets))
	}
	if seenOffsets[0] != "0" {
		t.Errorf("first offset = %q, want \"0\"", seenOffsets[0])
	}
	if seenOffsets[1] != "103" {
		t.Errorf("second offset = %q, want \"103\"", seenOffsets[1])
	}

	if len(sentReplies) != 2 {
		t.Fatalf("expected 2 replies (unauthorized user 999 dropped), got %d: %+v", len(sentReplies), sentReplies)
	}
	if sentReplies[0].ChatID != 111 || sentReplies[0].Text != "apw 0.1.0-alpha ok" {
		t.Errorf("unexpected first reply: %+v", sentReplies[0])
	}
	if sentReplies[1].ChatID != 222 || sentReplies[1].Text != "hello world" {
		t.Errorf("unexpected second reply: %+v", sentReplies[1])
	}
}

func TestBot_LiveOptIn(t *testing.T) {
	if os.Getenv("APW_LIVE_TEST") != "1" {
		t.Skip("skipping live Telegram test; set APW_LIVE_TEST=1 to enable")
	}
	if os.Getenv("APW_TELEGRAM_TOKEN") == "" || os.Getenv("APW_TELEGRAM_ALLOW") == "" {
		t.Skip("skipping live Telegram test; APW_TELEGRAM_TOKEN or APW_TELEGRAM_ALLOW not set")
	}

	bot, err := NewFromEnv("0.1.0-alpha")
	if err != nil {
		t.Fatal(err)
	}
	bot.PollTimeout = 1

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := bot.Run(ctx); err != nil {
		t.Fatalf("live bot run failed: %v", err)
	}
}
