package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/apiwat/apw-agent/internal/config"
)

// ErrNoModelConnected rejects work while a role still uses the built-in placeholder provider.
// Clients match its text ("no model is connected") because errors cross the daemon as strings.
var ErrNoModelConnected = errors.New("no model is connected")

// discoveredPrefixes are the provider keys that connected-account discovery adds.
var discoveredPrefixes = []string{"agy:", "claude-sub:", "chatgpt-sub:", "openai:", "ollama:"}

// noModelError explains which roles have no real model and what to connect, listing models that
// discovery already found in the configuration. It returns nil when both roles have a model.
func noModelError(cfg *config.Config) error {
	roles := cfg.PlaceholderRoles()
	if len(roles) == 0 {
		return nil
	}
	var found []string
	for key := range cfg.Providers {
		for _, prefix := range discoveredPrefixes {
			if strings.HasPrefix(key, prefix) {
				found = append(found, key)
				break
			}
		}
	}
	sort.Strings(found)
	subject := "the " + roles[0] + " role uses"
	if len(roles) > 1 {
		subject = "the " + strings.Join(roles, " and ") + " roles use"
	}
	msg := fmt.Sprintf("%s the built-in placeholder %q, which cannot plan or edit code.", subject, config.BuiltinProvider)
	if len(found) > 0 {
		more := ""
		if len(found) > 4 {
			more = fmt.Sprintf(" and %d more", len(found)-4)
			found = found[:4]
		}
		msg += fmt.Sprintf(" Connected models found: %s%s. Choose the architect and reflex models in Settings (Ctrl+,) or with /model in the apw shell.", strings.Join(found, ", "), more)
	} else {
		msg += " Connect a model first: sign in to the Antigravity CLI (agy), Claude Code (claude) or Codex (codex), set ANTHROPIC_API_KEY, or start Ollama. Then choose the architect and reflex models in Settings (Ctrl+,) or with /model in the apw shell."
	}
	return fmt.Errorf("%w: %s", ErrNoModelConnected, msg)
}
