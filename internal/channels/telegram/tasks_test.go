package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/store"
)

func TestDurableInboxOwnerBindingAndDeliveryRetry(t *testing.T) {
	cfg := &config.Config{Workspace: t.TempDir(), Providers: map[string]config.ProviderConfig{"mock": {Type: config.ProviderMock}}}
	cfg.Roles.Architect = "mock"
	cfg.Roles.Reflex = "mock"
	backend, err := app.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	bot, err := New("test-token", []int64{111, 222}, "test")
	if err != nil {
		t.Fatal(err)
	}
	bot.Backend = backend
	bot.Store = backend.Store
	u := update{UpdateID: 10, Message: &message{From: &user{ID: 111}, Chat: chat{ID: 111}, Text: "/review inspect source"}}
	if err = bot.processUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if err = bot.processUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	jobs, err := backend.List(context.Background(), "telegram:111")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("duplicate update created jobs: %+v %v", jobs, err)
	}
	group := update{UpdateID: 11, Message: &message{From: &user{ID: 111}, Chat: chat{ID: -999}, Text: "/review forbidden group"}}
	if err = bot.processUpdate(context.Background(), group); err != nil {
		t.Fatal(err)
	}
	jobs, _ = backend.List(context.Background(), "telegram:111")
	if len(jobs) != 1 {
		t.Fatal("group message bypassed private owner binding")
	}
	if _, err = backend.Get(context.Background(), jobs[0].ID, "telegram:222"); err == nil {
		t.Fatal("another user can access the job")
	}
	offset, err := bot.Store.TelegramOffset(context.Background(), bot.accountID())
	if err != nil || offset != 12 {
		t.Fatalf("offset not durable: %d %v", offset, err)
	}
	fresh, _ := New("test-token", []int64{111, 222}, "test")
	fresh.Backend = backend
	fresh.Store = backend.Store
	if err = fresh.processUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	jobs, _ = backend.List(context.Background(), "telegram:111")
	if len(jobs) != 1 {
		t.Fatal("reconnect replayed the command")
	}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var message sendMessageRequest
		_ = json.NewDecoder(r.Body).Decode(&message)
		if message.ChatID != 111 {
			t.Error("reply routed to another chat")
		}
		if requests == 1 {
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	bot.BaseURL = srv.URL
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j, _ := backend.Get(context.Background(), jobs[0].ID, "telegram:111")
		if j.Status == core.StatusSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = bot.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	pending, err := bot.Store.Deliveries(context.Background(), bot.accountID())
	if err != nil {
		t.Fatal(err)
	}
	// The rate-limited delivery must remain durably queued with a future retry time.
	if len(pending) != 0 {
		t.Fatal("rate-limited delivery retried before retry_after")
	}
	time.Sleep(1100 * time.Millisecond)
	if err = bot.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := requests
	if err = bot.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != after {
		t.Fatal("confirmed deliveries were sent again")
	}
	if after < 2 {
		t.Fatal("failed delivery was lost instead of retried")
	}
}

func TestFlushWatermarkAndDedupe(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	bot, _ := New("wm-token", []int64{111}, "test")
	bot.Store = st
	sent := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	bot.BaseURL = srv.URL
	mk := func(req string, status core.TaskStatus) string {
		j, _, err := st.CreateJob(ctx, store.Job{Task: core.Task{ID: "task-" + req, Workspace: ".", Instruction: "x", Mode: "review"}, Owner: "telegram:111", RequestID: req})
		if err != nil {
			t.Fatal(err)
		}
		if err = st.SetJob(ctx, j.ID, status, nil, ""); err != nil {
			t.Fatal(err)
		}
		return j.ID
	}
	mk("a", core.StatusSucceeded)
	if err = bot.flush(ctx); err != nil {
		t.Fatal(err)
	}
	if sent != 1 || bot.seen[111] == 0 {
		t.Fatalf("sent=%d seen=%v", sent, bot.seen)
	}
	if err = bot.flush(ctx); err != nil || sent != 1 {
		t.Fatalf("watermark did not suppress resend: sent=%d err=%v", sent, err)
	}
	mk("b", core.StatusFailed)
	if err = bot.flush(ctx); err != nil || sent != 2 {
		t.Fatalf("new terminal job not delivered: sent=%d err=%v", sent, err)
	}
	// A restart (fresh watermark) re-scans but dedupe_key prevents resends.
	fresh, _ := New("wm-token", []int64{111}, "test")
	fresh.Store = st
	fresh.BaseURL = srv.URL
	if err = fresh.flush(ctx); err != nil || sent != 2 {
		t.Fatalf("restart re-sent deliveries: sent=%d err=%v", sent, err)
	}
}
