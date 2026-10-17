package surgeon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/apiwat/apw-agent/internal/workspace"
)

type applyEntry struct {
	Path       string    `json:"path"`
	Before     FileState `json:"before"`
	BeforeData []byte    `json:"before_data,omitempty"`
	After      FileState `json:"after"`
	AfterData  []byte    `json:"after_data,omitempty"`
}
type applyJournal struct {
	TaskID  string       `json:"task_id"`
	State   string       `json:"state"`
	Entries []applyEntry `json:"entries"`
}

func journalPath(root, id string) (string, error) {
	if id == "" || !filepath.IsLocal(id) || filepath.Base(id) != id || id == "." {
		return "", errors.New("invalid transaction ID")
	}
	return filepath.Join(root, ".apw", "transactions", id+".json"), nil
}
func saveJournal(path string, j applyJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data, 0600)
}

func Applied(root, id string) bool {
	path, err := journalPath(root, id)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var j applyJournal
	return json.Unmarshal(data, &j) == nil && j.State == "committed" && j.TaskID == id
}

func restoreJournal(root string, j applyJournal) error {
	var failures []error
	for i := len(j.Entries) - 1; i >= 0; i-- {
		e := j.Entries[i]
		current, _, err := readState(root, e.Path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if current == e.Before {
			continue
		}
		if current != e.After {
			failures = append(failures, fmt.Errorf("%w during rollback: %s", ErrWorkspaceConflict, e.Path))
			continue
		}
		if e.Before.Exists {
			err = WriteInRoot(root, e.Path, e.BeforeData, e.Before.Mode, false)
		} else {
			err = RemoveInRoot(root, e.Path)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// RecoverTransactions rolls back interrupted apply sets without overwriting later user edits.
func RecoverTransactions(root string) error {
	lock, err := workspace.Acquire(root, "write")
	if err != nil {
		return err
	}
	defer lock.Close()
	entries, err := os.ReadDir(filepath.Join(root, ".apw", "transactions"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(root, ".apw", "transactions", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var j applyJournal
		if err = json.Unmarshal(data, &j); err != nil {
			return err
		}
		if j.State == "committed" || j.State == "rolled_back" {
			continue
		}
		if err = restoreJournal(root, j); err != nil {
			return fmt.Errorf("recover %s: %w", path, err)
		}
		j.State = "rolled_back"
		if err = saveJournal(path, j); err != nil {
			return err
		}
	}
	return nil
}

// ApplyToWorkspace checks the entire set before writing, then journals and rolls back failures.
func (w *Worktree) ApplyToWorkspace(modifiedFiles []string) error {
	return w.ApplyToWorkspaceContext(context.Background(), modifiedFiles)
}

func (w *Worktree) ApplyToWorkspaceContext(ctx context.Context, modifiedFiles []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cleanedUp {
		return errors.New("worktree already cleaned up")
	}
	if w.Baseline == nil {
		return errors.New("worktree snapshot missing; refusing unguarded apply")
	}
	lock, err := workspace.Acquire(w.Workspace, "write")
	if err != nil {
		return err
	}
	defer lock.Close()
	if Applied(w.Workspace, w.TaskID) {
		return nil
	}
	head, err := GetHeadCommit(context.Background(), w.Workspace)
	if err != nil {
		return err
	}
	if head != w.BaseCommit {
		return fmt.Errorf("%w: HEAD revision changed", ErrWorkspaceConflict)
	}
	paths := append([]string{}, modifiedFiles...)
	sort.Strings(paths)
	j := applyJournal{TaskID: w.TaskID, State: "prepared"}
	seen := map[string]bool{}
	for _, rel := range paths {
		if filepath.IsAbs(rel) {
			r, err := filepath.Rel(w.Path, rel)
			if err != nil {
				return err
			}
			rel = r
		}
		rel = filepath.Clean(rel)
		if err := AllowedPath(rel); err != nil {
			return err
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		current, oldData, err := readState(w.Workspace, rel)
		if err != nil {
			return err
		}
		if current != w.Baseline[rel] {
			return fmt.Errorf("%w: %s", ErrWorkspaceConflict, rel)
		}
		after, newData, err := readState(w.Path, rel)
		if err != nil {
			return err
		}
		if current != after {
			j.Entries = append(j.Entries, applyEntry{Path: rel, Before: current, BeforeData: oldData, After: after, AfterData: newData})
		}
	}
	path, err := journalPath(w.Workspace, w.TaskID)
	if err != nil {
		return err
	}
	if err = saveJournal(path, j); err != nil {
		return err
	}
	for _, e := range j.Entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, restoreJournal(w.Workspace, j))
		}
		current, _, checkErr := readState(w.Workspace, e.Path)
		if checkErr != nil {
			err = checkErr
		} else if current != e.Before {
			err = fmt.Errorf("%w: %s", ErrWorkspaceConflict, e.Path)
		} else if e.After.Exists {
			err = WriteInRoot(w.Workspace, e.Path, e.AfterData, e.After.Mode, !e.Before.Exists)
		} else {
			err = RemoveInRoot(w.Workspace, e.Path)
		}
		if err != nil {
			rollbackErr := restoreJournal(w.Workspace, j)
			if rollbackErr == nil {
				j.State = "rolled_back"
				rollbackErr = saveJournal(path, j)
			}
			return errors.Join(fmt.Errorf("apply %s: %w", e.Path, err), rollbackErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, restoreJournal(w.Workspace, j))
	}
	j.State = "committed"
	// Keep a durable receipt to distinguish a crash after apply from an unapplied task.
	if err = saveJournal(path, j); err != nil {
		return errors.Join(err, restoreJournal(w.Workspace, j))
	}
	for i := range j.Entries {
		j.Entries[i].BeforeData = nil
		j.Entries[i].AfterData = nil
	}
	_ = saveJournal(path, j)
	return nil
}
