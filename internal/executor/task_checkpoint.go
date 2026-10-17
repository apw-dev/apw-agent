package executor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func CheckpointForTask(root, id string) (*PlanCheckpoint, error) {
	entries, err := os.ReadDir(filepath.Join(root, ".apw", "plans"))
	if err != nil {
		return nil, err
	}
	var found *PlanCheckpoint
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".checkpoint.json") {
			continue
		}
		planID := strings.TrimSuffix(entry.Name(), ".checkpoint.json")
		cp, err := LoadCheckpoint(root, planID)
		if err != nil {
			return nil, err
		}
		if cp.PlanID != planID {
			return nil, errors.New("checkpoint filename identity mismatch")
		}
		if cp.TaskID == id {
			if found != nil {
				return nil, errors.New("multiple checkpoints for task; refusing ambiguous resume")
			}
			found = cp
		}
	}
	if found == nil {
		return nil, errors.New("no checkpoint for this task")
	}
	return found, nil
}
