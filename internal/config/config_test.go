package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRejectsTrailingJSONAndPreservesPreviousFile(t *testing.T) {
	path := writeTempConfig(t, `{"providers":{"m":{"type":"mock"}},"roles":{"architect":"m","reflex":"m"}}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	cfg.Roles.Architect = "missing"
	if err := cfg.Save(path); err == nil {
		t.Fatal("invalid configuration was saved")
	}
	actual, _ := os.ReadFile(path)
	if !bytes.Equal(original, actual) {
		t.Fatal("failed save changed the previous configuration")
	}
	if err := os.WriteFile(path, append(original, []byte(" {}")...), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func TestSavedCredentialsResolveAgainstConfiguredWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "project with spaces")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	const envName = "APW_CONFIG_CREDENTIAL_ROOT_TEST"
	t.Setenv(envName, "")
	if err := SaveAuthKey(workspace, envName, "fixture-key"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "settings.json")
	content := `{"workspace":"project with spaces","providers":{"m":{"type":"openai","model":"fixture","api_key_env":"` + envName + `"}},"roles":{"architect":"m","reflex":"m"}}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	key, err := cfg.Providers["m"].APIKey()
	if err != nil || key != "fixture-key" {
		t.Fatal("provider did not resolve credentials from configured workspace")
	}
}

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

func TestFailoverConfigValidationAndRoundTrip(t *testing.T) {
	base := `{"providers":{"flash":{"type":"mock","model":"flash"},"gpt":{"type":"mock","model":"gpt"}},"roles":{"reflex":"flash","architect":"gpt"},"failover":%s}`
	for _, tc := range []struct {
		failover string
		wantErr  bool
	}{
		{`{"architect":["gpt"],"directive_timeout_sec":45}`, false},
		{`{"reflex":["missing"]}`, true},
		{`{"directive_timeout_sec":-1}`, true},
	} {
		p := writeTempConfig(t, fmt.Sprintf(base, tc.failover))
		cfg, err := Load(p)
		if (err != nil) != tc.wantErr {
			t.Fatalf("failover %s: %v", tc.failover, err)
		}
		if err != nil {
			continue
		}
		if err := cfg.Save(p); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(p)
		if err != nil || loaded.Failover.DirectiveTimeoutSec != 45 || len(loaded.Failover.Architect) != 1 || loaded.Failover.Architect[0] != "gpt" {
			t.Fatalf("failover configuration did not survive save: %+v %v", loaded, err)
		}
	}
}

func TestRetentionIsOptionalAndRoundTrips(t *testing.T) {
	path := writeTempConfig(t, `{"providers":{"m":{"type":"mock"}},"roles":{"architect":"m","reflex":"m"}}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); bytes.Contains(data, []byte("retention")) {
		t.Fatalf("default retention was written into the config:\n%s", data)
	}
	path = writeTempConfig(t, `{"providers":{"m":{"type":"mock"}},"roles":{"architect":"m","reflex":"m"},"retention":{"succeeded_days":3,"failed_days":-1,"protect_tasks":["task-x"],"auto_cleanup":true}}`)
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Retention
	if r.SucceededDays != 3 || r.FailedDays != -1 || !r.AutoCleanup || len(r.ProtectTasks) != 1 || r.ProtectTasks[0] != "task-x" {
		t.Fatalf("retention = %+v", r)
	}
}

func TestVerifyStagesConfig(t *testing.T) {
	base := `{"providers":{"m":{"type":"mock"}},"roles":{"architect":"m","reflex":"m"},"verify":{"stages":%s}}`
	cfg, err := Load(writeTempConfig(t, fmt.Sprintf(base, `[{"name":"lint","run":["pnpm","lint"]},{"name":"test","run":["pnpm","test"],"env":{"CI":"true"},"timeout_sec":600}]`)))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Verify.Stages) != 2 || cfg.Verify.Stages[1].Env["CI"] != "true" || cfg.Verify.Stages[1].TimeoutSec != 600 {
		t.Fatalf("stages not loaded: %+v", cfg.Verify)
	}
	for name, stages := range map[string]string{
		"empty run":      `[{"name":"lint","run":[]}]`,
		"bad name":       `[{"name":"Lint Check","run":["x"]}]`,
		"duplicate name": `[{"name":"t","run":["a"]},{"name":"t","run":["b"]}]`,
		"neg timeout":    `[{"name":"t","run":["a"],"timeout_sec":-1}]`,
	} {
		if _, err := Load(writeTempConfig(t, fmt.Sprintf(base, stages))); err == nil || !strings.Contains(err.Error(), "verify.stages") {
			t.Fatalf("%s: expected verify.stages error, got %v", name, err)
		}
	}
}
