package desktop

import (
	"net/http"
	"sort"
	"strings"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/transport"
)

type providerOption struct {
	Key   string              `json:"key"`
	Type  config.ProviderType `json:"type"`
	Model string              `json:"model"`
}

type modelsPayload struct {
	Architect   string           `json:"architect"`
	Reflex      string           `json:"reflex"`
	Approval    string           `json:"approval"`
	SandboxMode string           `json:"sandbox_mode"`
	Providers   []providerOption `json:"providers"`
}

func buildModelsPayload(cfg *config.Config) modelsPayload {
	keys := make([]string, 0, len(cfg.Providers))
	for k := range cfg.Providers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	providers := make([]providerOption, 0, len(keys))
	for _, k := range keys {
		p := cfg.Providers[k]
		modelName := p.Model
		if modelName == "" {
			modelName = k
		}
		providers = append(providers, providerOption{
			Key:   k,
			Type:  p.Type,
			Model: modelName,
		})
	}
	approval := cfg.Approval
	if approval == "" {
		approval = "ask"
	}
	sandboxMode := cfg.Sandbox.Mode
	if sandboxMode == "" {
		sandboxMode = "off"
	}
	return modelsPayload{
		Architect:   cfg.Roles.Architect,
		Reflex:      cfg.Roles.Reflex,
		Approval:    approval,
		SandboxMode: sandboxMode,
		Providers:   providers,
	}
}

func (s *Server) handleGetModels(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	writeJSON(w, buildModelsPayload(cfg))
}

func (s *Server) handleUpdateModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Architect   string `json:"architect,omitempty"`
		Reflex      string `json:"reflex,omitempty"`
		Approval    string `json:"approval,omitempty"`
		SandboxMode string `json:"sandbox_mode,omitempty"`
	}
	if err := transport.Decode(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cfg, cfgFile, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	if v := strings.TrimSpace(body.Architect); v != "" {
		cfg.Roles.Architect = v
	}
	if v := strings.TrimSpace(body.Reflex); v != "" {
		cfg.Roles.Reflex = v
	}
	if v := strings.TrimSpace(body.Approval); v != "" {
		cfg.Approval = v
	}
	if v := strings.TrimSpace(body.SandboxMode); v != "" {
		cfg.Sandbox.Mode = v
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	if err := cfg.Save(cfgFile); err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	writeJSON(w, buildModelsPayload(cfg))
}
