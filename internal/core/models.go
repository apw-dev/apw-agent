package core

import (
	"time"
)

type TaskStatus string

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

type ModelResponse struct {
	RequestID     string    `json:"request_id"`
	Provider      string    `json:"provider"`
	ModelObserved string    `json:"model_observed"`
	Content       string    `json:"content"`
	Completed     bool      `json:"completed"`
	RawResponse   string    `json:"raw_response,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}
