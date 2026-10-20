package executor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// DefaultSubagentConcurrency is the default maximum number of parallel Surgeon subagents
// spawned for disjoint-file mutation waves.
const DefaultSubagentConcurrency = 4

type subagentSynthesis struct {
	subtaskIndex int
	subtask      architect.Subtask
	fileContent  string
	fileHash     string
	directive    *patchDirective
	err          error
}

// collectDisjointMutationWave identifies a contiguous wave of 2..maxWorkers create subtasks starting at
// start that target strictly distinct files and can be synthesized in parallel by subagents.
// Patch subtasks run sequentially so each step updates ReadContext and saves an exact-step checkpoint
// before the next model request is sent.
func collectDisjointMutationWave(subtasks []architect.Subtask, start, maxWorkers int) []int {
	if maxWorkers <= 1 || start < 0 || start+1 >= len(subtasks) {
		return nil
	}
	if maxWorkers > 8 {
		maxWorkers = 8
	}
	seenFiles := make(map[string]bool)
	var wave []int
	for i := start; i < len(subtasks) && len(wave) < maxWorkers; i++ {
		st := subtasks[i]
		if st.Action != architect.ActionCreate {
			break
		}
		norm := filepath.ToSlash(strings.TrimSpace(st.TargetFile))
		if norm == "" || surgeon.AllowedPath(norm) != nil || seenFiles[norm] {
			break
		}
		seenFiles[norm] = true
		wave = append(wave, i)
	}
	if len(wave) < 2 {
		return nil
	}
	return wave
}

// askDirectiveSafe invokes directive generation with mutex-guarded failover maps so parallel
// Surgeon subagents can await model responses concurrently without data races.
func (e *Executor) askDirectiveSafe(ctx context.Context, prompt, readSnapshot string) (*patchDirective, error) {
	fullPrompt := prompt + InstructionsBlock(e.Workspace) + "\nPreviously read source context:\n" + readSnapshot
	e.directiveMu.Lock()
	candidates := e.directiveCandidates()
	timeout := e.DirectiveTimeout
	e.directiveMu.Unlock()
	if timeout <= 0 {
		timeout = 90 * time.Second
	}

	var failures []error
	for _, cand := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id, group := candidateID(cand), candidateGroup(cand)
		e.directiveMu.Lock()
		if e.Failover.Models == nil {
			e.Failover.Models = make(map[string]time.Time)
		}
		if e.Failover.Groups == nil {
			e.Failover.Groups = make(map[string]time.Time)
		}
		until := e.Failover.Models[id]
		if e.Failover.Groups[group].After(until) {
			until = e.Failover.Groups[group]
		}
		paused := time.Now().Before(until)
		e.directiveMu.Unlock()
		if paused {
			failures = append(failures, fmt.Errorf("%s paused", cand.Model))
			continue
		}

		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		resp, err := cand.Provider.Ask(attemptCtx, core.ModelRequest{
			RequestID: fmt.Sprintf("subagent-req-%d", time.Now().UnixNano()),
			Prompt:    fullPrompt,
			Model:     cand.Model,
		})
		if attemptCtx.Err() != nil {
			err = attemptCtx.Err()
		}
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		if err == nil && resp != nil && resp.Completed && strings.TrimSpace(resp.Content) != "" {
			if dir, parseErr := parsePatchDirective(resp.Content); parseErr == nil {
				e.directiveMu.Lock()
				e.Failover.Preferred = id
				e.directiveMu.Unlock()
				return dir, nil
			} else {
				err = parseErr
			}
		} else if err == nil {
			err = errors.New("model returned empty or incomplete response")
		}
		failures = append(failures, fmt.Errorf("%s: %w", cand.Model, err))
	}
	if len(failures) == 0 {
		return nil, errors.New("no providers available for subagent directive generation")
	}
	return nil, fmt.Errorf("subagent directive candidates failed: %w", errors.Join(failures...))
}

// runSubagentWave spawns concurrent Surgeon subagents for a disjoint-file wave, synthesizes their
// code/patches in parallel, and commits each file atomically in deterministic subtask order.
func (e *Executor) runSubagentWave(
	ctx context.Context,
	base string,
	surg *surgeon.Surgeon,
	subtasks []architect.Subtask,
	wave []int,
	commitStep func(stepIdx int, sr StepReport, path string) error,
) error {
	var targetNames []string
	for _, idx := range wave {
		targetNames = append(targetNames, subtasks[idx].TargetFile)
	}
	e.emit(ProgressEvent{
		Stage:      "subagent_wave",
		StepIndex:  wave[0] + 1,
		TotalSteps: len(subtasks),
		Message:    fmt.Sprintf("⚡ Spawning %d parallel Surgeon Subagents for disjoint files: %s", len(wave), strings.Join(targetNames, ", ")),
	})

	readSnapshot := e.ReadContext
	results := make([]subagentSynthesis, len(wave))
	var wg sync.WaitGroup
	for wIdx, subIdx := range wave {
		wg.Add(1)
		go func(slot, idx int) {
			defer wg.Done()
			st := subtasks[idx]
			res := subagentSynthesis{subtaskIndex: idx, subtask: st}
			switch st.Action {
			case architect.ActionCreate:
				prompt := fmt.Sprintf("Create file %s.\nInstruction: %s\nRespond ONLY with JSON: {\"file_content\":\"...\"}", st.TargetFile, st.Instruction)
				res.directive, res.err = e.askDirectiveSafe(ctx, prompt, readSnapshot)
			case architect.ActionPatch:
				content, hash, err := surg.ReadWorkspaceFile(base, st.TargetFile)
				if err != nil {
					res.err = err
					break
				}
				res.fileContent = content
				res.fileHash = hash
				prompt := fmt.Sprintf("Provide surgical patch for %s.\nInstruction: %s\nCurrent File Content:\n[source]\n%s\n[/source]\nRespond ONLY with JSON: {\"old_content\":\"...\",\"new_content\":\"...\"}", st.TargetFile, st.Instruction, content)
				res.directive, res.err = e.askDirectiveSafe(ctx, prompt, readSnapshot)
			}
			results[slot] = res
		}(wIdx, subIdx)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return err
	}

	// Deterministic atomic commit in plan order
	for _, res := range results {
		st := res.subtask
		idx := res.subtaskIndex
		e.emit(ProgressEvent{
			Stage:      "step_start",
			StepIndex:  idx + 1,
			TotalSteps: len(subtasks),
			Message:    fmt.Sprintf("[SUBAGENT · %s] %s — %s", st.Action, st.ID, st.TargetFile),
		})
		sr := StepReport{SubtaskID: st.ID, Action: st.Action, TargetFile: st.TargetFile}
		if res.err != nil {
			sr.Message = res.err.Error()
			_ = commitStep(idx, sr, "")
			return res.err
		}

		var stepErr error
		switch st.Action {
		case architect.ActionCreate:
			content := res.directive.FileContent
			if content == "" {
				content = res.directive.NewContent
			}
			content = surgeon.FormatGoIfValid(st.TargetFile, content)
			before, err := surgeon.StateInRoot(base, st.TargetFile)
			if err != nil {
				stepErr = err
				break
			}
			if before.Exists {
				stepErr = errors.New("create target already exists")
				break
			}
			if e.mutation != nil {
				if err := e.mutation(st.TargetFile, before, surgeon.FileState{Exists: true, Hash: surgeon.CalculateHash(content), Mode: 0o644}); err != nil {
					stepErr = err
					break
				}
			}
			stepErr = surgeon.WriteInRoot(base, st.TargetFile, []byte(content), 0o644, true)

		case architect.ActionPatch:
			e.rememberRead(st.TargetFile, res.fileContent)
			_, err := surg.PatchWorkspaceFile(base, st.TargetFile, surgeon.PatchOperation{
				BaseHash:   res.fileHash,
				OldContent: res.directive.OldContent,
				NewContent: res.directive.NewContent,
			}, func(before, after surgeon.FileState) error {
				if e.mutation != nil {
					return e.mutation(st.TargetFile, before, after)
				}
				return nil
			})
			if err != nil && retryablePatch(err) {
				// Fallback to interactive patch retry if the first parallel synthesis did not match uniquely
				prompt := fmt.Sprintf("Provide surgical patch for %s.\nInstruction: %s\nCurrent File Content:\n[source]\n%s\n[/source]\nRespond ONLY with JSON: {\"old_content\":\"...\",\"new_content\":\"...\"}", st.TargetFile, st.Instruction, res.fileContent)
				_, err = e.patchWithRetry(ctx, base, surg, st.TargetFile, res.fileHash, false, prompt)
			}
			stepErr = err
		}

		if stepErr != nil {
			sr.Message = stepErr.Error()
			_ = commitStep(idx, sr, "")
			return stepErr
		}
		sr.Success = true
		sr.Message = fmt.Sprintf("%s %s (parallel subagent)", st.Action, st.TargetFile)
		if err := commitStep(idx, sr, st.TargetFile); err != nil {
			return err
		}
	}
	return nil
}
