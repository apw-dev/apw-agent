package executor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// FailoverCandidate represents a provider + model combination in the priority pool.
type FailoverCandidate struct {
	Provider    provider.Provider
	Model       string
	ProviderKey string
	QuotaGroup  string
}

// IsFailoverError determines if an error warrants failing over to another model candidate.
func IsFailoverError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	indicators := []string{
		"429",
		"rate limit",
		"quota",
		"credit",
		"capacity",
		"overloaded",
		"busy",
		"timed out",
		"timeout",
		"deadline exceeded",
		"empty response",
		"incomplete response",
		"provider unavailable",
		"command line is too long",
		"killed",
		"context deadline exceeded",
	}
	for _, ind := range indicators {
		if strings.Contains(msg, ind) {
			return true
		}
	}
	return false
}

// AskWithFailover attempts request across candidates until success or exhaustion.
func AskWithFailover(ctx context.Context, candidates []FailoverCandidate, req core.ModelRequest, onFailover func(from, to FailoverCandidate, cause error)) (*core.ModelResponse, FailoverCandidate, error) {
	if len(candidates) == 0 {
		return nil, FailoverCandidate{}, errors.New("failover pool: no candidates available")
	}

	var lastErr error
	for i, cand := range candidates {
		if ctx.Err() != nil {
			return nil, cand, ctx.Err()
		}
		if cand.Provider == nil {
			continue
		}

		reqCopy := req
		if cand.Model != "" {
			reqCopy.Model = cand.Model
		}

		resp, err := cand.Provider.Ask(ctx, reqCopy)
		if err == nil && resp != nil && strings.TrimSpace(resp.Content) != "" {
			return resp, cand, nil
		}

		lastErr = err
		if err == nil {
			lastErr = errors.New("model returned empty response")
		}

		// If this is the last candidate or error is not failover-eligible, don't attempt further
		if i == len(candidates)-1 {
			break
		}

		if IsFailoverError(lastErr) {
			next := candidates[i+1]
			if onFailover != nil {
				onFailover(cand, next, lastErr)
			}
			continue
		}

		// Non-failover error (e.g. fatal validation error)
		break
	}

	return nil, FailoverCandidate{}, fmt.Errorf("failover pool exhausted (%d candidate(s)): %w", len(candidates), lastErr)
}

// PlanCheckpoint represents an interrupted worktree state that can be resumed.
type PlanCheckpoint struct {
	Pending         *MutationIntent              `json:"pending_mutation,omitempty"`
	Mode            core.TaskMode                `json:"mode,omitempty"`
	Phase           string                       `json:"phase,omitempty"`
	ReadContext     string                       `json:"read_context,omitempty"`
	Baseline        *reflex.VerificationResult   `json:"baseline,omitempty"`
	VerifiedFiles   map[string]surgeon.FileState `json:"verified_files,omitempty"`
	Report          *ExecutionReport             `json:"report,omitempty"`
	PlanID          string                       `json:"plan_id"`
	TaskID          string                       `json:"task_id"`
	Timestamp       time.Time                    `json:"timestamp"`
	LastSuccessStep int                          `json:"last_success_step"`
	TotalSteps      int                          `json:"total_steps"`
	WorktreeBranch  string                       `json:"worktree_branch"`
	WorktreePath    string                       `json:"worktree_path"`
	ModifiedFiles   []string                     `json:"modified_files"`
	Plan            *architect.ExecutionPlan     `json:"plan,omitempty"`
	Failover        FailoverSnapshot             `json:"failover,omitempty"`
}

// planHashes caches the sha256 of the last plan JSON written to each plan file
// (keyed by the full plan file path) so unchanged plans are not rewritten and
// fsynced on every checkpoint save.
var planHashes sync.Map

// SaveCheckpoint writes the checkpoint to `.apw/plans/<plan_id>.checkpoint.json`.
func SaveCheckpoint(workspace string, cp PlanCheckpoint) error {
	target, err := checkpointPath(workspace, cp.PlanID, ".checkpoint.json")
	if err != nil {
		return err
	}
	data, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	if cp.Plan != nil {
		if cp.Plan.PlanID != cp.PlanID || cp.Plan.TaskID != cp.TaskID {
			return errors.New("checkpoint and plan identity mismatch")
		}
		planData, err := json.MarshalIndent(cp.Plan, "", "  ")
		if err != nil {
			return err
		}
		planPath, _ := checkpointPath(workspace, cp.PlanID, ".json")
		if err := writePlanFileIfChanged(planPath, planData); err != nil {
			return fmt.Errorf("save checkpoint plan: %w", err)
		}
	}
	return surgeon.WriteFileAtomic(target, data, 0644)
}

func writePlanFileIfChanged(planPath string, planData []byte) error {
	sum := sha256.Sum256(planData)
	if prev, ok := planHashes.Load(planPath); ok && prev.([sha256.Size]byte) == sum {
		if _, err := os.Stat(planPath); err == nil {
			return nil
		}
	}
	if err := surgeon.WriteFileAtomic(planPath, planData, 0644); err != nil {
		planHashes.Delete(planPath)
		return err
	}
	planHashes.Store(planPath, sum)
	return nil
}

// LoadCheckpoint reads a checkpoint by plan ID.
func LoadCheckpoint(workspace, planID string) (*PlanCheckpoint, error) {
	target, err := checkpointPath(workspace, planID, ".checkpoint.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	var cp PlanCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	return &cp, nil
}

// FindLatestCheckpoint scans `.apw/plans/` for the most recent unfinished checkpoint.
func FindLatestCheckpoint(workspace string) (*PlanCheckpoint, error) {
	dir := filepath.Join(workspace, ".apw", "plans")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var latestName string
	var latestMod time.Time

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".checkpoint.json") {
			info, err := e.Info()
			if err == nil && info.ModTime().After(latestMod) {
				latestMod = info.ModTime()
				latestName = e.Name()
			}
		}
	}

	if latestName == "" {
		return nil, os.ErrNotExist
	}

	planID := strings.TrimSuffix(latestName, ".checkpoint.json")
	return LoadCheckpoint(workspace, planID)
}

// ClearCheckpoint removes the checkpoint file once verified and merged.
func ClearCheckpoint(workspace, planID string) error {
	target, err := checkpointPath(workspace, planID, ".checkpoint.json")
	if err != nil {
		return err
	}
	err = os.Remove(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
