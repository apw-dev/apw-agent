package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// StepReport records the result of executing a single subtask.
type StepReport struct {
	SubtaskID  string                  `json:"subtask_id"`
	Action     architect.SubtaskAction `json:"action"`
	TargetFile string                  `json:"target_file,omitempty"`
	Success    bool                    `json:"success"`
	Message    string                  `json:"message"`
}

// ExecutionReport summarizes the full Triad loop (Architect -> Surgeon -> Reflex).
type ExecutionReport struct {
	TaskID         string                   `json:"task_id"`
	Plan           *architect.ExecutionPlan `json:"plan"`
	Steps          []StepReport             `json:"steps"`
	ReflexAttempts []reflex.Attempt         `json:"reflex_attempts,omitempty"`
	Passed         bool                     `json:"passed"`
	Duration       time.Duration            `json:"duration"`
}

// Executor coordinates Architect planning, Surgeon patching, and Reflex self-healing verification.
type Executor struct {
	Workspace         string
	Architect         *architect.Architect
	Surgeon           *surgeon.Surgeon
	Reflex            *reflex.ReflexEngine
	ReflexProvider    provider.Provider
	ReflexModel       string
	SkipReflexOnEmpty bool
}

// New creates a new Triad Executor.
func New(workspace string, archProv, reflexProv provider.Provider, archModel, reflexModel string, maxRepairs int) *Executor {
	if workspace == "" {
		workspace = "."
	}
	arch := architect.NewArchitect(archProv)
	arch.Model = archModel

	if reflexProv == nil {
		reflexProv = archProv
	}
	return &Executor{
		Workspace:      workspace,
		Architect:      arch,
		Surgeon:        surgeon.NewSurgeon(),
		Reflex:         reflex.NewReflexEngine(workspace, maxRepairs),
		ReflexProvider: reflexProv,
		ReflexModel:    reflexModel,
	}
}

type patchDirective struct {
	OldContent  string `json:"old_content"`
	NewContent  string `json:"new_content"`
	FileContent string `json:"file_content,omitempty"`
	TargetFile  string `json:"target_file,omitempty"`
}

// Execute runs the full Triad pipeline: Plan -> Apply Subtasks -> Verify & Self-Heal.
func (e *Executor) Execute(ctx context.Context, instruction string, codebaseOverview string) (*ExecutionReport, error) {
	start := time.Now()

	task := core.Task{
		ID:          fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Workspace:   e.Workspace,
		Instruction: instruction,
		Mode:        "execute",
		Status:      core.StatusPlanning,
		CreatedAt:   start,
		UpdatedAt:   start,
	}

	plan, err := e.Architect.Plan(ctx, task, codebaseOverview)
	if err != nil {
		return nil, fmt.Errorf("architect planning failed: %w", err)
	}

	report := &ExecutionReport{
		TaskID: task.ID,
		Plan:   plan,
	}

	var modifiedFiles []string
	for _, st := range plan.Subtasks {
		if err := ctx.Err(); err != nil {
			return report, err
		}

		sr, modFile, stepErr := e.executeSubtask(ctx, st)
		report.Steps = append(report.Steps, sr)
		if stepErr != nil {
			return report, stepErr
		}
		if modFile != "" {
			modifiedFiles = append(modifiedFiles, modFile)
		}
	}

	// Run Reflex verification and autonomous repair loop
	attempts, err := e.Reflex.Run(ctx, func(ctx context.Context, diagnostics string) error {
		return e.repairWithReflex(ctx, diagnostics, modifiedFiles)
	})
	report.ReflexAttempts = attempts
	report.Duration = time.Since(start)
	if err != nil {
		return report, err
	}
	if len(attempts) > 0 && attempts[len(attempts)-1].Result.Passed {
		report.Passed = true
	}
	return report, nil
}

func (e *Executor) executeSubtask(ctx context.Context, st architect.Subtask) (StepReport, string, error) {
	sr := StepReport{
		SubtaskID:  st.ID,
		Action:     st.Action,
		TargetFile: st.TargetFile,
	}

	switch st.Action {
	case architect.ActionVerify:
		sr.Success = true
		sr.Message = "verification scheduled for Reflex stage"
		return sr, "", nil

	case architect.ActionRead:
		fullPath, err := e.safePath(st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		_, hash, err := e.Surgeon.ReadFile(fullPath)
		if err != nil {
			return sr, "", fmt.Errorf("subtask %s read failed: %w", st.ID, err)
		}
		sr.Success = true
		sr.Message = fmt.Sprintf("read file (sha256: %s...)", shortHash(hash))
		return sr, "", nil

	case architect.ActionCreate:
		fullPath, err := e.safePath(st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		dir, err := e.askDirective(ctx, fmt.Sprintf(
			"Create file %s.\nInstruction: %s\nRespond with JSON: {\"file_content\": \"...\"}",
			st.TargetFile, st.Instruction,
		))
		if err != nil {
			return sr, "", fmt.Errorf("subtask %s create query failed: %w", st.ID, err)
		}
		content := dir.FileContent
		if content == "" {
			content = dir.NewContent
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return sr, "", err
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return sr, "", err
		}
		_, _, _ = e.Surgeon.ReadFile(fullPath)
		sr.Success = true
		sr.Message = "created file"
		return sr, fullPath, nil

	case architect.ActionPatch:
		fullPath, err := e.safePath(st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		currentContent, baseHash, err := e.Surgeon.ReadFile(fullPath)
		if err != nil {
			return sr, "", fmt.Errorf("subtask %s read before patch failed: %w", st.ID, err)
		}

		dir, err := e.askDirective(ctx, fmt.Sprintf(
			"Provide surgical patch for %s.\nInstruction: %s\nCurrent File Content:\n```\n%s\n```\nRespond ONLY with JSON: {\"old_content\": \"...\", \"new_content\": \"...\"}",
			st.TargetFile, st.Instruction, currentContent,
		))
		if err != nil {
			return sr, "", fmt.Errorf("subtask %s patch query failed: %w", st.ID, err)
		}

		newHash, err := e.Surgeon.ApplyPatch(surgeon.PatchOperation{
			Path:       fullPath,
			BaseHash:   baseHash,
			OldContent: dir.OldContent,
			NewContent: dir.NewContent,
		})
		if err != nil {
			return sr, "", fmt.Errorf("subtask %s ApplyPatch failed: %w", st.ID, err)
		}
		sr.Success = true
		sr.Message = fmt.Sprintf("patched file (new sha256: %s...)", shortHash(newHash))
		return sr, fullPath, nil

	case architect.ActionDelete:
		fullPath, err := e.safePath(st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return sr, "", fmt.Errorf("subtask %s delete failed: %w", st.ID, err)
		}
		sr.Success = true
		sr.Message = "deleted file"
		return sr, fullPath, nil

	default:
		return sr, "", fmt.Errorf("unsupported subtask action: %s", st.Action)
	}
}

func (e *Executor) repairWithReflex(ctx context.Context, diagnostics string, modifiedFiles []string) error {
	if e.ReflexProvider == nil {
		return errors.New("reflex provider not configured")
	}

	targetPath := ""
	if len(modifiedFiles) > 0 {
		targetPath = modifiedFiles[len(modifiedFiles)-1]
	}

	var fileContext string
	var baseHash string
	if targetPath != "" {
		content, h, err := e.Surgeon.ReadFile(targetPath)
		if err == nil {
			fileContext = content
			baseHash = h
		}
	}

	prompt := fmt.Sprintf(
		"You are Brain 1 (Reflex) in the APW Triad Engine.\nFix the verification failure surgically.\n\n%s\n\nCurrent file (%s):\n```\n%s\n```\nRespond ONLY with JSON: {\"target_file\": \"...\", \"old_content\": \"...\", \"new_content\": \"...\"}",
		diagnostics, targetPath, fileContext,
	)

	dir, err := e.askDirectiveWithProvider(ctx, e.ReflexProvider, e.ReflexModel, prompt)
	if err != nil {
		return err
	}

	if dir.TargetFile != "" {
		resolved, err := e.safePath(dir.TargetFile)
		if err != nil {
			return err
		}
		targetPath = resolved
		_, baseHash, err = e.Surgeon.ReadFile(targetPath)
		if err != nil {
			return err
		}
	}
	if targetPath == "" {
		return errors.New("reflex repair: no target file identified for patch")
	}

	_, err = e.Surgeon.ApplyPatch(surgeon.PatchOperation{
		Path:       targetPath,
		BaseHash:   baseHash,
		OldContent: dir.OldContent,
		NewContent: dir.NewContent,
	})
	return err
}

func (e *Executor) askDirective(ctx context.Context, prompt string) (*patchDirective, error) {
	return e.askDirectiveWithProvider(ctx, e.Architect.Provider, e.Architect.Model, prompt)
}

func (e *Executor) askDirectiveWithProvider(ctx context.Context, p provider.Provider, model, prompt string) (*patchDirective, error) {
	res, err := p.Ask(ctx, core.ModelRequest{
		RequestID: fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Prompt:    prompt,
		Model:     model,
	})
	if err != nil {
		return nil, err
	}

	raw := res.Content
	jsonStr := raw
	if idx := strings.Index(raw, "```json"); idx != -1 {
		rest := raw[idx+7:]
		if endIdx := strings.Index(rest, "```"); endIdx != -1 {
			jsonStr = rest[:endIdx]
		}
	} else if idx := strings.Index(raw, "```"); idx != -1 {
		rest := raw[idx+3:]
		if endIdx := strings.Index(rest, "```"); endIdx != -1 {
			jsonStr = rest[:endIdx]
		}
	}
	jsonStr = strings.TrimSpace(jsonStr)

	var dir patchDirective
	if err := json.Unmarshal([]byte(jsonStr), &dir); err != nil {
		return nil, fmt.Errorf("parse patch directive JSON: %w", err)
	}
	return &dir, nil
}

func (e *Executor) safePath(rel string) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("executor: target_file %q escapes workspace", rel)
	}
	return filepath.Join(e.Workspace, rel), nil
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
