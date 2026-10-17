package transport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
)

func TestRPCRejectsUnauthenticatedCrossOriginAndInvalidRequests(t *testing.T) {
	cfg := &config.Config{Workspace: t.TempDir(), Providers: map[string]config.ProviderConfig{"mock": {Type: config.ProviderMock}}}
	cfg.Roles.Architect = "mock"
	cfg.Roles.Reflex = "mock"
	backend, err := app.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	srv := httptest.NewServer(Handler(backend, "private-token", map[string]string{"protocol": Version}, nil))
	defer srv.Close()
	for _, tc := range []struct {
		method, path, token, origin, host, body, contentType string
		status                                               int
	}{
		{"GET", "/v1/tasks", "", "", "", "", "", 401},
		{"GET", "/v1/tasks", "private-token", "http://evil.example", "", "", "", 403},
		{"GET", "/v1/tasks", "private-token", "", "evil.example", "", "", 403},
		{"GET", "/v1/tasks/x/apply", "private-token", "", "", "", "", 405},
		{"POST", "/v1/tasks", "private-token", "", "", `{"request_id":"x","instruction":"review","mode":"review"}`, "text/plain", 415},
		{"POST", "/v1/tasks", "private-token", "", "", `{"request_id":"x","instruction":"review","mode":"review","owner":"telegram:111"}`, "application/json", 400},
		{"POST", "/v1/tasks", "private-token", "", "", `{"request_id":"x","instruction":"review","mode":"review"} {}`, "application/json", 400},
	} {
		req, _ := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(tc.body))
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.host != "" {
			req.Host = tc.host
		}
		if tc.contentType != "" {
			req.Header.Set("Content-Type", tc.contentType)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.path, response.StatusCode, tc.status)
		}
	}
	if jobs, err := backend.List(context.Background(), "local"); err != nil || len(jobs) != 0 {
		t.Fatalf("rejected requests created tasks: %+v %v", jobs, err)
	}
}

func TestRPCIdempotencyAndChannelOwnership(t *testing.T) {
	cfg := &config.Config{Workspace: t.TempDir(), Providers: map[string]config.ProviderConfig{"mock": {Type: config.ProviderMock}}}
	cfg.Roles.Architect = "mock"
	cfg.Roles.Reflex = "mock"
	backend, err := app.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	srv := httptest.NewServer(Handler(backend, "private-token", nil, nil))
	defer srv.Close()
	client := NewClient(srv.URL, "private-token")
	request := app.Request{RequestID: "same", Instruction: "review", Mode: core.ModeReview}
	a, err := client.Submit(context.Background(), "local", request)
	if err != nil {
		t.Fatal(err)
	}
	b, err := client.Submit(context.Background(), "local", request)
	if err != nil || a.ID != b.ID {
		t.Fatalf("RPC retry duplicated work: %+v %v", b, err)
	}
	private, err := backend.Submit(context.Background(), "telegram:111", request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Get(context.Background(), private.ID, "local"); err == nil {
		t.Fatal("local channel read another owner's job")
	}
	if _, err = client.Get(context.Background(), private.ID, "telegram:111"); err == nil {
		t.Fatal("RPC client impersonated another channel")
	}
}
