package auth

import (
	"github.com/apiwat/apw-agent/internal/config"
	"testing"
)

func TestDiscoveryCannotChangeSelectedModelsOrDropFallbacks(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"agy:flash":    {Type: config.ProviderAGY, Model: "flash"},
		"agy:fallback": {Type: config.ProviderAGY, Model: "fallback"},
		"agy:obsolete": {Type: config.ProviderAGY, Model: "obsolete"},
	}}
	cfg.Roles.Reflex = "agy:flash"
	cfg.Roles.Architect = "agy:flash"
	cfg.Failover.Architect = []string{"agy:fallback"}
	mergeDiscovered(cfg, []ConnectionStatus{{Source: "AGY CLI", Connected: true, Models: []DiscoveredModel{{ID: "gemini-4-argon-high", ProviderKey: "agy:argon", ProviderType: config.ProviderAGY}}}})
	if cfg.Roles.Reflex != "agy:flash" || cfg.Roles.Architect != "agy:flash" {
		t.Fatal("model refresh silently changed selected roles")
	}
	if _, ok := cfg.Providers["agy:fallback"]; !ok {
		t.Fatal("model refresh removed an authorized fallback")
	}
	if _, ok := cfg.Providers["agy:obsolete"]; ok {
		t.Fatal("obsolete unreferenced catalog entry remained")
	}
	if _, ok := cfg.Providers["agy:argon"]; !ok {
		t.Fatal("discovered model is not selectable")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Roles.Architect = "apw-builtin"
	cfg.Providers["apw-builtin"] = config.ProviderConfig{Type: config.ProviderMock}
	mergeDiscovered(cfg, []ConnectionStatus{{Source: "AGY CLI", Connected: true, Models: []DiscoveredModel{{ID: "gemini-4-argon-high", ProviderKey: "agy:argon", ProviderType: config.ProviderAGY}}}})
	if cfg.Roles.Architect != "apw-builtin" {
		t.Fatal("discovery enrolled a real provider without selection")
	}
}

func TestDiscoveryMigratesLegacyDefunctAGYModels(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"agy:claude-sonnet-4-6":        {Type: config.ProviderAGY, Model: "claude-sonnet-4-6"},
		"agy:claude-opus-4-6-thinking": {Type: config.ProviderAGY, Model: "claude-opus-4-6-thinking"},
	}}
	cfg.Roles.Architect = "agy:claude-sonnet-4-6"
	cfg.Roles.Reflex = "agy:claude-sonnet-4-6"
	cfg.Failover.Architect = []string{"agy:claude-opus-4-6-thinking"}
	mergeDiscovered(cfg, []ConnectionStatus{{
		Source:    "AGY CLI",
		Connected: true,
		Models: []DiscoveredModel{
			{ID: "claude-sonnet-5-5-high", ProviderKey: "agy:claude-sonnet-5-5-high", ProviderType: config.ProviderAGY},
			{ID: "claude-opus-5-5-high", ProviderKey: "agy:claude-opus-5-5-high", ProviderType: config.ProviderAGY},
		},
	}})
	if cfg.Roles.Architect != "agy:claude-sonnet-5-5-high" {
		t.Fatalf("expected legacy agy:claude-sonnet-4-6 to migrate to agy:claude-sonnet-5-5-high, got %q", cfg.Roles.Architect)
	}
	if cfg.Failover.Architect[0] != "agy:claude-opus-5-5-high" {
		t.Fatalf("expected legacy agy:claude-opus-4-6-thinking to migrate to agy:claude-opus-5-5-high, got %q", cfg.Failover.Architect[0])
	}
	if _, ok := cfg.Providers["agy:claude-sonnet-4-6"]; ok {
		t.Fatal("expected defunct agy:claude-sonnet-4-6 to be pruned after migration")
	}
}
