package desktop

import (
	"path/filepath"
	"sort"

	"github.com/apiwat/apw-agent/internal/auth"
	"github.com/apiwat/apw-agent/internal/config"
)

// discoverModels merges the models of connected accounts into cfg; tests replace it.
var discoverModels = func(workspace string, cfg *config.Config) {
	auth.SyncConfigWithDiscovered(workspace, cfg, true)
}

// DiscoverProviders adds the models of connected accounts (AGY, Claude, ChatGPT/OpenAI, Ollama)
// to the configuration at cfgPath, as /model does in the apw shell. It runs only while a role
// still uses the built-in placeholder, so a configured workspace is never rewritten, and it
// leaves the roles for the user to choose. It returns the provider keys it added.
func DiscoverProviders(cfgPath string) ([]string, error) {
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadOrDefault(abs)
	if err != nil {
		return nil, err
	}
	if len(cfg.PlaceholderRoles()) == 0 {
		return nil, nil
	}
	root := cfg.Workspace
	if !filepath.IsAbs(root) {
		root = filepath.Join(filepath.Dir(abs), root)
	}
	before := make(map[string]bool, len(cfg.Providers))
	for key := range cfg.Providers {
		before[key] = true
	}
	discoverModels(root, cfg)
	var added []string
	for key := range cfg.Providers {
		if !before[key] {
			added = append(added, key)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	sort.Strings(added)
	return added, cfg.Save(abs)
}
