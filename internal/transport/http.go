package transport

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/store"
)

const Version = "1"

func LocalRequest(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "http" || u.Host != r.Host {
			return false
		}
	}
	return true
}

func Guard(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !LocalRequest(r) {
			http.Error(w, "local origin required", http.StatusForbidden)
			return
		}
		given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
				http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		}
		next.ServeHTTP(w, r)
	})
}
func Decode(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON content")
	}
	return nil
}
func WriteJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func Handler(backend app.API, token string, health any, stop func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/plan", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Instruction string `json:"instruction"`
		}
		if err := Decode(r, &request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		plan, err := backend.Plan(r.Context(), "local", request.Instruction)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		WriteJSON(w, plan)
	})
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, health) })
	mux.HandleFunc("POST /v1/shutdown", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, map[string]bool{"ok": true})
		if stop != nil {
			go stop()
		}
	})
	mux.HandleFunc("POST /v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		var request app.Request
		if err := Decode(r, &request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		job, err := backend.Submit(r.Context(), "local", request)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		WriteJSON(w, job)
	})
	mux.HandleFunc("GET /v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := backend.List(r.Context(), "local")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		WriteJSON(w, jobs)
	})
	mux.HandleFunc("GET /v1/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, err := backend.Get(r.Context(), r.PathValue("id"), "local")
		if err != nil {
			http.Error(w, "task not found", 404)
			return
		}
		WriteJSON(w, job)
	})
	mux.HandleFunc("GET /v1/tasks/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		if r.URL.Query().Get("after") == "" {
			err = nil
		}
		if err != nil || after < 0 {
			http.Error(w, "invalid cursor", 400)
			return
		}
		events, err := backend.Events(r.Context(), r.PathValue("id"), "local", after)
		if err != nil {
			http.Error(w, "task not found", 404)
			return
		}
		WriteJSON(w, events)
	})
	for _, action := range []string{"cancel", "resume", "apply"} {
		mux.HandleFunc("POST /v1/tasks/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			id := r.PathValue("id")
			var job *store.Job
			var err error
			switch action {
			case "cancel":
				err = backend.Cancel(r.Context(), id, "local")
				if err == nil {
					job, err = backend.Get(r.Context(), id, "local")
				}
			case "resume":
				job, err = backend.Resume(r.Context(), id, "local")
			case "apply":
				job, err = backend.Apply(r.Context(), id, "local")
			}
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			WriteJSON(w, job)
		})
	}
	return Guard(token, mux)
}

type Client struct {
	URL, Token string
	HTTP       *http.Client
}

func (c *Client) Plan(ctx context.Context, owner, instruction string) (*architect.ExecutionPlan, error) {
	if err := localOwner(owner); err != nil {
		return nil, err
	}
	var plan architect.ExecutionPlan
	client := *c
	httpClient := *c.HTTP
	httpClient.Timeout = 20 * time.Minute
	client.HTTP = &httpClient
	err := client.Call(ctx, "POST", "/v1/plan", map[string]string{"instruction": instruction}, &plan)
	return &plan, err
}

func NewClient(address, token string) *Client {
	return &Client{URL: address, Token: token, HTTP: &http.Client{Timeout: 10 * time.Second}}
}
func (c *Client) Call(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("service %d: %s", res.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 8*1024*1024)).Decode(out)
	}
	return nil
}
func localOwner(owner string) error {
	if owner != "local" {
		return errors.New("local RPC cannot impersonate another owner")
	}
	return nil
}
func (c *Client) Submit(ctx context.Context, owner string, r app.Request) (*store.Job, error) {
	if err := localOwner(owner); err != nil {
		return nil, err
	}
	var j store.Job
	err := c.Call(ctx, "POST", "/v1/tasks", r, &j)
	return &j, err
}
func (c *Client) Get(ctx context.Context, id, owner string) (*store.Job, error) {
	if err := localOwner(owner); err != nil {
		return nil, err
	}
	var j store.Job
	err := c.Call(ctx, "GET", "/v1/tasks/"+url.PathEscape(id), nil, &j)
	return &j, err
}
func (c *Client) List(ctx context.Context, owner string) ([]store.Job, error) {
	if err := localOwner(owner); err != nil {
		return nil, err
	}
	var jobs []store.Job
	err := c.Call(ctx, "GET", "/v1/tasks", nil, &jobs)
	return jobs, err
}
func (c *Client) Events(ctx context.Context, id, owner string, after int64) ([]store.Event, error) {
	if err := localOwner(owner); err != nil {
		return nil, err
	}
	var events []store.Event
	err := c.Call(ctx, "GET", "/v1/tasks/"+url.PathEscape(id)+"/events?after="+strconv.FormatInt(after, 10), nil, &events)
	return events, err
}
func (c *Client) operation(ctx context.Context, id, owner, action string) (*store.Job, error) {
	if err := localOwner(owner); err != nil {
		return nil, err
	}
	var j store.Job
	err := c.Call(ctx, "POST", "/v1/tasks/"+url.PathEscape(id)+"/"+action, map[string]string{}, &j)
	return &j, err
}
func (c *Client) Cancel(ctx context.Context, id, owner string) error {
	_, err := c.operation(ctx, id, owner, "cancel")
	return err
}
func (c *Client) Resume(ctx context.Context, id, owner string) (*store.Job, error) {
	return c.operation(ctx, id, owner, "resume")
}
func (c *Client) Apply(ctx context.Context, id, owner string) (*store.Job, error) {
	return c.operation(ctx, id, owner, "apply")
}

var _ app.API = (*Client)(nil)
