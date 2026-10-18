package desktop

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
)

// ApprovalDecisionType represents the user's choice on a human-in-the-loop tool prompt.
type ApprovalDecisionType string

const (
	DecisionOnce   ApprovalDecisionType = "once"   // อนุญาตครั้งเดียว
	DecisionAlways ApprovalDecisionType = "always" // อนุญาตเสมอสำหรับคำสั่งนี้
	DecisionDeny   ApprovalDecisionType = "deny"   // ปฏิเสธ
)

// maxApprovalHistory bounds how many past requests the gatekeeper remembers.
const maxApprovalHistory = 100

// applyTool is the tool name of "apply task changes to the workspace" approvals. An "always"
// decision for it covers every later apply, whatever the task.
const applyTool = "Apply"

// ToolApprovalRequest describes a pending tool or system command awaiting user confirmation.
type ToolApprovalRequest struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id,omitempty"`
	ToolName    string    `json:"tool_name"`
	Command     string    `json:"command"`
	TargetPath  string    `json:"target_path,omitempty"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "pending", "approved_once", "approved_always", "denied"
	CreatedAt   time.Time `json:"created_at"`

	// replyCh unblocks the waiting caller with true (approved) or false (denied).
	replyCh  chan bool
	decision ApprovalDecisionType
}

// ApprovalGatekeeper coordinates human-in-the-loop approvals: callers wait for the user's decision
// and an in-memory allowlist remembers the commands the user marked "always allow".
type ApprovalGatekeeper struct {
	mu          sync.Mutex
	pending     map[string]*ToolApprovalRequest
	history     []*ToolApprovalRequest
	alwaysAllow map[string]bool
}

// NewApprovalGatekeeper initializes a new Human-in-the-loop ApprovalGatekeeper.
func NewApprovalGatekeeper() *ApprovalGatekeeper {
	return &ApprovalGatekeeper{
		pending:     make(map[string]*ToolApprovalRequest),
		alwaysAllow: make(map[string]bool),
	}
}

func normalizeCommandKey(toolName, command string) string {
	if strings.EqualFold(strings.TrimSpace(toolName), applyTool) {
		return strings.ToLower(applyTool)
	}
	return strings.ToLower(strings.TrimSpace(toolName)) + "::" + strings.TrimSpace(command)
}

// IsAlwaysAllowed reports whether the user previously selected "อนุญาตเสมอสำหรับคำสั่งนี้" for this tool+command.
func (g *ApprovalGatekeeper) IsAlwaysAllowed(toolName, command string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.alwaysAllow[normalizeCommandKey(toolName, command)]
}

// RequestApproval creates a pending approval request and blocks until the user responds in the
// Desktop UI (allow once, allow always, or deny) or ctx is cancelled.
func (g *ApprovalGatekeeper) RequestApproval(ctx context.Context, taskID, toolName, command, targetPath, description string) (bool, error) {
	key := normalizeCommandKey(toolName, command)

	g.mu.Lock()
	if g.alwaysAllow[key] {
		g.mu.Unlock()
		return true, nil
	}

	req := &ToolApprovalRequest{
		ID:          "appr-" + app.NewID(),
		TaskID:      taskID,
		ToolName:    toolName,
		Command:     command,
		TargetPath:  targetPath,
		Description: description,
		Status:      "pending",
		CreatedAt:   time.Now(),
		replyCh:     make(chan bool, 1),
	}
	g.pending[req.ID] = req
	g.remember(req)
	g.mu.Unlock()

	select {
	case allowed := <-req.replyCh:
		return allowed, nil
	case <-ctx.Done():
		g.mu.Lock()
		if cur, ok := g.pending[req.ID]; ok {
			cur.Status = "cancelled"
			delete(g.pending, req.ID)
		}
		g.mu.Unlock()
		return false, ctx.Err()
	}
}

// EnqueueApproval creates a non-blocking pending approval request and returns both the request
// metadata and a channel that receives the decision, for callers that want to wait asynchronously.
func (g *ApprovalGatekeeper) EnqueueApproval(taskID, toolName, command, targetPath, description string) (*ToolApprovalRequest, <-chan bool) {
	key := normalizeCommandKey(toolName, command)
	replyCh := make(chan bool, 1)

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.alwaysAllow[key] {
		replyCh <- true
		req := &ToolApprovalRequest{
			ID:          "appr-" + app.NewID(),
			TaskID:      taskID,
			ToolName:    toolName,
			Command:     command,
			TargetPath:  targetPath,
			Description: description,
			Status:      "approved_always",
			CreatedAt:   time.Now(),
			replyCh:     replyCh,
			decision:    DecisionAlways,
		}
		return req, replyCh
	}

	req := &ToolApprovalRequest{
		ID:          "appr-" + app.NewID(),
		TaskID:      taskID,
		ToolName:    toolName,
		Command:     command,
		TargetPath:  targetPath,
		Description: description,
		Status:      "pending",
		CreatedAt:   time.Now(),
		replyCh:     replyCh,
	}
	g.pending[req.ID] = req
	g.remember(req)
	return req, replyCh
}

// Resolve submits the user's decision ("once", "always", or "deny") and releases the waiting caller.
func (g *ApprovalGatekeeper) Resolve(id string, decision ApprovalDecisionType) (*ToolApprovalRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	req, ok := g.pending[id]
	if !ok {
		return nil, fmt.Errorf("approval request %q not found or already resolved", id)
	}
	delete(g.pending, id)
	req.decision = decision

	var allowed bool
	switch decision {
	case DecisionOnce:
		req.Status = "approved_once"
		allowed = true
	case DecisionAlways:
		req.Status = "approved_always"
		g.alwaysAllow[normalizeCommandKey(req.ToolName, req.Command)] = true
		allowed = true
	case DecisionDeny:
		req.Status = "denied"
		allowed = false
	default:
		return nil, fmt.Errorf("invalid decision %q (expected once, always, or deny)", decision)
	}

	select {
	case req.replyCh <- allowed:
	default:
	}

	snapshot := *req
	return &snapshot, nil
}

// remember appends req to the bounded history, dropping the oldest entries. Caller holds g.mu.
func (g *ApprovalGatekeeper) remember(req *ToolApprovalRequest) {
	g.history = append(g.history, req)
	if n := len(g.history); n > maxApprovalHistory {
		g.history = append([]*ToolApprovalRequest(nil), g.history[n-maxApprovalHistory:]...)
	}
}

// ListPending returns all currently waiting approval requests.
func (g *ApprovalGatekeeper) ListPending() []ToolApprovalRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]ToolApprovalRequest, 0, len(g.pending))
	for _, r := range g.pending {
		out = append(out, *r)
	}
	return out
}

// AlwaysAllowedCommands returns the keys of the commands the user approved with "always allow".
func (g *ApprovalGatekeeper) AlwaysAllowedCommands() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.alwaysAllow))
	for k, v := range g.alwaysAllow {
		if v {
			out = append(out, k)
		}
	}
	return out
}
