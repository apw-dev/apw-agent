package executor

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

var diagFilePattern = regexp.MustCompile(`([A-Za-z0-9_./\\-]+\.[A-Za-z0-9]+):(\d+)`)

// extractDiagnosticFiles finds workspace-relative file paths referenced in compiler/test diagnostics.
func extractDiagnosticFiles(base, diagnostics string) []string {
	matches := diagFilePattern.FindAllStringSubmatch(diagnostics, 20)
	var out []string
	seen := make(map[string]bool)
	for _, m := range matches {
		raw := filepath.ToSlash(strings.TrimSpace(m[1]))
		raw = strings.TrimPrefix(raw, "./")
		if raw == "" || seen[raw] {
			continue
		}
		if err := surgeon.AllowedPath(raw); err != nil {
			continue
		}
		st, err := surgeon.StateInRoot(base, raw)
		if err != nil || !st.Exists {
			continue
		}
		seen[raw] = true
		out = append(out, raw)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

// buildDiagnosticContext formats the contents of files referenced in diagnostics alongside cross-file AST impact.
func buildDiagnosticContext(base string, surg *surgeon.Surgeon, primaryTarget string, diagFiles []string, modified []string) string {
	var sb strings.Builder
	shown := map[string]bool{filepath.ToSlash(primaryTarget): true}
	for _, f := range diagFiles {
		if shown[f] {
			continue
		}
		shown[f] = true
		content, _, err := surg.ReadWorkspaceFile(base, f)
		if err != nil || content == "" {
			continue
		}
		fmt.Fprintf(&sb, "\nReferenced diagnostic file %s:\n[source]\n%s\n[/source]\n", f, headUTF8(content, 12*1024))
	}
	if g, err := surgeon.InspectWorkspaceGraph(base); err == nil && g != nil {
		if impact := g.ImpactOfFiles(modified); impact != "" {
			sb.WriteString("\n")
			sb.WriteString(impact)
		}
	}
	return sb.String()
}

// replanWithArchitect asks the Architect to synthesize a recovery ExecutionPlan when initial subtasks or repairs fail verification.
func (e *Executor) replanWithArchitect(ctx context.Context, base, instruction string, prevPlan *architect.ExecutionPlan, diagnostics string, modified []string) (*architect.ExecutionPlan, error) {
	if len(e.architectCandidates()) == 0 {
		return nil, fmt.Errorf("no architect configured for iterative replanning")
	}
	e.emit(ProgressEvent{
		Stage:   "replanning",
		Message: "Iterative Re-Planning: synthesizing recovery subtasks from verification diagnostics",
	})

	var contextBuilder strings.Builder
	contextBuilder.WriteString("ITERATIVE RECOVERY RE-PLANNING CONTEXT\n")
	contextBuilder.WriteString("Previous Plan Objective: " + prevPlan.Objective + "\n")
	contextBuilder.WriteString("Already Modified Files: " + strings.Join(modified, ", ") + "\n")
	contextBuilder.WriteString("\nVerification Failure Diagnostics:\n" + headUTF8(diagnostics, 16*1024) + "\n")

	if g, err := surgeon.InspectWorkspaceGraph(base); err == nil && g != nil {
		if impact := g.ImpactOfFiles(modified); impact != "" {
			contextBuilder.WriteString("\n" + impact + "\n")
		}
	}
	contextBuilder.WriteString("\n" + ProjectOverview(base, e.Profile(base)))

	task := core.Task{
		ID:          e.TaskID + "-replan",
		Workspace:   base,
		Instruction: instruction + "\n\nFix the verification failure shown in the diagnostics while preserving the objective.",
		Mode:        string(e.Mode),
	}

	var recoveryPlan *architect.ExecutionPlan
	err := e.withArchitectFailover(ctx, "replan", func(ctx context.Context, c FailoverCandidate) error {
		a := architect.NewArchitect(c.Provider)
		a.Model = c.Model
		p, err := a.Plan(ctx, task, contextBuilder.String())
		if err != nil {
			return err
		}
		recoveryPlan = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, st := range recoveryPlan.Subtasks {
		if st.Action != architect.ActionVerify {
			if err := surgeon.AllowedPath(st.TargetFile); err != nil {
				return nil, err
			}
		}
	}
	return recoveryPlan, nil
}
