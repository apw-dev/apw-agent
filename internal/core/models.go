package core

import (
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
	RequestID string            `json:"request_id"`
	Prompt    string            `json:"prompt"`
	Model     string            `json:"model"`
	Options   map[string]string `json:"options,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
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
