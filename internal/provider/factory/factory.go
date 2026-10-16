package factory

import (
	"fmt"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/provider"
	"github.com/apiwat/apw-agent/internal/provider/api"
	"github.com/apiwat/apw-agent/internal/provider/mock"
)

const defaultMockPlanJSON = "```json\n" + `{
  "objective": "Execute requested task",
  "affected_files": ["main.go"],
  "subtasks": [
    {
      "id": "step-1",
      "description": "Verify workspace status",
      "action": "verify",
      "instruction": "Run closed-loop verification"
    }
  ],
  "acceptance_criteria": ["verification passes"]
}` + "\n```"

// New instantiates a provider.Provider from a ProviderConfig.
func New(cfg config.ProviderConfig) (provider.Provider, error) {
	switch cfg.Type {
	case config.ProviderAPI:
		return api.New(cfg)
	case config.ProviderMock:
		return mock.New(defaultMockPlanJSON), nil
	default:
		return nil, fmt.Errorf("factory: unsupported provider type %q", cfg.Type)
	}
}
