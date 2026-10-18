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

	// replyCh is the Go channel used to unblock the waiting goroutine with true (approved) or false (denied).
	replyCh  chan bool
	decision ApprovalDecisionType
}

// ApprovalGatekeeper coordinates Human-in-the-loop tool approvals via Go channels (`chan bool`)
// and maintains an in-memory allowlist for commands marked "อนุญาตเสมอสำหรับคำสั่งนี้".
type ApprovalGatekeeper struct {
	mu          sync.Mutex
	pending     map[string]*ToolApprovalRequest
	history     []*ToolApprovalRequest
	alwaysAllow map[string]bool
	subscribers map[chan ToolApprovalRequest]struct{}
}

// NewApprovalGatekeeper initializes a new Human-in-the-loop ApprovalGatekeeper.
func NewApprovalGatekeeper() *ApprovalGatekeeper {
	return &ApprovalGatekeeper{
		pending:     make(map[string]*ToolApprovalRequest),
		alwaysAllow: make(map[string]bool),
		subscribers: make(map[chan ToolApprovalRequest]struct{}),
	}
}

func normalizeCommandKey(toolName, command string) string {
	return strings.ToLower(strings.TrimSpace(toolName)) + "::" + strings.TrimSpace(command)
}

// IsAlwaysAllowed reports whether the user previously selected "อนุญาตเสมอสำหรับคำสั่งนี้" for this tool+command.
func (g *ApprovalGatekeeper) IsAlwaysAllowed(toolName, command string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.alwaysAllow[normalizeCommandKey(toolName, command)]
}

// RequestApproval creates a pending approval request and blocks on its `chan bool` until the user responds
// in the Desktop UI ("อนุญาตครั้งเดียว", "อนุญาตเสมอสำหรับคำสั่งนี้", or "ปฏิเสธ") or ctx is cancelled.
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
	g.history = append(g.history, req)
	snapshot := *req
	for sub := range g.subscribers {
		select {
		case sub <- snapshot:
		default:
		}
	}
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

// EnqueueApproval creates a non-blocking pending approval request and returns both the request metadata
// and its `<-chan bool` confirmation channel for callers that want to wait asynchronously.
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
	g.history = append(g.history, req)
	snapshot := *req
	for sub := range g.subscribers {
		select {
		case sub <- snapshot:
		default:
		}
	}
	return req, replyCh
}

// Resolve submits the user's decision ("once", "always", or "deny") and sends the boolean result to `replyCh`.
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
	for sub := range g.subscribers {
		select {
		case sub <- snapshot:
		default:
		}
	}
	return &snapshot, nil
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

// AlwaysAllowedCommands returns the list of command keys approved with "อนุญาตเสมอสำหรับคำสั่งนี้".
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

// Subscribe registers a channel to receive real-time approval request updates (for SSE/WebSocket streaming).
func (g *ApprovalGatekeeper) Subscribe() (chan ToolApprovalRequest, func()) {
	ch := make(chan ToolApprovalRequest, 16)
	g.mu.Lock()
	g.subscribers[ch] = struct{}{}
	g.mu.Unlock()

	unsubscribe := func() {
		g.mu.Lock()
		delete(g.subscribers, ch)
		g.mu.Unlock()
	}
	return ch, unsubscribe
}
