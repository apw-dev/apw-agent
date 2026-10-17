package executor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

// TaskCheckpoints maps task IDs to the plan IDs of their checkpoints in `.apw/plans/`.
// Unreadable checkpoints are returned separately so retention can refuse to guess about them.
func TaskCheckpoints(workspace string) (map[string][]string, []string, error) {
	entries, err := os.ReadDir(filepath.Join(workspace, ".apw", "plans"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]string{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	byTask := map[string][]string{}
	var unreadable []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".checkpoint.json") {
			continue
		}
		planID := strings.TrimSuffix(entry.Name(), ".checkpoint.json")
		cp, err := LoadCheckpoint(workspace, planID)
		if err != nil || cp.PlanID != planID || cp.TaskID == "" {
			unreadable = append(unreadable, entry.Name())
			continue
		}
		byTask[cp.TaskID] = append(byTask[cp.TaskID], planID)
	}
	return byTask, unreadable, nil
}

// RemoveCheckpointFiles deletes a plan's checkpoint and saved plan through the rooted state-file guard.
// It returns the removed file names.
func RemoveCheckpointFiles(workspace, planID string) ([]string, error) {
	var removed []string
	for _, suffix := range []string{".checkpoint.json", ".json"} {
		path, err := checkpointPath(workspace, planID, suffix)
		if err != nil {
			return removed, err
		}
		ok, err := surgeon.RemoveStateFile(workspace, filepath.Join(".apw", "plans"), filepath.Base(path))
		if err != nil {
			return removed, err
		}
		planHashes.Delete(path)
		if ok {
			removed = append(removed, filepath.Base(path))
		}
	}
	return removed, nil
}

// CompletedRecoveryExport returns a `.apw/recovery/<task-id>-*` directory whose manifest
// records a finished export for that task, or "" when there is none.
func CompletedRecoveryExport(workspace, id string) (string, error) {
	parent := filepath.Join(workspace, ".apw", "recovery")
	entries, err := os.ReadDir(parent)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	found := ""
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), id+"-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(parent, entry.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var manifest RecoveryExport
		if json.Unmarshal(data, &manifest) == nil && manifest.TaskID == id && manifest.Status == "exported" {
			found = filepath.Join(parent, entry.Name())
		}
	}
	return found, nil
}
