package executor

import (
	"fmt"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/provider"
	"github.com/apiwat/apw-agent/internal/provider/factory"
)

// ConfigureFailover uses selected roles and explicit fallbacks only. In particular,
// discovering an Argon model must not add it to a Flash user's execution path.
// The caller owns the primary providers and must close the returned extra providers.
func (e *Executor) ConfigureFailover(cfg *config.Config) (func(), error) {
	var owned []provider.Provider
	closeExtras := func() {
		for _, p := range owned {
			_ = p.Close()
		}
	}
	instances := map[string]provider.Provider{
		cfg.Roles.Architect: e.Architect.Provider,
	}
	if _, exists := instances[cfg.Roles.Reflex]; !exists {
		instances[cfg.Roles.Reflex] = e.ReflexProvider
	}
	build := func(primary string, extras []string) ([]FailoverCandidate, error) {
		var pool []FailoverCandidate
		seen := make(map[string]bool)
		for _, key := range append([]string{primary}, extras...) {
			if seen[key] {
				continue
			}
			seen[key] = true
			pc, ok := cfg.Providers[key]
			if !ok {
				return nil, fmt.Errorf("failover: unknown provider %q", key)
			}
			p, ok := instances[key]
			if !ok {
				var err error
				p, err = factory.New(pc)
				if err != nil {
					return nil, fmt.Errorf("failover provider %q: %w", key, err)
				}
				instances[key] = p
				owned = append(owned, p)
			}
			pool = append(pool, FailoverCandidate{
				Provider: p, Model: pc.Model, ProviderKey: key,
				QuotaGroup: configuredQuotaGroup(pc),
			})
		}
		return pool, nil
	}
	var err error
	e.ReflexPool, err = build(cfg.Roles.Reflex, cfg.Failover.Reflex)
	if err == nil {
		e.ArchitectPool, err = build(cfg.Roles.Architect, cfg.Failover.Architect)
	}
	if err != nil {
		closeExtras()
		return nil, err
	}
	e.DirectiveTimeout = time.Duration(cfg.Failover.DirectiveTimeoutSec) * time.Second
	if e.DirectiveTimeout <= 0 {
		e.DirectiveTimeout = 90 * time.Second
	}
	return closeExtras, nil
}

func configuredQuotaGroup(pc config.ProviderConfig) string {
	if pc.Type == config.ProviderAGY {
		return agyQuotaGroup(pc.Model)
	}
	// Different transports and API credentials must not share a cooldown.
	return fmt.Sprintf("%s:%s:%s", pc.Type, pc.Endpoint, pc.APIKeyEnv)
}

// AddArchitectFallbacks appends cfg.Failover.Architect to the architect pool for
// read-only work (review and planning). Unlike ConfigureFailover it tolerates
// fallbacks that cannot be constructed (for example missing credentials): those
// are skipped and reported, so an unusable fallback never blocks read-only work.
// The caller must close the returned providers.
func (e *Executor) AddArchitectFallbacks(cfg *config.Config) (func(), []error) {
	var owned []provider.Provider
	closeExtras := func() {
		for _, p := range owned {
			_ = p.Close()
		}
	}
	if len(e.ArchitectPool) == 0 && e.Architect != nil && e.Architect.Provider != nil {
		e.ArchitectPool = []FailoverCandidate{{Provider: e.Architect.Provider, Model: e.Architect.Model, ProviderKey: cfg.Roles.Architect}}
	}
	seen := map[string]bool{cfg.Roles.Architect: true}
	for _, c := range e.ArchitectPool {
		if c.ProviderKey != "" {
			seen[c.ProviderKey] = true
		}
	}
	var skipped []error
	for _, key := range cfg.Failover.Architect {
		if seen[key] {
			continue
		}
		seen[key] = true
		pc, ok := cfg.Providers[key]
		if !ok {
			skipped = append(skipped, fmt.Errorf("architect fallback %q: unknown provider", key))
			continue
		}
		if pc.Workspace == "" {
			pc.Workspace = cfg.Workspace
		}
		p, err := factory.New(pc)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("architect fallback %q: %w", key, err))
			continue
		}
		owned = append(owned, p)
		e.ArchitectPool = append(e.ArchitectPool, FailoverCandidate{Provider: p, Model: pc.Model, ProviderKey: key, QuotaGroup: configuredQuotaGroup(pc)})
	}
	return closeExtras, skipped
}
