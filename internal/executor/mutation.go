package executor

import (
	"fmt"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// The intent is durable before changing a file. Resume can recognize a write
// that completed just before a crash but before the step checkpoint was updated.
type MutationIntent struct {
	Step   int               `json:"step"`
	Repair bool              `json:"repair,omitempty"`
	Path   string            `json:"path"`
	Before surgeon.FileState `json:"before"`
	After  surgeon.FileState `json:"after"`
}

func recoverMutation(cp *PlanCheckpoint) error {
	if cp.Pending == nil {
		return nil
	}
	m := cp.Pending
	if m.Step != cp.LastSuccessStep || cp.Plan == nil || m.Step < 0 || m.Step > len(cp.Plan.Subtasks) {
		return fmt.Errorf("invalid pending mutation")
	}
	if m.Repair {
		if m.Step != len(cp.Plan.Subtasks) {
			return fmt.Errorf("invalid pending repair")
		}
	} else if m.Step >= len(cp.Plan.Subtasks) || cp.Plan.Subtasks[m.Step].TargetFile != m.Path {
		return fmt.Errorf("pending mutation does not match the plan step")
	}
	state, err := surgeon.StateInRoot(cp.WorktreePath, m.Path)
	if err != nil {
		return err
	}
	if state == m.After {
		appendModifiedFile(&cp.ModifiedFiles, m.Path)
		if !m.Repair {
			st := cp.Plan.Subtasks[m.Step]
			cp.LastSuccessStep = m.Step + 1
			if cp.Report != nil {
				cp.Report.Steps = append(cp.Report.Steps, StepReport{SubtaskID: st.ID, Action: st.Action, TargetFile: m.Path, Success: true, Message: "Recovered a completed file mutation"})
			}
		}
	} else if state != m.Before {
		return fmt.Errorf("pending file %s differs from both recorded states; manual review required", m.Path)
	}
	cp.Pending = nil
	return nil
}
