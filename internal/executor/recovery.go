package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/workspace"
)

type RecoveryExport struct {
	TaskID           string                       `json:"task_id"`
	PlanID           string                       `json:"plan_id"`
	Worktree         string                       `json:"worktree"`
	Directory        string                       `json:"directory"`
	BaseCommit       string                       `json:"base_commit"`
	Completed        int                          `json:"completed_steps"`
	Total            int                          `json:"total_steps"`
	Files            map[string]surgeon.FileState `json:"files"`
	CurrentWorkspace map[string]surgeon.FileState `json:"current_workspace"`
	Status           string                       `json:"status"`
	Note             string                       `json:"note"`
}

func RecoverLegacyTask(ctx context.Context, root, id string) (*RecoveryExport, error) {
	lock, err := workspace.Acquire(root, "write")
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	cp, err := CheckpointForTask(root, id)
	if err != nil {
		return nil, err
	}
	if _, err := loadResumePlan(root, *cp); err != nil {
		return nil, err
	}
	if err := validateResumeWorktree(ctx, root, *cp); err != nil {
		return nil, err
	}
	parent := filepath.Join(root, ".apw", "recovery")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	if !filepath.IsLocal(id) || filepath.Base(id) != id || id == "." {
		return nil, fmt.Errorf("invalid task ID")
	}
	dir, err := os.MkdirTemp(parent, id+"-")
	if err != nil {
		return nil, err
	}
	report := &RecoveryExport{TaskID: id, PlanID: cp.PlanID, Worktree: cp.WorktreePath, Directory: dir, Completed: cp.LastSuccessStep, Total: cp.TotalSteps, Status: "preparing", Note: "Export only. Original dirty baseline is unknown for legacy work. Review before preparing a new verified task; source and original worktree are preserved."}
	save := func() error {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		return surgeon.WriteFileAtomic(filepath.Join(dir, "manifest.json"), data, 0600)
	}
	if err := save(); err != nil {
		return nil, err
	}
	filesDir := filepath.Join(dir, "files")
	if err := os.Mkdir(filesDir, 0700); err != nil {
		return nil, err
	}
	report.BaseCommit, report.Files, err = surgeon.ExportDirtyWorktree(ctx, cp.WorktreePath, filesDir)
	if err != nil {
		return nil, fmt.Errorf("incomplete export retained at %s: %w", dir, err)
	}
	report.CurrentWorkspace = map[string]surgeon.FileState{}
	for file := range report.Files {
		state, err := surgeon.StateInRoot(root, file)
		if err != nil {
			return nil, err
		}
		report.CurrentWorkspace[file] = state
	}
	for _, suffix := range []string{".json", ".checkpoint.json"} {
		path, err := checkpointPath(root, cp.PlanID, suffix)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := surgeon.WriteFileAtomic(filepath.Join(dir, cp.PlanID+suffix), data, 0600); err != nil {
			return nil, err
		}
	}
	report.Status = "exported"
	if err := save(); err != nil {
		return nil, err
	}
	return report, nil
}
