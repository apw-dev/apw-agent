package desktop

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/transport"
)

// backendState is one connected backend plus the /v1/ handler built on top of it.
type backendState struct {
	api     app.API
	handler http.Handler
}

// backendCache holds the connected backend so that polling does not reload the
// config, re-read daemon state and rebuild the transport mux on every request.
type backendCache struct {
	mu    sync.Mutex
	state *backendState
}

func defaultConnect(configPath string) func(context.Context) (app.API, error) {
	return func(ctx context.Context) (app.API, error) {
		c, err := daemon.Connect(ctx, configPath)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

// current returns the cached state, building it on first use. The lock is held
// during connect so concurrent first requests share a single connection attempt.
func (s *Server) current(ctx context.Context) (*backendState, error) {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	if st := s.cache.state; st != nil {
		return st, nil
	}
	st := &backendState{}
	if s.Backend != nil {
		st.api = s.Backend
	} else {
		connect := s.connect
		if connect == nil {
			connect = defaultConnect(s.ConfigPath)
		}
		api, err := connect(ctx)
		if err != nil {
			return nil, err
		}
		st.api = &invalidatingAPI{API: api, server: s, state: st}
	}
	st.handler = transport.HandlerWith(st.api, s.Token, map[string]string{"protocol": transport.Version}, nil, s.allowRequest)
	s.cache.state = st
	return st, nil
}

// invalidate drops st from the cache (only if it is still the current one) so
// the next request reconnects.
func (s *Server) invalidate(st *backendState) {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	if s.cache.state == st {
		s.cache.state = nil
	}
}

func (s *Server) backend(ctx context.Context) (app.API, error) {
	st, err := s.current(ctx)
	if err != nil {
		return nil, err
	}
	return st.api, nil
}

func isTransportError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var urlErr *url.Error
	var netErr net.Error
	return errors.As(err, &urlErr) || errors.As(err, &netErr) || errors.Is(err, syscall.ECONNREFUSED)
}

// invalidatingAPI forwards every app.API call and resets the server's cached
// backend when the daemon turns out to be unreachable.
type invalidatingAPI struct {
	app.API
	server *Server
	state  *backendState
}

func (a *invalidatingAPI) check(err error) error {
	if isTransportError(err) {
		a.server.invalidate(a.state)
	}
	return err
}

func (a *invalidatingAPI) Plan(ctx context.Context, owner, instruction string) (*architect.ExecutionPlan, error) {
	p, err := a.API.Plan(ctx, owner, instruction)
	return p, a.check(err)
}

func (a *invalidatingAPI) Submit(ctx context.Context, owner string, r app.Request) (*store.Job, error) {
	j, err := a.API.Submit(ctx, owner, r)
	return j, a.check(err)
}

func (a *invalidatingAPI) Get(ctx context.Context, id, owner string) (*store.Job, error) {
	j, err := a.API.Get(ctx, id, owner)
	return j, a.check(err)
}

func (a *invalidatingAPI) List(ctx context.Context, owner string) ([]store.Job, error) {
	j, err := a.API.List(ctx, owner)
	return j, a.check(err)
}

func (a *invalidatingAPI) Events(ctx context.Context, id, owner string, after int64) ([]store.Event, error) {
	e, err := a.API.Events(ctx, id, owner, after)
	return e, a.check(err)
}

func (a *invalidatingAPI) Cancel(ctx context.Context, id, owner string) error {
	return a.check(a.API.Cancel(ctx, id, owner))
}

func (a *invalidatingAPI) Resume(ctx context.Context, id, owner string) (*store.Job, error) {
	j, err := a.API.Resume(ctx, id, owner)
	return j, a.check(err)
}

func (a *invalidatingAPI) Apply(ctx context.Context, id, owner string) (*store.Job, error) {
	j, err := a.API.Apply(ctx, id, owner)
	return j, a.check(err)
}
