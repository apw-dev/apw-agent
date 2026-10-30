package core

import (
	"strings"
	"time"
)

type TaskStatus string

type TaskMode string

const (
	ModeReview  TaskMode = "review"
	ModePreview TaskMode = "preview"
	ModeExecute TaskMode = "execute"
	ModeVerify  TaskMode = "verify"
	// ModeUnverified prepares a patch like preview (no repository commands run, so no trust is
	// needed) but lets it be applied after APW's file-level acceptance checks; build, test and
	// browser checks are skipped and the report says so.
	ModeUnverified TaskMode = "unverified"
)

func (m TaskMode) Valid() bool {
	return m == ModeReview || m == ModePreview || m == ModeExecute || m == ModeVerify || m == ModeUnverified
}

const (
	StatusQueued              TaskStatus = "queued"
	StatusAnalyzing           TaskStatus = "analyzing"
	StatusPlanning            TaskStatus = "planning"
	StatusPatching            TaskStatus = "patching"
	StatusVerifying           TaskStatus = "verifying"
	StatusSucceeded           TaskStatus = "succeeded"
	StatusFailed              TaskStatus = "failed"
	StatusCancelled           TaskStatus = "cancelled"
	StatusProviderUnavailable TaskStatus = "provider_unavailable"
	StatusReady               TaskStatus = "ready_to_apply"
	StatusPreview             TaskStatus = "preview_ready"
	StatusInterrupted         TaskStatus = "interrupted"
	StatusApplying            TaskStatus = "applying"
)

type Task struct {
	ID          string     `json:"id"`
	Workspace   string     `json:"workspace"`
	Instruction string     `json:"instruction"`
	Mode        string     `json:"mode"` // preview, execute
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ModelRequest struct {
	RequestID   string            `json:"request_id"`
	System      string            `json:"system,omitempty"`
	CachePrefix string            `json:"cache_prefix,omitempty"`
	Prompt      string            `json:"prompt"`
	Model       string            `json:"model"`
	Options     map[string]string `json:"options,omitempty"`
}

// EffectivePrompt returns the complete prompt text regardless of whether CachePrefix is stored
// separately or already forms the leading prefix of Prompt.
func (r ModelRequest) EffectivePrompt() string {
	switch {
	case r.CachePrefix == "":
		return r.Prompt
	case r.Prompt == "":
		return r.CachePrefix
	case strings.HasPrefix(r.Prompt, r.CachePrefix):
		return r.Prompt
	default:
		return r.CachePrefix + "\n" + r.Prompt
	}
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type ModelResponse struct {
	ModelRequested string    `json:"model_requested,omitempty"`
	ModelEvidence  string    `json:"model_evidence,omitempty"`
	RequestID      string    `json:"request_id"`
	Provider       string    `json:"provider"`
	ModelObserved  string    `json:"model_observed"`
	Content        string    `json:"content"`
	Completed      bool      `json:"completed"`
	Usage          *Usage    `json:"usage,omitempty"`
	RawResponse    string    `json:"raw_response,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

// PromptTrace records real-time telemetry and full prompt contents sent to external AI models.
type PromptTrace struct {
	RequestID     string    `json:"request_id"`
	Stage         string    `json:"stage"` // "planning", "directive", "subagent", "review"
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Prompt        string    `json:"prompt"`
	PromptBytes   int       `json:"prompt_bytes,omitempty"`
	Response      string    `json:"response,omitempty"`
	ResponseBytes int       `json:"response_bytes,omitempty"`
	Status        string    `json:"status"` // "running", "completed", "failed", "fallback"
	Error         string    `json:"error,omitempty"`
	DurationMs    int64     `json:"duration_ms,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// RouteCandidateInfo describes a model candidate in the active failover route stack.
type RouteCandidateInfo struct {
	ProviderKey string `json:"provider_key"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Priority    int    `json:"priority"`
	Role        string `json:"role"`   // "architect" | "reflex" | "fallback"
	Status      string `json:"status"` // "ready" | "active" | "paused" | "failed"
	Detail      string `json:"detail,omitempty"`
}
