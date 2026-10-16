package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "apw.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_DefaultsAndValid(t *testing.T) {
	p := writeTempConfig(t, `{
		"providers": {
			"main": {
				"type": "api",
				"model": "claude-sonnet-4-20250514",
				"api_key_env": "TEST_APW_API_KEY"
			}
		},
		"roles": {
			"architect": "main",
			"reflex": "main"
		}
	}`)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Workspace != "." {
		t.Errorf("Workspace default = %q, want \".\"", cfg.Workspace)
	}
	if cfg.Budget.MaxRepairs != 3 {
		t.Errorf("MaxRepairs default = %d, want 3", cfg.Budget.MaxRepairs)
	}
	if cfg.Budget.TaskTimeoutSec != 600 {
		t.Errorf("TaskTimeoutSec default = %d, want 600", cfg.Budget.TaskTimeoutSec)
	}
	if cfg.Providers["main"].TimeoutSec != 120 {
		t.Errorf("Provider TimeoutSec default = %d, want 120", cfg.Providers["main"].TimeoutSec)
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantSub string
	}{
		{
			name: "role references missing provider",
			json: `{
				"providers": {"m": {"type": "mock"}},
				"roles": {"architect": "missing", "reflex": "m"}
			}`,
			wantSub: "unknown provider \"missing\"",
		},
		{
			name: "unknown provider type",
			json: `{
				"providers": {"m": {"type": "browser", "model": "x"}},
				"roles": {"architect": "m", "reflex": "m"}
			}`,
			wantSub: "unknown type \"browser\"",
		},
		{
			name: "api provider missing api_key_env",
			json: `{
				"providers": {"m": {"type": "api", "model": "claude"}},
				"roles": {"architect": "m", "reflex": "m"}
			}`,
			wantSub: "requires api_key_env",
		},
		{
			name: "unknown field rejected by DisallowUnknownFields",
			json: `{
				"workspce": ".",
				"providers": {"m": {"type": "mock"}},
				"roles": {"architect": "m", "reflex": "m"}
			}`,
			wantSub: "unknown field \"workspce\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeTempConfig(t, tc.json)
			_, err := Load(p)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestProviderConfig_APIKey(t *testing.T) {
	pc := ProviderConfig{
		Type:      ProviderAPI,
		Model:     "claude",
		APIKeyEnv: "APW_TEST_SECRET_ENV_VAR",
	}

	t.Setenv("APW_TEST_SECRET_ENV_VAR", "")
	_, err := pc.APIKey()
	if err == nil || !strings.Contains(err.Error(), "APW_TEST_SECRET_ENV_VAR") {
		t.Fatalf("expected error mentioning env var name, got %v", err)
	}

	t.Setenv("APW_TEST_SECRET_ENV_VAR", "sk-test-123")
	key, err := pc.APIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-test-123" {
		t.Errorf("got key %q, want \"sk-test-123\"", key)
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "apw.example.json"))
	if err != nil {
		t.Fatalf("apw.example.json should be valid: %v", err)
	}
	if cfg.Roles.Architect == "" || cfg.Roles.Reflex == "" {
		t.Fatal("expected roles to be populated in apw.example.json")
	}
}
