package auth

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/process"
)

// DiscoveredModel represents a real model fetched from an authenticated CLI or API account.
type DiscoveredModel struct {
	ID           string              `json:"id"`
	DisplayName  string              `json:"display_name"`
	ProviderKey  string              `json:"provider_key"`
	ProviderType config.ProviderType `json:"provider_type"`
	Source       string              `json:"source"` // "AGY CLI", "Claude Sub", "Claude API", "OpenAI API", "ChatGPT Sub", "Ollama"
	Endpoint     string              `json:"endpoint,omitempty"`
	APIKeyEnv    string              `json:"api_key_env,omitempty"`
}

// ConnectionStatus reports the authentication & model discovery status of each provider channel.
type ConnectionStatus struct {
	Name       string            `json:"name"`
	Source     string            `json:"source"`
	Connected  bool              `json:"connected"`
	AuthMethod string            `json:"auth_method"`
	Detail     string            `json:"detail"`
	Models     []DiscoveredModel `json:"models"`
}

type modelCache struct {
	UpdatedAt time.Time          `json:"updated_at"`
	Statuses  []ConnectionStatus `json:"statuses"`
}

func cacheFilePath(workspace string) string {
	return filepath.Join(workspace, ".apw", "models_cache.json")
}

// DiscoverAll checks all authentication sources (AGY CLI, Claude Subscription/API, ChatGPT Subscription/OpenAI API, Ollama)
// concurrently and returns their connection status and live models. Uses a short-lived cache unless forceRefresh is true.
func DiscoverAll(workspace string, forceRefresh bool) []ConnectionStatus {
	if flag.Lookup("test.v") != nil && !forceRefresh {
		return nil
	}

	if !forceRefresh {
		if cached, ok := readCache(workspace); ok {
			return cached
		}
	}

	var (
		agySt    ConnectionStatus
		claudeSt ConnectionStatus
		openaiSt ConnectionStatus
		ollamaSt ConnectionStatus
		wg       sync.WaitGroup
	)

	wg.Add(4)
	go func() {
		defer wg.Done()
		agySt = discoverAGY()
	}()
	go func() {
		defer wg.Done()
		claudeSt = discoverClaude(workspace)
	}()
	go func() {
		defer wg.Done()
		openaiSt = discoverOpenAI(workspace)
	}()
	go func() {
		defer wg.Done()
		ollamaSt = discoverOllama()
	}()
	wg.Wait()

	statuses := []ConnectionStatus{agySt, claudeSt, openaiSt, ollamaSt}
	writeCache(workspace, statuses)
	return statuses
}

// SyncConfigWithDiscovered updates selectable models without changing authorized roles or fallbacks.
func SyncConfigWithDiscovered(workspace string, cfg *config.Config, forceRefresh bool) []ConnectionStatus {
	statuses := DiscoverAll(workspace, forceRefresh)
	mergeDiscovered(cfg, statuses)
	return statuses
}

func mergeDiscovered(cfg *config.Config, statuses []ConnectionStatus) {
	if cfg == nil {
		return
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}

	activeKeys := make(map[string]bool)
	connectedPrefixes := make(map[string]bool)

	for _, st := range statuses {
		if !st.Connected {
			continue
		}
		switch st.Source {
		case "AGY CLI":
			connectedPrefixes["agy:"] = true
		case "Claude", "Claude Sub":
			connectedPrefixes["claude-sub:"] = true
		case "OpenAI", "ChatGPT Sub":
			connectedPrefixes["chatgpt-sub:"] = true
		}

		for _, m := range st.Models {
			activeKeys[m.ProviderKey] = true
			timeoutSec := 300
			if existing, exists := cfg.Providers[m.ProviderKey]; exists && existing.TimeoutSec > 0 {
				timeoutSec = existing.TimeoutSec
			} else {
				lower := strings.ToLower(m.ID)
				if strings.Contains(lower, "flash") || strings.Contains(lower, "haiku") || strings.Contains(lower, "mini") {
					timeoutSec = 60
				} else if strings.Contains(lower, "sol") || strings.Contains(lower, "opus") || strings.Contains(lower, "high") {
					timeoutSec = 600
				}
			}
			cfg.Providers[m.ProviderKey] = config.ProviderConfig{
				Type:       m.ProviderType,
				Endpoint:   m.Endpoint,
				Model:      m.ID,
				APIKeyEnv:  m.APIKeyEnv,
				TimeoutSec: timeoutSec,
			}

		}
	}

	// Prune obsolete provider keys from connected CLI sources (e.g. removed agy:* or chatgpt-sub:*)
	referenced := map[string]bool{cfg.Roles.Architect: true, cfg.Roles.Reflex: true}
	for _, key := range append(append([]string{}, cfg.Failover.Reflex...), cfg.Failover.Architect...) {
		referenced[key] = true
	}
	for k := range cfg.Providers {
		if referenced[k] {
			continue
		}
		for prefix := range connectedPrefixes {
			if strings.HasPrefix(k, prefix) && !activeKeys[k] {
				delete(cfg.Providers, k)
			}
		}
	}

}

func discoverAGY() ConnectionStatus {
	st := ConnectionStatus{
		Name:   "AGY CLI Connect",
		Source: "AGY CLI",
	}
	agyPath, err := exec.LookPath("agy")
	if err != nil {
		st.Connected = false
		st.AuthMethod = "not installed"
		st.Detail = "Install Antigravity CLI (agy) to connect Google AI Pro / AGY models"
		return st
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, agyPath, "models")
	var out process.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := process.Run(ctx, cmd); err != nil {
		st.Connected = false
		st.AuthMethod = "CLI found, auth required"
		st.Detail = "Run 'agy' to sign in"
		return st
	}

	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "fetching") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		modelID := strings.TrimSpace(parts[0])
		if modelID == "" || strings.Contains(modelID, " ") {
			continue
		}
		displayName := modelID
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			displayName = strings.TrimSpace(parts[1])
		}
		provKey := "agy:" + modelID
		st.Models = append(st.Models, DiscoveredModel{
			ID:           modelID,
			DisplayName:  displayName,
			ProviderKey:  provKey,
			ProviderType: config.ProviderAGY,
			Source:       "AGY CLI",
		})
	}

	if len(st.Models) > 0 {
		st.Connected = true
		st.AuthMethod = "agy session"
		st.Detail = fmt.Sprintf("%d live models fetched via agy CLI", len(st.Models))
	} else {
		st.Connected = false
		st.AuthMethod = "no models returned"
		st.Detail = "Run 'agy models' to verify connection"
	}
	return st
}

func discoverClaude(workspace string) ConnectionStatus {
	st := ConnectionStatus{
		Name:   "Claude Subscription / API",
		Source: "Claude",
	}

	// 1. Check Claude Code Subscription CLI (`claude auth status`)
	if claudePath, err := exec.LookPath("claude"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, claudePath, "auth", "status")
		var out process.Buffer
		cmd.Stdout = &out
		if err := process.Run(ctx, cmd); err == nil {
			var statusObj struct {
				LoggedIn   bool   `json:"loggedIn"`
				AuthMethod string `json:"authMethod"`
			}
			if json.Unmarshal([]byte(out.String()), &statusObj) == nil && statusObj.LoggedIn {
				st.Connected = true
				st.AuthMethod = "Claude Subscription (" + statusObj.AuthMethod + ")"
				subModels := []struct {
					id   string
					name string
				}{
					{"claude-sonnet-5-5", "Claude Sonnet 5.5 (Subscription)"},
					{"claude-opus-5-5", "Claude Opus 5.5 (Subscription)"},
					{"claude-haiku-5-5", "Claude Haiku 5.5 (Subscription)"},
					{"claude-sonnet-4-6", "Claude Sonnet 4.6 (Subscription)"},
					{"claude-opus-4-6", "Claude Opus 4.6 (Subscription)"},
					{"claude-haiku-4-5-20251001", "Claude Haiku 4.5 (Subscription)"},
				}
				for _, m := range subModels {
					st.Models = append(st.Models, DiscoveredModel{
						ID:           m.id,
						DisplayName:  m.name,
						ProviderKey:  "claude-sub:" + m.id,
						ProviderType: config.ProviderClaudeCLI,
						Source:       "Claude Sub",
					})
				}
			}
		}
	}

	// 2. Check ANTHROPIC_API_KEY (Environment or .apw/auth.json)
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = config.LoadSavedKey(workspace, "ANTHROPIC_API_KEY")
	}
	if apiKey != "" {
		apiModels := fetchAnthropicAPIModels(apiKey)
		if len(apiModels) == 0 {
			// Fallback standard Anthropic API models if /v1/models endpoint not reachable
			apiModels = []DiscoveredModel{
				{ID: "claude-sonnet-4-20250514", DisplayName: "Claude Sonnet 4 (API)", ProviderKey: "claude-api:sonnet-4", ProviderType: config.ProviderAPI, Source: "Claude API", Endpoint: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY"},
				{ID: "claude-3-5-haiku-latest", DisplayName: "Claude 3.5 Haiku (API)", ProviderKey: "claude-api:haiku-3.5", ProviderType: config.ProviderAPI, Source: "Claude API", Endpoint: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY"},
			}
		}
		st.Models = append(st.Models, apiModels...)
		st.Connected = true
		if st.AuthMethod == "" {
			st.AuthMethod = "ANTHROPIC_API_KEY"
		} else {
			st.AuthMethod += " + API Key"
		}
	}

	if st.Connected {
		st.Detail = fmt.Sprintf("%d models ready (%s)", len(st.Models), st.AuthMethod)
	} else {
		st.AuthMethod = "not authenticated"
		st.Detail = "Run '/auth' -> Connect Claude Subscription or set ANTHROPIC_API_KEY"
	}
	return st
}

func fetchAnthropicAPIModels(apiKey string) []DiscoveredModel {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return nil
	}

	var out []DiscoveredModel
	for _, item := range parsed.Data {
		if item.ID == "" {
			continue
		}
		disp := item.DisplayName
		if disp == "" {
			disp = item.ID
		}
		out = append(out, DiscoveredModel{
			ID:           item.ID,
			DisplayName:  disp + " (API)",
			ProviderKey:  "claude-api:" + item.ID,
			ProviderType: config.ProviderAPI,
			Source:       "Claude API",
			Endpoint:     "https://api.anthropic.com",
			APIKeyEnv:    "ANTHROPIC_API_KEY",
		})
	}
	return out
}

func discoverOpenAI(workspace string) ConnectionStatus {
	st := ConnectionStatus{
		Name:   "ChatGPT Subscription / OpenAI API",
		Source: "OpenAI",
	}

	// 1. Check OpenAI Codex CLI (ChatGPT Plus/Pro Subscription)
	if _, err := exec.LookPath("codex"); err == nil {
		st.Connected = true
		st.AuthMethod = "ChatGPT Subscription (Codex CLI)"

		// Attempt to read Codex CLI's live model catalog (~/.codex/models_cache.json)
		if home, err := os.UserHomeDir(); err == nil {
			codexCachePath := filepath.Join(home, ".codex", "models_cache.json")
			if data, err := os.ReadFile(codexCachePath); err == nil {
				var catalog struct {
					Models []struct {
						Slug        string `json:"slug"`
						DisplayName string `json:"display_name"`
						Visibility  string `json:"visibility"`
					} `json:"models"`
				}
				if json.Unmarshal(data, &catalog) == nil && len(catalog.Models) > 0 {
					for _, m := range catalog.Models {
						if m.Visibility == "list" && m.Slug != "" {
							disp := m.DisplayName
							if disp == "" {
								disp = m.Slug
							}
							st.Models = append(st.Models, DiscoveredModel{
								ID:           m.Slug,
								DisplayName:  disp + " (ChatGPT Subscription)",
								ProviderKey:  "chatgpt-sub:" + m.Slug,
								ProviderType: config.ProviderCodexCLI,
								Source:       "ChatGPT Sub",
							})
						}
					}
				}
			}
		}

		// Fallback to verified active Codex models if cache file was not present
		if len(st.Models) == 0 {
			codexModels := []struct {
				id   string
				name string
			}{
				{"gpt-6.1-sol", "GPT-6.1-Sol (ChatGPT Subscription)"},
				{"gpt-6-astra", "GPT-6-Astra (ChatGPT Subscription)"},
				{"gpt-6-sol", "GPT-6-Sol (ChatGPT Subscription)"},
				{"gpt-6-luna", "GPT-6-Luna (ChatGPT Subscription)"},
				{"gpt-5.6-sol", "GPT-5.6-Sol (ChatGPT Subscription)"},
				{"gpt-5.6-terra", "GPT-5.6-Terra (ChatGPT Subscription)"},
				{"gpt-5.6-luna", "GPT-5.6-Luna (ChatGPT Subscription)"},
			}
			for _, m := range codexModels {
				st.Models = append(st.Models, DiscoveredModel{
					ID:           m.id,
					DisplayName:  m.name,
					ProviderKey:  "chatgpt-sub:" + m.id,
					ProviderType: config.ProviderCodexCLI,
					Source:       "ChatGPT Sub",
				})
			}
		}
	}

	// 2. Check OPENAI_API_KEY (Environment or .apw/auth.json)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = config.LoadSavedKey(workspace, "OPENAI_API_KEY")
	}
	if apiKey != "" {
		liveModels := fetchOpenAIAPIModels(apiKey)
		if len(liveModels) == 0 {
			liveModels = []DiscoveredModel{
				{ID: "gpt-4.1", DisplayName: "GPT-4.1 (OpenAI API)", ProviderKey: "openai:gpt-4.1", ProviderType: config.ProviderOpenAI, Source: "OpenAI API", Endpoint: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"},
				{ID: "gpt-4.1-mini", DisplayName: "GPT-4.1 Mini (OpenAI API)", ProviderKey: "openai:gpt-4.1-mini", ProviderType: config.ProviderOpenAI, Source: "OpenAI API", Endpoint: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"},
				{ID: "o4-mini", DisplayName: "o4-mini Reasoning (OpenAI API)", ProviderKey: "openai:o4-mini", ProviderType: config.ProviderOpenAI, Source: "OpenAI API", Endpoint: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"},
			}
		}
		st.Models = append(st.Models, liveModels...)
		st.Connected = true
		if st.AuthMethod == "" {
			st.AuthMethod = "OPENAI_API_KEY"
		} else {
			st.AuthMethod += " + OPENAI_API_KEY"
		}
	}

	if st.Connected {
		st.Detail = fmt.Sprintf("%d models ready (%s)", len(st.Models), st.AuthMethod)
	} else {
		st.AuthMethod = "not authenticated"
		st.Detail = "Run '/auth' -> Connect ChatGPT Subscription (codex) or set OPENAI_API_KEY"
	}
	return st
}

func fetchOpenAIAPIModels(apiKey string) []DiscoveredModel {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return nil
	}

	preferredPrefixes := []string{"gpt-4.1", "gpt-4o", "o3", "o4", "codex"}
	var out []DiscoveredModel
	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		keep := false
		for _, pref := range preferredPrefixes {
			if strings.HasPrefix(id, pref) && !strings.Contains(id, "audio") && !strings.Contains(id, "realtime") && !strings.Contains(id, "transcribe") && !strings.Contains(id, "tts") {
				keep = true
				break
			}
		}
		if !keep {
			continue
		}
		out = append(out, DiscoveredModel{
			ID:           id,
			DisplayName:  id + " (OpenAI API)",
			ProviderKey:  "openai:" + id,
			ProviderType: config.ProviderOpenAI,
			Source:       "OpenAI API",
			Endpoint:     "https://api.openai.com",
			APIKeyEnv:    "OPENAI_API_KEY",
		})
		if len(out) >= 10 {
			break
		}
	}
	return out
}

func discoverOllama() ConnectionStatus {
	st := ConnectionStatus{
		Name:   "Ollama Local Runtime",
		Source: "Ollama",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:11434/api/tags", nil)
	if err != nil {
		st.Detail = "Not running on 127.0.0.1:11434"
		return st
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		st.AuthMethod = "offline"
		st.Detail = "Start Ollama server to use local models"
		return st
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Models) > 0 {
		st.Connected = true
		st.AuthMethod = "local HTTP"
		for _, m := range parsed.Models {
			st.Models = append(st.Models, DiscoveredModel{
				ID:           m.Name,
				DisplayName:  m.Name + " (Local Ollama)",
				ProviderKey:  "ollama:" + m.Name,
				ProviderType: config.ProviderLocal,
				Source:       "Ollama",
				Endpoint:     "http://127.0.0.1:11434",
			})
		}
		st.Detail = fmt.Sprintf("%d local models installed", len(st.Models))
	} else {
		st.AuthMethod = "no models pulled"
		st.Detail = "Run 'ollama pull qwen2.5-coder:7b'"
	}
	return st
}

func readCache(workspace string) ([]ConnectionStatus, bool) {
	b, err := os.ReadFile(cacheFilePath(workspace))
	if err != nil {
		return nil, false
	}
	var c modelCache
	if err := json.Unmarshal(b, &c); err != nil || len(c.Statuses) == 0 {
		return nil, false
	}
	// Invalidate cache immediately if it contains legacy defunct model IDs
	for _, st := range c.Statuses {
		for _, m := range st.Models {
			if strings.Contains(m.ID, "claude-opus-4-6-thinking") {
				return nil, false
			}
		}
	}
	if time.Since(c.UpdatedAt) > 3*time.Minute {
		return nil, false
	}
	return c.Statuses, true
}

func writeCache(workspace string, statuses []ConnectionStatus) {
	dir := filepath.Join(workspace, ".apw")
	_ = os.MkdirAll(dir, 0755)
	c := modelCache{
		UpdatedAt: time.Now(),
		Statuses:  statuses,
	}
	if b, err := json.MarshalIndent(c, "", "  "); err == nil {
		_ = os.WriteFile(cacheFilePath(workspace), b, 0644)
	}
}
