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

// readAcceptanceFile reads a workspace file for acceptance checking, falling back across common
// env-template and deploy-directory aliases when a task documents a path deviation.
func readAcceptanceFile(root, rel string) ([]byte, os.FileMode, error) {
	data, mode, err := surgeon.ReadInRoot(root, rel)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return data, mode, err
	}
	slash := filepath.ToSlash(strings.TrimSpace(rel))
	base := strings.ToLower(filepath.Base(slash))
	if base == ".env.example" || base == "example.env" || base == ".env.sample" {
		for _, alt := range []string{".env.example", "example.env", "deploy/example.env", "deploy/.env.example"} {
			if d, m, altErr := surgeon.ReadInRoot(root, alt); altErr == nil {
				return d, m, nil
			}
		}
	}
	if strings.HasPrefix(slash, "deploy/") {
		if d, m, altErr := surgeon.ReadInRoot(root, strings.TrimPrefix(slash, "deploy/")); altErr == nil {
			return d, m, nil
		}
	} else if !strings.Contains(slash, "/") {
		if d, m, altErr := surgeon.ReadInRoot(root, "deploy/"+slash); altErr == nil {
			return d, m, nil
		}
	}
	return data, mode, err
}

func contentContains(content, needle string) bool {
	if strings.Contains(content, needle) {
		return true
	}
	norm := func(s string) string {
		r := strings.NewReplacer("'", `"`, "`", `"`)
		return strings.Join(strings.Fields(r.Replace(s)), " ")
	}
	nContent := norm(content)
	nNeedle := norm(needle)
	if nNeedle == "" {
		return false
	}
	if strings.Contains(nContent, nNeedle) {
		return true
	}
	return strings.Contains(strings.ToLower(nContent), strings.ToLower(nNeedle))
}

func evalSingleCheck(root string, check architect.AcceptanceCheck, verified bool, wt *surgeon.Worktree, visit pageVisitor, skipCommands bool) (bool, string) {
	var data []byte
	var err error
	if check.Kind != "browser" && check.Kind != "verification" && check.Kind != "only_changes" && check.Kind != "file_absent" {
		data, _, err = readAcceptanceFile(root, check.File)
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
		if surgeon.AllowedPath(check.File) != nil {
			if filepath.IsLocal(check.File) {
				_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(check.File)))
				ok = errors.Is(statErr, os.ErrNotExist)
			}
		} else {
			_, _, readErr := surgeon.ReadInRoot(root, check.File)
			ok = errors.Is(readErr, os.ErrNotExist)
		}
		evidence = "file_absent " + check.File
	case check.Kind == "contains":
		ok = err == nil && contentContains(string(data), check.Value)
		if !ok && strings.ToLower(filepath.Base(filepath.ToSlash(check.File))) == "package.json" {
			for _, alt := range []string{"package.json", "api/package.json", "web/package.json", "backend/package.json", "frontend/package.json"} {
				if altData, _, altErr := surgeon.ReadInRoot(root, alt); altErr == nil && contentContains(string(altData), check.Value) {
					ok = true
					data = altData
					break
				}
			}
		}
	case check.Kind == "not_contains":
		ok = errors.Is(err, os.ErrNotExist) || (err == nil && !strings.Contains(string(data), check.Value))
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
	if evidence == "" {
		evidence = check.Kind + " " + check.File + " (content hash " + surgeon.CalculateHash(string(data)) + ")"
	}
	return ok, evidence
}

func acceptanceHintsForFile(plan *architect.ExecutionPlan, targetFile string) string {
	if plan == nil || targetFile == "" {
		return ""
	}
	cleanTarget := filepath.Clean(filepath.FromSlash(strings.TrimSpace(targetFile)))
	var lines []string
	for _, c := range plan.Checks {
		if filepath.Clean(filepath.FromSlash(strings.TrimSpace(c.File))) != cleanTarget {
			continue
		}
		switch c.Kind {
		case "contains":
			if c.Value != "" {
				lines = append(lines, fmt.Sprintf("- MUST contain exact text: %q (for criterion: %s)", c.Value, c.Criterion))
			}
		case "not_contains":
			if c.Value != "" {
				lines = append(lines, fmt.Sprintf("- MUST NOT contain text: %q anywhere, not even in comments (for criterion: %s)", c.Value, c.Criterion))
			}
		case "go_symbol":
			if c.Value != "" {
				lines = append(lines, fmt.Sprintf("- MUST declare top-level Go symbol: %s (for criterion: %s)", c.Value, c.Criterion))
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "\nMandatory acceptance check requirements for " + targetFile + ":\n" + strings.Join(lines, "\n") + "\n"
}

func removeModifiedFile(files *[]string, target string) {
	if files == nil || target == "" {
		return
	}
	cleanTarget := filepath.Clean(filepath.FromSlash(strings.TrimSpace(target)))
	var kept []string
	for _, f := range *files {
		if filepath.Clean(filepath.FromSlash(strings.TrimSpace(f))) != cleanTarget {
			kept = append(kept, f)
		}
	}
	*files = kept
}

func (e *Executor) restoreBaselineFile(base, targetFile string, wt *surgeon.Worktree, modified *[]string, steps *[]StepReport, save func() error) bool {
	if surgeon.AllowedPath(targetFile) != nil {
		return false
	}
	before, stErr := surgeon.StateInRoot(base, targetFile)
	if stErr != nil {
		return false
	}
	cleanRel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(targetFile)))
	if e.Workspace != "" {
		origData, origMode, origErr := surgeon.ReadInRoot(e.Workspace, cleanRel)
		if origErr == nil {
			afterState := surgeon.FileState{Exists: true, Hash: surgeon.CalculateHash(string(origData)), Mode: origMode}
			if before == afterState {
				removeModifiedFile(modified, targetFile)
				_ = save()
				return true
			}
			if e.mutation != nil {
				_ = e.mutation(targetFile, before, afterState)
			}
			if wErr := surgeon.WriteInRoot(base, cleanRel, origData, origMode, !before.Exists); wErr == nil {
				e.Pending = nil
				removeModifiedFile(modified, targetFile)
				sr := StepReport{SubtaskID: "acceptance-restore", Action: architect.ActionPatch, TargetFile: targetFile, Success: true, Message: "restore " + targetFile + " to unchanged baseline"}
				*steps = append(*steps, sr)
				e.emit(ProgressEvent{Stage: "step_done", Step: &sr, Message: sr.Message})
				_ = save()
				return true
			}
		} else if errors.Is(origErr, os.ErrNotExist) && before.Exists {
			if e.mutation != nil {
				_ = e.mutation(targetFile, before, surgeon.FileState{})
			}
			if rmErr := surgeon.RemoveInRoot(base, cleanRel); rmErr == nil {
				e.Pending = nil
				removeModifiedFile(modified, targetFile)
				sr := StepReport{SubtaskID: "acceptance-restore", Action: architect.ActionDelete, TargetFile: targetFile, Success: true, Message: "remove " + targetFile + " to restore baseline"}
				*steps = append(*steps, sr)
				e.emit(ProgressEvent{Stage: "step_done", Step: &sr, Message: sr.Message})
				_ = save()
				return true
			}
		}
	} else if wt != nil {
		if expected, ok := wt.Baseline[cleanRel]; ok && !expected.Exists && before.Exists {
			if rmErr := surgeon.RemoveInRoot(base, cleanRel); rmErr == nil {
				e.Pending = nil
				removeModifiedFile(modified, targetFile)
				_ = save()
				return true
			}
		}
	}
	return false
}

func (e *Executor) repairUnmetAcceptance(
	ctx context.Context,
	base string,
	surg *surgeon.Surgeon,
	plan *architect.ExecutionPlan,
	wt *surgeon.Worktree,
	modified *[]string,
	steps *[]StepReport,
	save func() error,
) {
	if plan == nil || len(plan.Checks) == 0 {
		return
	}
	positiveChecked := map[string]bool{}
	for _, c := range plan.Checks {
		if c.Kind == "contains" || c.Kind == "not_contains" || c.Kind == "go_symbol" || c.Kind == "file_absent" {
			f := strings.TrimSpace(c.File)
			if f != "" {
				positiveChecked[filepath.Clean(filepath.FromSlash(f))] = true
			}
		}
	}
	for _, c := range plan.Checks {
		if ctx.Err() != nil {
			return
		}
		switch c.Kind {
		case "unchanged":
			f := strings.TrimSpace(c.File)
			if f == "" || positiveChecked[filepath.Clean(filepath.FromSlash(f))] {
				continue
			}
			if ok, _ := evalSingleCheck(base, c, true, wt, nil, true); !ok {
				e.restoreBaselineFile(base, f, wt, modified, steps, save)
			}
		case "only_changes":
			if wt == nil {
				continue
			}
			if ok, _ := evalSingleCheck(base, c, true, wt, nil, true); !ok {
				allowed := map[string]bool{}
				for _, af := range c.Files {
					allowed[filepath.Clean(filepath.FromSlash(strings.TrimSpace(af)))] = true
				}
				changes, scanErr := wt.ChangedFiles(ctx)
				if scanErr == nil {
					for _, chg := range changes {
						cleanChg := filepath.Clean(filepath.FromSlash(chg))
						if !allowed[cleanChg] && !positiveChecked[cleanChg] {
							e.restoreBaselineFile(base, filepath.ToSlash(cleanChg), wt, modified, steps, save)
						}
					}
				}
			}
		}
	}
	var targetOrder []string
	failingByFile := map[string][]architect.AcceptanceCheck{}
	allByFile := map[string][]architect.AcceptanceCheck{}
	for _, c := range plan.Checks {
		if c.Kind != "file_exists" && c.Kind != "file_absent" && c.Kind != "contains" && c.Kind != "not_contains" && c.Kind != "go_symbol" {
			continue
		}
		f := strings.TrimSpace(c.File)
		if f == "" {
			continue
		}
		allByFile[f] = append(allByFile[f], c)
		ok, _ := evalSingleCheck(base, c, true, wt, nil, true)
		if !ok {
			if _, seen := failingByFile[f]; !seen {
				targetOrder = append(targetOrder, f)
			}
			failingByFile[f] = append(failingByFile[f], c)
		}
	}
	if len(targetOrder) == 0 {
		return
	}
	for _, targetFile := range targetOrder {
		if ctx.Err() != nil {
			return
		}
		if surgeon.AllowedPath(targetFile) != nil {
			continue
		}
		failing := failingByFile[targetFile]
		wantAbsent := false
		for _, fc := range failing {
			if fc.Kind == "file_absent" {
				wantAbsent = true
				break
			}
		}
		if wantAbsent {
			before, stErr := surgeon.StateInRoot(base, targetFile)
			if stErr == nil && before.Exists {
				if e.mutation != nil {
					_ = e.mutation(targetFile, before, surgeon.FileState{})
				}
				if rmErr := surgeon.RemoveInRoot(base, targetFile); rmErr == nil {
					e.Pending = nil
					appendModifiedFile(modified, targetFile)
					sr := StepReport{SubtaskID: "acceptance-clean", Action: architect.ActionDelete, TargetFile: targetFile, Success: true, Message: "delete " + targetFile + " (acceptance repair)"}
					*steps = append(*steps, sr)
					e.emit(ProgressEvent{Stage: "step_done", Step: &sr, Message: sr.Message})
					_ = save()
				}
			}
			continue
		}

		subInstruction := ""
		for _, st := range plan.Subtasks {
			if filepath.Clean(filepath.FromSlash(st.TargetFile)) == filepath.Clean(filepath.FromSlash(targetFile)) && st.Instruction != "" {
				subInstruction = st.Instruction
				break
			}
		}
		reqBlock := acceptanceHintsForFile(plan, targetFile)
		content, hash, readErr := surg.ReadWorkspaceFile(base, targetFile)
		e.emit(ProgressEvent{
			Stage:   "step_start",
			Message: fmt.Sprintf("[ACCEPTANCE · repair] %s", targetFile),
		})
		if errors.Is(readErr, os.ErrNotExist) {
			prompt := fmt.Sprintf("Create file %s to satisfy the plan objective and acceptance checks.\nPlan Objective: %s\nInstruction: %s%s\nRespond ONLY with JSON: {\"file_content\":\"...\"}", targetFile, plan.Objective, subInstruction, reqBlock)
			dir, askErr := e.askDirective(ctx, prompt)
			if askErr != nil {
				continue
			}
			newContent := dir.FileContent
			if newContent == "" {
				newContent = dir.NewContent
			}
			if newContent == "" {
				continue
			}
			newContent = surgeon.FormatGoIfValid(targetFile, newContent)
			before, stErr := surgeon.StateInRoot(base, targetFile)
			if stErr != nil {
				continue
			}
			if e.mutation != nil {
				if err := e.mutation(targetFile, before, surgeon.FileState{Exists: true, Hash: surgeon.CalculateHash(newContent), Mode: 0o644}); err != nil {
					continue
				}
			}
			if wErr := surgeon.WriteInRoot(base, targetFile, []byte(newContent), 0o644, !before.Exists); wErr == nil {
				e.Pending = nil
				appendModifiedFile(modified, targetFile)
				sr := StepReport{SubtaskID: "acceptance-create", Action: architect.ActionCreate, TargetFile: targetFile, Success: true, Message: "create " + targetFile + " (acceptance repair)"}
				*steps = append(*steps, sr)
				e.emit(ProgressEvent{Stage: "step_done", Step: &sr, Message: sr.Message})
				_ = save()
			}
		} else if readErr == nil {
			snippet := promptSourceSnippet(targetFile, content)
			prompt := fmt.Sprintf("Update file %s so that every acceptance check on this file passes while preserving its behavior.\nPlan Objective: %s\nInstruction: %s%s\nCurrent File Content:\n[source]\n%s\n[/source]\nRespond ONLY with JSON: {\"old_content\":\"...\",\"new_content\":\"...\"}", targetFile, plan.Objective, subInstruction, reqBlock, snippet)
			if _, pErr := e.patchWithRetry(ctx, base, surg, targetFile, hash, false, prompt); pErr == nil {
				e.Pending = nil
				appendModifiedFile(modified, targetFile)
				sr := StepReport{SubtaskID: "acceptance-patch", Action: architect.ActionPatch, TargetFile: targetFile, Success: true, Message: "patch " + targetFile + " (acceptance repair)"}
				*steps = append(*steps, sr)
				e.emit(ProgressEvent{Stage: "step_done", Step: &sr, Message: sr.Message})
				_ = save()
			} else if !isLockFile(targetFile) {
				fullPrompt := fmt.Sprintf("Rewrite file %s so that every acceptance check on this file passes while preserving its behavior.\nPlan Objective: %s\nInstruction: %s%s\nCurrent File Content:\n[source]\n%s\n[/source]\nRespond ONLY with JSON: {\"file_content\":\"...\"}", targetFile, plan.Objective, subInstruction, reqBlock, snippet)
				if dir, askErr := e.askDirective(ctx, fullPrompt); askErr == nil {
					newContent := dir.FileContent
					if newContent == "" {
						newContent = dir.NewContent
					}
					if newContent != "" {
						newContent = surgeon.FormatGoIfValid(targetFile, newContent)
						before, stErr := surgeon.StateInRoot(base, targetFile)
						if stErr == nil {
							if e.mutation != nil {
								_ = e.mutation(targetFile, before, surgeon.FileState{Exists: true, Hash: surgeon.CalculateHash(newContent), Mode: 0o644})
							}
							if wErr := surgeon.WriteInRoot(base, targetFile, []byte(newContent), 0o644, !before.Exists); wErr == nil {
								e.Pending = nil
								appendModifiedFile(modified, targetFile)
								sr := StepReport{SubtaskID: "acceptance-patch", Action: architect.ActionPatch, TargetFile: targetFile, Success: true, Message: "patch " + targetFile + " (acceptance repair)"}
								*steps = append(*steps, sr)
								e.emit(ProgressEvent{Stage: "step_done", Step: &sr, Message: sr.Message})
								_ = save()
							}
						}
					}
				}
			}
		}
	}
}

// acceptanceSnapshot evaluates the plan's checks. skipCommands (unverified mode) treats checks that
// need repository commands (verification, browser) as skipped rather than failed.
func acceptanceSnapshot(root string, plan *architect.ExecutionPlan, verified bool, wt *surgeon.Worktree, visit pageVisitor, skipCommands bool) ([]CheckResult, bool) {
	var results []CheckResult
	var uncheckedIndices []int
	explicitChecksPassed := true
	for _, criterion := range plan.AcceptanceCriteria {
		result := CheckResult{Criterion: criterion, Evidence: "No executable acceptance check was provided"}
		count := 0
		passed := true
		for _, check := range plan.Checks {
			if check.Criterion != criterion {
				continue
			}
			count++
			ok, evidence := evalSingleCheck(root, check, verified, wt, visit, skipCommands)
			passed = passed && ok
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
			explicitChecksPassed = explicitChecksPassed && passed
		} else {
			lower := strings.ToLower(criterion)
			if lower == "builds cleanly" || lower == "builds" || strings.Contains(lower, "gofmt") && strings.Contains(lower, "go test") {
				result.Passed = verified || skipCommands
				result.Evidence = "project verification pipeline result"
				if skipCommands {
					result.Evidence = "SKIPPED (unverified mode runs no build or test)"
				}
			} else {
				uncheckedIndices = append(uncheckedIndices, len(results))
			}
		}
		results = append(results, result)
	}
	if len(plan.Checks) > 0 {
		for _, idx := range uncheckedIndices {
			results[idx].Passed = explicitChecksPassed
			results[idx].Evidence = "covered by executable plan checks"
		}
	}
	all := true
	for _, r := range results {
		all = all && r.Passed
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
