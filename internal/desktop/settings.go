package desktop

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apiwat/apw-agent/internal/auth"
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
	Architect          string           `json:"architect"`
	Reflex             string           `json:"reflex"`
	ArchitectFallbacks []string         `json:"architect_fallbacks"`
	ReflexFallbacks    []string         `json:"reflex_fallbacks"`
	Approval           string           `json:"approval"`
	SandboxMode        string           `json:"sandbox_mode"`
	Providers          []providerOption `json:"providers"`
}

type telegramChannelPayload struct {
	Enabled         bool    `json:"enabled"`
	TokenEnv        string  `json:"token_env"`
	TokenConfigured bool    `json:"token_configured"`
	TokenMasked     string  `json:"token_masked"`
	AllowUsers      []int64 `json:"allow_users"`
}

type channelsPayload struct {
	Telegram telegramChannelPayload `json:"telegram"`
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
	archFallbacks := append([]string{}, cfg.Failover.Architect...)
	if archFallbacks == nil {
		archFallbacks = []string{}
	}
	reflexFallbacks := append([]string{}, cfg.Failover.Reflex...)
	if reflexFallbacks == nil {
		reflexFallbacks = []string{}
	}
	return modelsPayload{
		Architect:          cfg.Roles.Architect,
		Reflex:             cfg.Roles.Reflex,
		ArchitectFallbacks: archFallbacks,
		ReflexFallbacks:    reflexFallbacks,
		Approval:           approval,
		SandboxMode:        sandboxMode,
		Providers:          providers,
	}
}

func buildChannelsPayload(cfg *config.Config, credentialRoot string) channelsPayload {
	tg := cfg.Channels.Telegram
	tok := tg.ResolveToken(credentialRoot)
	allow := tg.AllowUsers
	if allow == nil {
		allow = []int64{}
	}
	return channelsPayload{
		Telegram: telegramChannelPayload{
			Enabled:         tg.Enabled,
			TokenEnv:        tg.EffectiveTokenEnv(),
			TokenConfigured: tok != "",
			TokenMasked:     config.MaskSecret(tok),
			AllowUsers:      allow,
		},
	}
}

func resolveCredentialRoot(cfg *config.Config, cfgFile string) string {
	root := cfg.Workspace
	if !filepath.IsAbs(root) {
		root = filepath.Join(filepath.Dir(cfgFile), root)
	}
	return root
}

func (s *Server) handleGetModels(w http.ResponseWriter, r *http.Request) {
	cfg, cfgFile, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	wsRoot := resolveCredentialRoot(cfg, cfgFile)
	beforeCount := len(cfg.Providers)
	auth.SyncConfigWithDiscovered(wsRoot, cfg, forceRefresh)
	if forceRefresh || len(cfg.Providers) != beforeCount {
		_ = cfg.Save(cfgFile)
	}
	writeJSON(w, buildModelsPayload(cfg))
}

func (s *Server) handleUpdateModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Architect          string    `json:"architect,omitempty"`
		Reflex             string    `json:"reflex,omitempty"`
		ArchitectFallbacks *[]string `json:"architect_fallbacks,omitempty"`
		ReflexFallbacks    *[]string `json:"reflex_fallbacks,omitempty"`
		Approval           string    `json:"approval,omitempty"`
		SandboxMode        string    `json:"sandbox_mode,omitempty"`
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
	if body.ArchitectFallbacks != nil {
		var clean []string
		for _, k := range *body.ArchitectFallbacks {
			k = strings.TrimSpace(k)
			if k != "" && k != cfg.Roles.Architect {
				clean = append(clean, k)
			}
		}
		cfg.Failover.Architect = clean
	}
	if body.ReflexFallbacks != nil {
		var clean []string
		for _, k := range *body.ReflexFallbacks {
			k = strings.TrimSpace(k)
			if k != "" && k != cfg.Roles.Reflex {
				clean = append(clean, k)
			}
		}
		cfg.Failover.Reflex = clean
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

func (s *Server) handleGetChannels(w http.ResponseWriter, r *http.Request) {
	cfg, cfgFile, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	writeJSON(w, buildChannelsPayload(cfg, resolveCredentialRoot(cfg, cfgFile)))
}

func (s *Server) handleUpdateChannels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Telegram *struct {
			Enabled    *bool   `json:"enabled,omitempty"`
			Token      string  `json:"token,omitempty"`
			ClearToken bool    `json:"clear_token,omitempty"`
			AllowUsers []int64 `json:"allow_users,omitempty"`
		} `json:"telegram,omitempty"`
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
	credentialRoot := resolveCredentialRoot(cfg, cfgFile)

	if body.Telegram != nil {
		envName := cfg.Channels.Telegram.EffectiveTokenEnv()
		if body.Telegram.ClearToken {
			if err := config.DeleteAuthKey(credentialRoot, envName); err != nil {
				writeError(w, http.StatusInternalServerError, "config_error", err.Error())
				return
			}
			_ = os.Unsetenv(envName)
		} else if tok := strings.TrimSpace(body.Telegram.Token); tok != "" {
			if err := config.SaveAuthKey(credentialRoot, envName, tok); err != nil {
				writeError(w, http.StatusInternalServerError, "config_error", err.Error())
				return
			}
			_ = os.Setenv(envName, tok)
			cfg.Channels.Telegram.TokenEnv = envName
		}
		if body.Telegram.AllowUsers != nil {
			cfg.Channels.Telegram.AllowUsers = body.Telegram.AllowUsers
		}
		if body.Telegram.Enabled != nil {
			cfg.Channels.Telegram.Enabled = *body.Telegram.Enabled
		}
		if cfg.Channels.Telegram.Enabled && cfg.Channels.Telegram.ResolveToken(credentialRoot) == "" {
			writeError(w, http.StatusBadRequest, "invalid_config", "Telegram Bot Token is required when enabling the Telegram channel")
			return
		}
	}

	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	if err := cfg.Save(cfgFile); err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	writeJSON(w, buildChannelsPayload(cfg, credentialRoot))
}
