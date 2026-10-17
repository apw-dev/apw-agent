package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ProviderType identifies the backend type for a model provider.
type ProviderType string

const (
	ProviderAPI       ProviderType = "api"
	ProviderOpenAI    ProviderType = "openai"
	ProviderLocal     ProviderType = "local"
	ProviderAGY       ProviderType = "agy"
	ProviderClaudeCLI ProviderType = "claude-cli"
	ProviderCodexCLI  ProviderType = "codex-cli"
	ProviderMock      ProviderType = "mock"
)

// ProviderConfig defines connection and model settings for a single provider.
type ProviderConfig struct {
	Workspace  string       `json:"-"` // resolved credential root; never stored as provider config
	Type       ProviderType `json:"type"`
	Endpoint   string       `json:"endpoint,omitempty"`
	Model      string       `json:"model"`
	APIKeyEnv  string       `json:"api_key_env,omitempty"` // env var name, never the raw key
	TimeoutSec int          `json:"timeout_sec,omitempty"` // default 120
}

// APIKey resolves the secret key from the environment variable named in APIKeyEnv, or from .apw/auth.json.
func (p ProviderConfig) APIKey() (string, error) {
	envName := strings.TrimSpace(p.APIKeyEnv)
	if envName == "" {
		return "", errors.New("config: api_key_env is not set")
	}
	if val := os.Getenv(envName); val != "" {
		return val, nil
	}
	root := p.Workspace
	if root == "" {
		root = "."
	}
	if val := LoadSavedKey(root, envName); val != "" {
		return val, nil
	}
	return "", fmt.Errorf("config: environment variable %q is not set or empty", envName)
}

// SaveAuthKey stores an API key reference locally in .apw/auth.json (which is gitignored).
func SaveAuthKey(workspace, envName, secret string) error {
	dir := filepath.Join(workspace, ".apw")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	authPath := filepath.Join(dir, "auth.json")
	keys := make(map[string]string)
	if data, err := os.ReadFile(authPath); err == nil {
		if err := json.Unmarshal(data, &keys); err != nil {
			return fmt.Errorf("read saved credentials: %w", err)
		}
		if keys == nil {
			return errors.New("saved credentials must contain a JSON object")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	keys[strings.TrimSpace(envName)] = strings.TrimSpace(secret)
	out, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(authPath, out, 0600)
}

// LoadSavedKey reads a saved API key from .apw/auth.json if present.
func LoadSavedKey(workspace, envName string) string {
	authPath := filepath.Join(workspace, ".apw", "auth.json")
	data, err := os.ReadFile(authPath)
	if err != nil {
		return ""
	}
	var keys map[string]string
	if err := json.Unmarshal(data, &keys); err != nil {
		return ""
	}
	return strings.TrimSpace(keys[strings.TrimSpace(envName)])
}

// Config holds the top-level configuration for apw.
type Config struct {
	Workspace string                    `json:"workspace"`
	Providers map[string]ProviderConfig `json:"providers"`
	Failover  FailoverConfig            `json:"failover,omitempty"`
	Policy    struct {
		Trusted bool `json:"trusted"`
	} `json:"policy"`
	Roles struct {
		Architect string `json:"architect"` // key in Providers
		Reflex    string `json:"reflex"`
	} `json:"roles"`
	Budget struct {
		MaxRepairs     int `json:"max_repairs"`      // default 3
		TaskTimeoutSec int `json:"task_timeout_sec"` // default 600
	} `json:"budget"`
	Retention RetentionConfig `json:"retention,omitzero"`
	Verify    VerifyConfig    `json:"verify,omitzero"`
}

// VerifyConfig overrides the detected verification pipeline. It is how a project in
// any language (or with a custom framework) tells Reflex what proves a change works.
// Stages run in order inside the isolated worktree and only after `apw trust`.
type VerifyConfig struct {
	Stages []VerifyStage `json:"stages,omitempty"`
}

// VerifyStage is one command; Run is an argument vector (no shell).
type VerifyStage struct {
	Name       string            `json:"name"`
	Run        []string          `json:"run"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutSec int               `json:"timeout_sec,omitempty"`
}

// RetentionConfig bounds retained task state. Zero selects the default; a negative value disables that rule.
type RetentionConfig struct {
	SucceededDays  int      `json:"succeeded_days,omitempty"`   // default 14
	FailedDays     int      `json:"failed_days,omitempty"`      // failed/cancelled/provider_unavailable; default 30
	OutboxSentDays int      `json:"outbox_sent_days,omitempty"` // delivered Telegram replies; default 7
	InboxKeep      int      `json:"inbox_keep,omitempty"`       // newest Telegram update IDs per account; default 1000
	LogMaxMB       int      `json:"log_max_mb,omitempty"`       // daemon.log rotation threshold; default 8
	AutoCleanup    bool     `json:"auto_cleanup,omitempty"`     // conservative pass at daemon startup
	ProtectTasks   []string `json:"protect_tasks,omitempty"`
}

// FailoverConfig lists explicitly allowed fallback providers, in priority order.
// Discovery makes models selectable; it does not authorize using every model.
type FailoverConfig struct {
	Reflex              []string `json:"reflex,omitempty"`
	Architect           []string `json:"architect,omitempty"`
	DirectiveTimeoutSec int      `json:"directive_timeout_sec,omitempty"`
}

// Load reads a JSON config file, applies defaults, and validates the configuration.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("parse config %q: trailing JSON content", path)
	}

	cfg.applyDefaults()
	cfg.bindProviders(path)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadOrDefault loads the config at path, or returns a sensible auto-configured default if the file does not exist.
func LoadOrDefault(path string) (*Config, error) {
	if _, err := os.Stat(path); err == nil {
		return Load(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg := &Config{
		Workspace: ".",
		Providers: map[string]ProviderConfig{
			"claude-sonnet": {
				Type:       ProviderAPI,
				Endpoint:   "https://api.anthropic.com",
				Model:      "claude-sonnet-4-20250514",
				APIKeyEnv:  "ANTHROPIC_API_KEY",
				TimeoutSec: 120,
			},
			"claude-haiku": {
				Type:       ProviderAPI,
				Endpoint:   "https://api.anthropic.com",
				Model:      "claude-3-5-haiku-latest",
				APIKeyEnv:  "ANTHROPIC_API_KEY",
				TimeoutSec: 60,
			},
			"ollama-local": {
				Type:       ProviderLocal,
				Endpoint:   "http://127.0.0.1:11434",
				Model:      "qwen2.5-coder:7b",
				TimeoutSec: 120,
			},
			"apw-builtin": {
				Type:       ProviderMock,
				Model:      "APW Engine",
				TimeoutSec: 30,
			},
		},
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		cfg.Roles.Architect = "claude-sonnet"
		cfg.Roles.Reflex = "claude-haiku"
	} else {
		cfg.Roles.Architect = "apw-builtin"
		cfg.Roles.Reflex = "apw-builtin"
	}
	cfg.applyDefaults()
	cfg.bindProviders(path)
	return cfg, nil
}

// Save writes the configuration to path as formatted JSON.
func (c *Config) Save(path string) error {
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(path, data, 0644)
}

// bindProviders keeps saved credentials relative to the configured workspace, not cwd.
func (c *Config) bindProviders(path string) {
	root := c.Workspace
	if !filepath.IsAbs(root) {
		root = filepath.Join(filepath.Dir(path), root)
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	for key, provider := range c.Providers {
		provider.Workspace = root
		c.Providers[key] = provider
	}
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".apw-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

// DetectStack inspects the workspace directory and returns detected languages/frameworks.
func DetectStack(workspace string) string {
	var stacks []string
	checks := []struct {
		file  string
		label string
	}{
		{"go.mod", "Go"},
		{"tsconfig.json", "TypeScript"},
		{"package.json", "Node.js"},
		{"Cargo.toml", "Rust"},
		{"pyproject.toml", "Python"},
		{"requirements.txt", "Python"},
	}

	seen := make(map[string]bool)
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(workspace, c.file)); err == nil {
			if c.label == "Node.js" && seen["TypeScript"] {
				continue
			}
			if !seen[c.label] {
				seen[c.label] = true
				stacks = append(stacks, c.label)
			}
		}
	}
	if len(stacks) == 0 {
		return "Go"
	}
	return strings.Join(stacks, " / ")
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Workspace) == "" {
		c.Workspace = "."
	}
	if c.Budget.MaxRepairs <= 0 {
		c.Budget.MaxRepairs = 3
	}
	if c.Budget.TaskTimeoutSec <= 0 {
		c.Budget.TaskTimeoutSec = 600
	}
	for k, p := range c.Providers {
		if p.TimeoutSec <= 0 {
			p.TimeoutSec = 120
			c.Providers[k] = p
		}
	}
}

// Validate checks that roles reference valid providers and provider settings are complete.
func (c *Config) Validate() error {
	if c.Failover.DirectiveTimeoutSec < 0 {
		return errors.New("config: failover.directive_timeout_sec must not be negative")
	}
	for _, key := range append(append([]string{}, c.Failover.Reflex...), c.Failover.Architect...) {
		if _, ok := c.Providers[key]; !ok {
			return fmt.Errorf("config: failover references unknown provider %q", key)
		}
	}
	if len(c.Providers) == 0 {
		return errors.New("config: at least one provider must be configured")
	}
	seenStage := map[string]bool{}
	for i, s := range c.Verify.Stages {
		if !validStageName(s.Name) {
			return fmt.Errorf("config: verify.stages[%d].name %q must be lowercase letters, digits, '-', '_' or ':'", i, s.Name)
		}
		if seenStage[s.Name] {
			return fmt.Errorf("config: verify.stages has duplicate name %q", s.Name)
		}
		seenStage[s.Name] = true
		if len(s.Run) == 0 || strings.TrimSpace(s.Run[0]) == "" {
			return fmt.Errorf("config: verify.stages[%d] (%s) requires a run command", i, s.Name)
		}
		if s.TimeoutSec < 0 {
			return fmt.Errorf("config: verify.stages[%d] (%s) timeout_sec must not be negative", i, s.Name)
		}
	}

	for name, p := range c.Providers {
		switch p.Type {
		case ProviderAPI, ProviderOpenAI:
			if strings.TrimSpace(p.APIKeyEnv) == "" {
				return fmt.Errorf("config: provider %q (type %s) requires api_key_env", name, p.Type)
			}
			if strings.TrimSpace(p.Model) == "" {
				return fmt.Errorf("config: provider %q (type %s) requires model", name, p.Type)
			}
		case ProviderLocal, ProviderAGY, ProviderClaudeCLI, ProviderCodexCLI:
			if strings.TrimSpace(p.Model) == "" {
				return fmt.Errorf("config: provider %q (type %s) requires model", name, p.Type)
			}
		case ProviderMock:
		default:
			return fmt.Errorf("config: provider %q has unknown type %q", name, p.Type)
		}
	}

	if c.Roles.Architect == "" {
		return errors.New("config: roles.architect must be set")
	}
	if _, ok := c.Providers[c.Roles.Architect]; !ok {
		return fmt.Errorf("config: roles.architect references unknown provider %q", c.Roles.Architect)
	}

	if c.Roles.Reflex == "" {
		return errors.New("config: roles.reflex must be set")
	}
	if _, ok := c.Providers[c.Roles.Reflex]; !ok {
		return fmt.Errorf("config: roles.reflex references unknown provider %q", c.Roles.Reflex)
	}

	return nil
}

// validStageName matches verification stage names usable in plan checks.
func validStageName(name string) bool {
	if name == "" || len(name) > 32 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == ':') {
			return false
		}
	}
	return true
}
