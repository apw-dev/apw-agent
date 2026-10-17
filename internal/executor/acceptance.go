package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/browser"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

type CheckResult struct {
	Criterion string `json:"criterion"`
	Passed    bool   `json:"passed"`
	Evidence  string `json:"evidence"`
}

// pageVisitor renders one page of the worktree's build output for "browser" acceptance checks and
// returns its visible text, or nil when the project has no browser stage.
type pageVisitor func(path string) (*browser.Page, error)

func acceptance(root string, plan *architect.ExecutionPlan, verified bool) ([]CheckResult, bool) {
	return acceptanceSnapshot(root, plan, verified, nil, nil, false)
}

// pageVisitor returns a visitor for root's browser stage, or nil when there is none.
func (e *Executor) pageVisitor(ctx context.Context, root string) pageVisitor {
	stage, ok := e.Profile(root).BrowserStage()
	if !ok {
		return nil
	}
	dir := filepath.Join(root, filepath.FromSlash(stage.Dir))
	return func(path string) (*browser.Page, error) {
		return browser.Visit(ctx, browser.Options{Dir: dir, Path: path, Offline: e.Sandbox.Active()})
	}
}

// acceptanceSnapshot evaluates the plan's checks. skipCommands (unverified mode) treats checks that
// need repository commands (verification, browser) as skipped rather than failed.
func acceptanceSnapshot(root string, plan *architect.ExecutionPlan, verified bool, wt *surgeon.Worktree, visit pageVisitor, skipCommands bool) ([]CheckResult, bool) {
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
			var data []byte
			var err error
			if check.Kind != "browser" {
				data, _, err = surgeon.ReadInRoot(root, check.File)
			}
			ok := false
			evidence := ""
			switch {
			case skipCommands && (check.Kind == "verification" || check.Kind == "browser"):
				ok = true
				evidence = "SKIPPED (unverified mode runs no build, test or browser): " + check.Kind + " " + check.Value
			case check.Kind == "verification":
				ok = verified
				evidence = "project verification pipeline (" + check.Value + ")"
			case check.Kind == "unchanged":
				if wt != nil {
					state, stateErr := surgeon.StateInRoot(root, check.File)
					ok = stateErr == nil && state == wt.Baseline[filepath.Clean(check.File)]
				}
				evidence = "original snapshot comparison: " + check.File
			case check.Kind == "only_changes":
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

			case check.Kind == "browser":
				page := check.File
				if page == "" {
					page = "/"
				}
				evidence = "headless browser render of " + page
				switch {
				case !verified:
					evidence += ": skipped because verification did not pass"
				case visit == nil:
					evidence += ": unavailable (no browser stage for this project)"
				default:
					rendered, visitErr := visit(page)
					if visitErr != nil {
						evidence += ": " + visitErr.Error()
						break
					}
					ok = strings.Contains(rendered.Text, strings.Join(strings.Fields(check.Value), " "))
					evidence += fmt.Sprintf(": visible text %q", clip(rendered.Text, 160))
					if len(rendered.Errors) > 0 {
						ok = false
						evidence += "; uncaught errors: " + strings.Join(rendered.Errors, " | ")
					}
				}
			case check.Kind == "file_exists":
				ok = err == nil
			case check.Kind == "file_absent":
				ok = errors.Is(err, os.ErrNotExist)
			case check.Kind == "contains":
				ok = err == nil && strings.Contains(string(data), check.Value)
			case check.Kind == "not_contains":
				ok = err == nil && !strings.Contains(string(data), check.Value)
			case check.Kind == "go_symbol":
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
				result.Passed = verified || skipCommands
				result.Evidence = "project verification pipeline result"
				if skipCommands {
					result.Evidence = "SKIPPED (unverified mode runs no build or test)"
				}
			}
		}
		all = all && result.Passed
		results = append(results, result)
	}
	return results, all
}

// clip shortens s to at most n runes for evidence text.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "..."
}
