package desktop

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/store"
)

func stubDiscovery(t *testing.T, keys ...string) *int {
	t.Helper()
	calls := 0
	prev := discoverModels
	discoverModels = func(_ string, cfg *config.Config) {
		calls++
		for _, k := range keys {
			cfg.Providers[k] = config.ProviderConfig{Type: config.ProviderAGY, Model: strings.TrimPrefix(k, "agy:")}
		}
	}
	t.Cleanup(func() { discoverModels = prev })
	return &calls
}

func TestDiscoverProvidersOnFirstLaunchAddsModelsAndKeepsRoles(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	stubDiscovery(t, "agy:gemini-3.8-flash-high")
	cfgPath := filepath.Join(t.TempDir(), "apw.json") // no file yet: a first launch
	added, err := DiscoverProviders(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0] != "agy:gemini-3.8-flash-high" {
		t.Fatalf("added = %v", added)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Providers["agy:gemini-3.8-flash-high"]; !ok {
		t.Fatalf("discovered model not saved: %v", cfg.Providers)
	}
	if cfg.Roles.Architect != config.BuiltinProvider || cfg.Roles.Reflex != config.BuiltinProvider {
		t.Fatalf("discovery must leave roles for the user: %+v", cfg.Roles)
	}
}

func TestDiscoverProvidersLeavesConfiguredWorkspaceAlone(t *testing.T) {
	calls := stubDiscovery(t, "agy:new-model")
	cfgPath := filepath.Join(t.TempDir(), "apw.json")
	body := `{"workspace": ".", "providers": {"m": {"type": "agy", "model": "gemini-3.8-flash-high"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if added, err := DiscoverProviders(cfgPath); err != nil || len(added) != 0 || *calls != 0 {
		t.Fatalf("added=%v err=%v calls=%d", added, err, *calls)
	}
	if got, _ := os.ReadFile(cfgPath); string(got) != body {
		t.Fatalf("configured workspace was rewritten:\n%s", got)
	}
}

// noModelBackend rejects submissions the way app.Service does without a connected model.
type noModelBackend struct{ stubBackend }

func (b *noModelBackend) Submit(context.Context, string, app.Request) (*store.Job, error) {
	return nil, fmt.Errorf("%w: the architect and reflex role uses the built-in placeholder", app.ErrNoModelConnected)
}

func TestRunWithoutModelReportsNoModel(t *testing.T) {
	ws := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "apw.json")
	if err := os.WriteFile(cfgPath, []byte(`{"workspace": `+mustQuote(ws)+`, "policy": {"trusted": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	e := serve(t, ws, cfgPath, &noModelBackend{stubBackend{jobs: map[string]*store.Job{}}})
	status, body := e.do("POST", "/api/run", `{"request_id":"r1","instruction":"x","mode":"review"}`)
	if status != http.StatusConflict || !strings.Contains(string(body), `"no_model"`) || !strings.Contains(string(body), "no model is connected") {
		t.Fatalf("status=%d body=%s", status, body)
	}
}
