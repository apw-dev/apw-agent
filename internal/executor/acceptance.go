package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

type CheckResult struct {
	Criterion string `json:"criterion"`
	Passed    bool   `json:"passed"`
	Evidence  string `json:"evidence"`
}

func acceptance(root string, plan *architect.ExecutionPlan, verified bool) ([]CheckResult, bool) {
	return acceptanceSnapshot(root, plan, verified, nil)
}
func acceptanceSnapshot(root string, plan *architect.ExecutionPlan, verified bool, wt *surgeon.Worktree) ([]CheckResult, bool) {
	var results []CheckResult
	all := true
	for _, criterion := range plan.AcceptanceCriteria {
		result := CheckResult{Criterion: criterion, Evidence: "No executable acceptance check was provided"}
		count := 0
		passed := true
		for _, check := range plan.Checks {
			if check.Criterion != criterion {
				continue
			}
			count++
			data, _, err := surgeon.ReadInRoot(root, check.File)
			ok := false
			evidence := ""
			switch check.Kind {
			case "verification":
				ok = verified
				evidence = "fixed fmt/vet/build/test pipeline: " + check.Value
			case "unchanged":
				if wt != nil {
					state, stateErr := surgeon.StateInRoot(root, check.File)
					ok = stateErr == nil && state == wt.Baseline[filepath.Clean(check.File)]
				}
				evidence = "original snapshot comparison: " + check.File
			case "only_changes":
				if wt != nil {
					changes, scanErr := wt.ChangedFiles(context.Background())
					allowed := map[string]bool{}
					for _, file := range check.Files {
						allowed[filepath.Clean(file)] = true
					}
					ok = scanErr == nil
					for _, file := range changes {
						ok = ok && allowed[filepath.Clean(file)]
					}
					evidence = "actual changed files: " + strings.Join(changes, ", ")
				}

			case "file_exists":
				ok = err == nil
			case "file_absent":
				ok = errors.Is(err, os.ErrNotExist)
			case "contains":
				ok = err == nil && strings.Contains(string(data), check.Value)
			case "not_contains":
				ok = err == nil && !strings.Contains(string(data), check.Value)
			case "go_symbol":
				if err == nil {
					syms, e := surgeon.InspectWorkspaceSymbols(root, check.File)
					if e == nil {
						for _, sym := range syms {
							if sym.Name == check.Value {
								ok = true
							}
						}
					}
				}
			}
			passed = passed && ok
			if evidence == "" {
				evidence = check.Kind + " " + check.File + " (content hash " + surgeon.CalculateHash(string(data)) + ")"
			}
			if result.Evidence == "No executable acceptance check was provided" {
				result.Evidence = ""
			}
			if result.Evidence != "" {
				result.Evidence += "; "
			}
			result.Evidence += evidence
		}
		if count > 0 {
			result.Passed = passed
		} else {
			lower := strings.ToLower(criterion)
			if lower == "builds cleanly" || lower == "builds" || strings.Contains(lower, "gofmt") && strings.Contains(lower, "go test") {
				result.Passed = verified
				result.Evidence = "fmt/vet/build/test verification result"
			}
		}
		all = all && result.Passed
		results = append(results, result)
	}
	return results, all
}
