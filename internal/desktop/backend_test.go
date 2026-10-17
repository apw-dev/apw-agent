package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/store"
)

type fakeAPI struct {
	listErr error
}

func (f *fakeAPI) Plan(context.Context, string, string) (*architect.ExecutionPlan, error) {
	return nil, nil
}
func (f *fakeAPI) Submit(context.Context, string, app.Request) (*store.Job, error) { return nil, nil }
func (f *fakeAPI) Get(context.Context, string, string) (*store.Job, error)         { return nil, nil }
func (f *fakeAPI) List(context.Context, string) ([]store.Job, error)               { return nil, f.listErr }
func (f *fakeAPI) Events(context.Context, string, string, int64) ([]store.Event, error) {
	return nil, nil
}
func (f *fakeAPI) Cancel(context.Context, string, string) error               { return nil }
func (f *fakeAPI) Resume(context.Context, string, string) (*store.Job, error) { return nil, nil }
func (f *fakeAPI) Apply(context.Context, string, string) (*store.Job, error)  { return nil, nil }

func TestBackendCachedAndInvalidatedOnTransportError(t *testing.T) {
	var connects atomic.Int32
	var mu sync.Mutex
	var current *fakeAPI
	srv := NewServer("unused.json", "test")
	srv.connect = func(context.Context) (app.API, error) {
		connects.Add(1)
		mu.Lock()
		defer mu.Unlock()
		current = &fakeAPI{}
		return current, nil
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	list := func() int {
		req, _ := http.NewRequest("GET", ts.URL+"/v1/tasks", nil)
		req.Header.Set("Authorization", "Bearer "+srv.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	for i := 0; i < 5; i++ {
		if code := list(); code != 200 {
			t.Fatalf("status = %d", code)
		}
	}
	if n := connects.Load(); n != 1 {
		t.Fatalf("connects after 5 requests = %d, want 1", n)
	}

	// Ordinary application error: no rebuild.
	mu.Lock()
	current.listErr = fmt.Errorf("service 404: not found")
	mu.Unlock()
	if code := list(); code != 500 {
		t.Fatalf("status = %d, want 500", code)
	}
	if n := connects.Load(); n != 1 {
		t.Fatalf("connects after app error = %d, want 1", n)
	}

	// Transport error: next request reconnects.
	mu.Lock()
	current.listErr = &url.Error{Op: "Get", URL: "http://127.0.0.1:1", Err: syscall.ECONNREFUSED}
	mu.Unlock()
	list()
	if n := connects.Load(); n != 1 {
		t.Fatalf("connects right after failing call = %d, want 1", n)
	}
	if code := list(); code != 200 {
		t.Fatalf("status after reconnect = %d", code)
	}
	if n := connects.Load(); n != 2 {
		t.Fatalf("connects after transport error = %d, want 2", n)
	}
	list()
	if n := connects.Load(); n != 2 {
		t.Fatalf("connects after recovery = %d, want 2", n)
	}
}

func TestBackendConnectFailureNotCached(t *testing.T) {
	var connects atomic.Int32
	srv := NewServer("unused.json", "test")
	srv.connect = func(context.Context) (app.API, error) {
		if connects.Add(1) == 1 {
			return nil, errors.New("daemon down")
		}
		return &fakeAPI{}, nil
	}
	if _, err := srv.backend(context.Background()); err == nil {
		t.Fatal("expected connect error")
	}
	if _, err := srv.backend(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.backend(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := connects.Load(); n != 2 {
		t.Fatalf("connects = %d, want 2", n)
	}
}

func TestIsTransportError(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"nil":      {nil, false},
		"app":      {errors.New("service 404: x"), false},
		"canceled": {&url.Error{Op: "Get", Err: context.Canceled}, false},
		"url":      {&url.Error{Op: "Get", Err: errors.New("boom")}, true},
		"net":      {&net.OpError{Op: "dial", Err: errors.New("x")}, true},
		"refused":  {fmt.Errorf("wrap: %w", syscall.ECONNREFUSED), true},
	}
	for name, c := range cases {
		if got := isTransportError(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", name, got, c.want)
		}
	}
}
