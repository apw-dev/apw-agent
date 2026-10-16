package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ProviderType identifies the backend type for a model provider.
type ProviderType string

const (
	ProviderAPI   ProviderType = "api"
	ProviderLocal ProviderType = "local"
	ProviderMock  ProviderType = "mock"
)

// ProviderConfig defines connection and model settings for a single provider.
type ProviderConfig struct {
	Type       ProviderType `json:"type"`
	Endpoint   string       `json:"endpoint,omitempty"`
	Model      string       `json:"model"`
	APIKeyEnv  string       `json:"api_key_env,omitempty"` // env var name, never the raw key
	TimeoutSec int          `json:"timeout_sec,omitempty"` // default 120
}

// APIKey resolves the secret key from the environment variable named in APIKeyEnv.
func (p ProviderConfig) APIKey() (string, error) {
	envName := strings.TrimSpace(p.APIKeyEnv)
	if envName == "" {
		return "", errors.New("config: api_key_env is not set")
	}
	val := os.Getenv(envName)
	if val == "" {
		return "", fmt.Errorf("config: environment variable %q is not set or empty", envName)
	}
	return val, nil
}

// Config holds the top-level configuration for apw.
type Config struct {
	Workspace string                    `json:"workspace"`
	Providers map[string]ProviderConfig `json:"providers"`
	Roles     struct {
		Architect string `json:"architect"` // key in Providers
		Reflex    string `json:"reflex"`
	} `json:"roles"`
	Budget struct {
		MaxRepairs     int `json:"max_repairs"`      // default 3
		TaskTimeoutSec int `json:"task_timeout_sec"` // default 600
	} `json:"budget"`
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

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
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
	if len(c.Providers) == 0 {
		return errors.New("config: at least one provider must be configured")
	}

	for name, p := range c.Providers {
		switch p.Type {
		case ProviderAPI:
			if strings.TrimSpace(p.APIKeyEnv) == "" {
				return fmt.Errorf("config: provider %q (type api) requires api_key_env", name)
			}
			if strings.TrimSpace(p.Model) == "" {
				return fmt.Errorf("config: provider %q (type api) requires model", name)
			}
		case ProviderLocal:
			if strings.TrimSpace(p.Model) == "" {
				return fmt.Errorf("config: provider %q (type local) requires model", name)
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
