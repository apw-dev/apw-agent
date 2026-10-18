package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/provider"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/research"
	"github.com/apiwat/apw-agent/internal/sandbox"
	"github.com/apiwat/apw-agent/internal/session"
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
	TaskID         string                     `json:"task_id"`
	Plan           *architect.ExecutionPlan   `json:"plan"`
	Steps          []StepReport               `json:"steps"`
	ReflexAttempts []reflex.Attempt           `json:"reflex_attempts,omitempty"`
	Passed         bool                       `json:"passed"`
	Duration       time.Duration              `json:"duration"`
	Summary        string                     `json:"summary,omitempty"`
	Mode           core.TaskMode              `json:"mode"`
	WorktreePath   string                     `json:"worktree_path,omitempty"`
	ModifiedFiles  []string                   `json:"modified_files,omitempty"`
	Diff           string                     `json:"diff,omitempty"`
	Baseline       *reflex.VerificationResult `json:"baseline,omitempty"`
	Acceptance     []CheckResult              `json:"acceptance,omitempty"`
	Applied        bool                       `json:"applied"`
	Diagnostics    *DiagnosticComparison      `json:"diagnostic_comparison,omitempty"`
	Research       []research.Source          `json:"research,omitempty"`
	// Unverified is set for unverified-mode tasks: no repository command checked the change.
	Unverified bool `json:"unverified,omitempty"`
	// Warnings are non-blocking findings for the reviewer, such as tests that may only mirror the
	// implementation.
	Warnings []string `json:"warnings,omitempty"`
}

// ProgressEvent describes a real-time progress update during Triad execution.
type ProgressEvent struct {
	Stage      string // "planning", "plan_ready", "step_start", "step_done", "verifying", "verified", "synthesizing", "done"
	Message    string
	StepIndex  int
	TotalSteps int
	Duration   time.Duration
	Step       *StepReport
	Passed     bool
}

// ProgressCallback receives live progress events as each Triad phase runs.
type ProgressCallback func(event ProgressEvent)

// Executor coordinates Architect planning, Surgeon patching, and Reflex self-healing verification.
type Executor struct {
	Pending              *MutationIntent
	mutation             func(string, surgeon.FileState, surgeon.FileState) error
	RecoveredSteps       []StepReport
	TaskID               string
	Mode                 core.TaskMode
	Trusted              bool
	ReadContext          string
	Phase                string
	VerificationBaseline *reflex.VerificationResult
	VerifiedFiles        map[string]surgeon.FileState
	CurrentReport        *ExecutionReport
	Workspace            string
	Architect            *architect.Architect
	Surgeon              *surgeon.Surgeon
	Reflex               *reflex.ReflexEngine
	ReflexProvider       provider.Provider
	ReflexModel          string
	ArchitectPool        []FailoverCandidate
	ReflexPool           []FailoverCandidate
	DirectiveTimeout     time.Duration
	Failover             FailoverSnapshot
	Session              *session.Session
	SkipReflexOnEmpty    bool
	OnProgress           ProgressCallback
	// VerifyOverride replaces the detected verification pipeline (apw.json verify.stages).
	VerifyOverride []project.Stage
	// WorktreeRoot overrides where task worktrees are created (apw.json worktree_root).
	WorktreeRoot string
	// Sandbox isolates the repository commands verification runs (apw.json sandbox).
	Sandbox sandbox.Policy
	// BrowserCheck is apw.json verify.browser: "off" drops the headless page check.
	BrowserCheck string
	// Research, when set, lets the Architect read allowlisted documentation before planning.
	Research *research.Client
	// ResearchSources lists what the last planning run fetched (text omitted from reports).
	ResearchSources []research.Source
	// Proposals > 1 asks that many architect candidates for plans and ranks them; JudgeProposals
	// also asks the primary architect to rate them.
	Proposals      int
	JudgeProposals bool
	// Lookup is apw.json planning.lookup: "auto" (default) offers read-only code lookup when the
	// code context was truncated, "always" always, "off" never.
	Lookup string
}

// Profile detects the project at root, applying any configured verification override.
func (e *Executor) Profile(root string) project.Profile {
	p := project.Detect(root, e.VerifyOverride)
	if e.BrowserCheck == "off" {
		p = p.WithoutBrowser()
	}
	return p
}

func (e *Executor) emit(ev ProgressEvent) {
	if e.OnProgress != nil {
		e.OnProgress(ev)
	}
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
	var archPool []FailoverCandidate
	if archProv != nil {
		archPool = append(archPool, FailoverCandidate{Provider: archProv, Model: archModel})
	}
	var refPool []FailoverCandidate
	if reflexProv != nil {
		refPool = append(refPool, FailoverCandidate{Provider: reflexProv, Model: reflexModel})
	}
	return &Executor{
		Workspace:      workspace,
		Architect:      arch,
		Surgeon:        surgeon.NewSurgeon(),
		Reflex:         reflex.NewReflexEngine(workspace, maxRepairs),
		ReflexProvider: reflexProv,
		ReflexModel:    reflexModel,
		ArchitectPool:  archPool,
		ReflexPool:     refPool,
	}
}

type patchDirective struct {
	OldContent  string `json:"old_content"`
	NewContent  string `json:"new_content"`
	FileContent string `json:"file_content,omitempty"`
	TargetFile  string `json:"target_file,omitempty"`
}

func (e *Executor) formatPartialReportSummary(instruction string, plan *architect.ExecutionPlan, steps []StepReport, lastStep, totalSteps int, taskErr error) string {
	var sb strings.Builder
	sb.WriteString("### ⚠️ สรุปผลการปฏิบัติการ (การทำงานหยุดชั่วคราว / Partial Execution)\n\n")
	sb.WriteString(fmt.Sprintf("**ภารกิจ:** %s\n\n", instruction))
	sb.WriteString(fmt.Sprintf("**เป้าหมายของแผน:** %s\n\n", plan.Objective))
	sb.WriteString(fmt.Sprintf("**ความคืบหน้า:** ดำเนินการสำเร็จ %d จาก %d ขั้นตอน\n\n", lastStep, totalSteps))
	sb.WriteString("#### 🛠️ ขั้นตอนที่ดำเนินการไปแล้ว:\n")
	for _, s := range steps {
		status := "✅"
		if !s.Success {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("- %s `[%s]` %s: %s\n", status, s.Action, s.SubtaskID, s.Message))
	}
	sb.WriteString(fmt.Sprintf("\n#### ❌ สาเหตุที่หยุดทำงาน:\n`%v`\n\n", taskErr))
	sb.WriteString("#### 💡 คำแนะนำในการทำงานต่อ:\n")
	sb.WriteString("- โค้ดที่ทำสำเร็จได้รับการปกป้องไว้ใน Git Worktree และบันทึก Checkpoint เรียบร้อยแล้ว\n")
	sb.WriteString("- สามารถพิมพ์คำสั่ง `/resume` หรือ `apw resume` เพื่อทำงานต่อจากขั้นตอนที่ค้างไว้ได้ทันทีโดยไม่ต้องเริ่มต้นใหม่\n")
	return sb.String()
}

func (e *Executor) formatReportSummary(instruction string, plan *architect.ExecutionPlan, steps []StepReport, passed bool, duration time.Duration) string {
	var sb strings.Builder
	sb.WriteString("### 📊 รายละเอียดขั้นตอนการทำงานและสถิติ (Execution Details)\n\n")
	sb.WriteString(fmt.Sprintf("**ภารกิจ:** %s\n\n", instruction))
	sb.WriteString(fmt.Sprintf("**เป้าหมายของแผน:** %s\n\n", plan.Objective))
	sb.WriteString("#### 🛠️ ขั้นตอนที่ดำเนินการ:\n")
	reads := 0
	patches := 0
	creates := 0
	for _, s := range steps {
		switch s.Action {
		case architect.ActionRead:
			reads++
		case architect.ActionPatch:
			patches++
		case architect.ActionCreate:
			creates++
		}
		sb.WriteString(fmt.Sprintf("- `[%s]` %s: %s\n", s.Action, s.SubtaskID, s.Message))
	}
	sb.WriteString("\n#### ⚡ ผลการตรวจสอบด้วย Reflex (Closed-Loop Verification):\n")
	if passed {
		stages := strings.Join(e.Profile(e.Workspace).StageNames(), "`, `")
		sb.WriteString(fmt.Sprintf("- ✅ ตรวจสอบสำเร็จสมบูรณ์ (`%s`) ในเวลา %v\n", stages, duration.Round(time.Millisecond)))
		sb.WriteString(fmt.Sprintf("- 📊 สถิติการทำงาน: อ่านบริบท %d จุด, ผ่าตัดโค้ด %d จุด, สร้างไฟล์ใหม่ %d จุด\n", reads, patches, creates))
	} else {
		sb.WriteString(fmt.Sprintf("- ❌ การตรวจสอบไม่ผ่านหลังจากใช้เวลา %v\n", duration.Round(time.Millisecond)))
	}
	if r := plan.Ranking; r != nil {
		sb.WriteString("\n#### 🧭 Plan proposals:\n")
		for i, p := range r.Proposals {
			mark := ""
			if i == r.Chosen {
				mark = " (chosen)"
			}
			if p.Error != "" {
				sb.WriteString(fmt.Sprintf("- %s: failed — %s\n", p.Model, p.Error))
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s%s: score %d, %d steps — %s\n", p.Model, mark, p.Score, p.Steps, p.Objective))
		}
	}
	if len(e.ResearchSources) > 0 {
		sb.WriteString("\n#### 📚 Context gathered before planning:\n")
		for _, s := range e.ResearchSources {
			label := s.URL
			if s.Query != "" {
				label = "search: " + s.Query
			}
			if s.Error != "" {
				sb.WriteString(fmt.Sprintf("- %s — not used: %s\n", label, s.Error))
			} else {
				sb.WriteString(fmt.Sprintf("- %s (%d bytes)\n", label, s.Bytes))
			}
		}
	}
	return sb.String()
}

func (e *Executor) executeSubtask(ctx context.Context, base string, surg *surgeon.Surgeon, st architect.Subtask) (StepReport, string, error) {
	sr := StepReport{SubtaskID: st.ID, Action: st.Action, TargetFile: st.TargetFile}
	if st.Action == architect.ActionVerify {
		sr.Success = true
		sr.Message = "verification scheduled"
		return sr, "", nil
	}
	if err := surgeon.AllowedPath(st.TargetFile); err != nil {
		return sr, "", err
	}
	if e.Mode == core.ModeReview && st.Action != architect.ActionRead {
		return sr, "", errors.New("review mode forbids mutations")
	}
	switch st.Action {
	case architect.ActionRead:
		content, hash, err := surg.ReadWorkspaceFile(base, st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		e.rememberRead(st.TargetFile, content)
		sr.Success = true
		sr.Message = fmt.Sprintf("read %s (sha256 %s)", st.TargetFile, shortHash(hash))
		return sr, "", nil
	case architect.ActionCreate:
		dir, err := e.askDirective(ctx, fmt.Sprintf("Create file %s.\nInstruction: %s\nRespond ONLY with JSON: {\"file_content\":\"...\"}", st.TargetFile, st.Instruction))
		if err != nil {
			return sr, "", err
		}
		content := dir.FileContent
		if content == "" {
			content = dir.NewContent
		}
		if err = ctx.Err(); err != nil {
			return sr, "", err
		}
		before, err := surgeon.StateInRoot(base, st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		if before.Exists {
			return sr, "", errors.New("create target already exists")
		}
		if e.mutation != nil {
			if err = e.mutation(st.TargetFile, before, surgeon.FileState{Exists: true, Hash: surgeon.CalculateHash(content), Mode: 0644}); err != nil {
				return sr, "", err
			}
		}
		if err = surgeon.WriteInRoot(base, st.TargetFile, []byte(content), 0644, true); err != nil {
			return sr, "", err
		}
	case architect.ActionPatch:
		content, hash, err := surg.ReadWorkspaceFile(base, st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		e.rememberRead(st.TargetFile, content)
		prompt := fmt.Sprintf("Provide surgical patch for %s.\nInstruction: %s\nCurrent File Content:\n[source]\n%s\n[/source]\nRespond ONLY with JSON: {\"old_content\":\"...\",\"new_content\":\"...\"}", st.TargetFile, st.Instruction, content)
		if _, err = e.patchWithRetry(ctx, base, surg, st.TargetFile, hash, false, prompt); err != nil {
			return sr, "", err
		}
	case architect.ActionDelete:
		if err := ctx.Err(); err != nil {
			return sr, "", err
		}
		before, err := surgeon.StateInRoot(base, st.TargetFile)
		if err != nil {
			return sr, "", err
		}
		if e.mutation != nil {
			if err = e.mutation(st.TargetFile, before, surgeon.FileState{}); err != nil {
				return sr, "", err
			}
		}
		if err := surgeon.RemoveInRoot(base, st.TargetFile); err != nil {
			return sr, "", err
		}
	default:
		return sr, "", fmt.Errorf("unsupported action %s", st.Action)
	}
	sr.Success = true
	sr.Message = fmt.Sprintf("%s %s", st.Action, st.TargetFile)
	return sr, st.TargetFile, nil
}

func (e *Executor) repairWithReflex(ctx context.Context, base string, surg *surgeon.Surgeon, diagnostics string, files *[]string) error {
	target := ""
	if len(*files) > 0 {
		target = (*files)[len(*files)-1]
	}
	var content string
	if target != "" {
		content, _, _ = surg.ReadWorkspaceFile(base, target)
	}
	prompt := fmt.Sprintf("Fix verification failure with a minimal patch.\n%s\nCurrent file %s:\n%s\nRespond ONLY with JSON: {\"target_file\":\"...\",\"old_content\":\"...\",\"new_content\":\"...\"}", diagnostics, target, content)
	patched, err := e.patchWithRetry(ctx, base, surg, target, "", true, prompt)
	if err == nil {
		appendModifiedFile(files, patched)
	}
	return err
}

// patchMatchAttempts bounds how often one patch is requested when its old_content does not match
// the file exactly once.
const patchMatchAttempts = 3

// retryablePatch reports Surgeon errors that the patch model can fix by choosing old_content again.
func retryablePatch(err error) bool {
	return errors.Is(err, surgeon.ErrTargetNotFound) || errors.Is(err, surgeon.ErrMultipleMatches) || errors.Is(err, surgeon.ErrEmptyTarget)
}

// patchWithRetry asks for a patch directive and applies it. When old_content does not occur
// exactly once, the Surgeon's error and the file's current content go back to the patch model,
// up to patchMatchAttempts requests. A failed match never writes or records a mutation. With
// allowTarget the directive may name another target file (repairs); it returns the patched file.
// shownHash, when set, is the hash of the content the first prompt showed: the first patch is
// refused if the file changed since (later attempts show and pin the current content).
func (e *Executor) patchWithRetry(ctx context.Context, base string, surg *surgeon.Surgeon, target, shownHash string, allowTarget bool, prompt string) (string, error) {
	feedback := ""
	for attempt := 1; ; attempt++ {
		dir, err := e.askDirective(ctx, prompt+feedback)
		if err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		file := target
		if allowTarget && dir.TargetFile != "" {
			file = dir.TargetFile
		}
		if file == "" {
			return "", errors.New("repair target missing")
		}
		current, hash, err := surg.ReadWorkspaceFile(base, file)
		if err != nil {
			return "", err
		}
		if attempt == 1 && shownHash != "" && file == target {
			hash = shownHash
		}
		_, err = surg.PatchWorkspaceFile(base, file, surgeon.PatchOperation{BaseHash: hash, OldContent: dir.OldContent, NewContent: dir.NewContent}, func(before, after surgeon.FileState) error {
			if e.mutation != nil {
				return e.mutation(file, before, after)
			}
			return nil
		})
		if err == nil {
			return file, nil
		}
		if !retryablePatch(err) || attempt >= patchMatchAttempts {
			return "", err
		}
		e.emit(ProgressEvent{Stage: "patch_retry", Message: fmt.Sprintf("%s: %v; asking for the patch again (%d/%d)", file, err, attempt+1, patchMatchAttempts)})
		feedback = patchFeedback(err, file, dir.OldContent, current)
	}
}

// patchFeedback explains a failed match to the patch model and shows the file as it is now.
func patchFeedback(err error, file, oldContent, current string) string {
	reason := "old_content was empty; it must be the exact existing text to replace."
	switch {
	case errors.Is(err, surgeon.ErrTargetNotFound):
		reason = "old_content does not occur in the file. Copy it exactly from the current content below, including indentation, blank lines and line breaks."
	case errors.Is(err, surgeon.ErrMultipleMatches):
		reason = "old_content occurs more than once. Extend it with surrounding lines so it matches exactly one place."
	}
	return fmt.Sprintf("\n\nYOUR PREVIOUS PATCH FOR %s WAS REJECTED: %s\nRejected old_content:\n[old]\n%s\n[/old]\nCurrent content of %s:\n[source]\n%s\n[/source]\nRespond again with the same JSON format.", file, reason, headUTF8(oldContent, 4096), file, headUTF8(current, 48*1024))
}

func decodeDirective(raw []byte, dir *patchDirective) error {
	if len(raw) > 512*1024 {
		return errors.New("directive exceeds response limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dir); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing directive content")
	}
	return nil
}

func parsePatchDirective(raw string) (*patchDirective, error) {
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
	if err := decodeDirective([]byte(jsonStr), &dir); err != nil {
		// Robust fallback 1: sanitize invalid escape sequences (e.g. "\ ", "\.", raw newlines)
		sanitized := sanitizeJSONEscapes(jsonStr)
		if errSan := decodeDirective([]byte(sanitized), &dir); errSan == nil {
			return &dir, nil
		}

		// Robust fallback 2: try extracting outermost JSON object if model wrapped output in conversational text
		if start := strings.Index(raw, "{"); start != -1 {
			if end := strings.LastIndex(raw, "}"); end != -1 && end > start {
				extracted := strings.TrimSpace(raw[start : end+1])
				if err2 := decodeDirective([]byte(extracted), &dir); err2 == nil {
					return &dir, nil
				}
				if err3 := decodeDirective([]byte(sanitizeJSONEscapes(extracted)), &dir); err3 == nil {
					return &dir, nil
				}
			}
		}
		preview := raw
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, fmt.Errorf("parse patch directive JSON: %w (raw: %q)", err, preview)
	}
	return &dir, nil
}

func sanitizeJSONEscapes(input string) string {
	var sb strings.Builder
	sb.Grow(len(input) + 32)
	inString := false
	runes := []rune(input)
	n := len(runes)

	for i := 0; i < n; i++ {
		ch := runes[i]
		if ch == '"' {
			backslashes := 0
			for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				inString = !inString
			}
			sb.WriteRune(ch)
			continue
		}

		if inString {
			if ch == '\\' {
				if i+1 < n {
					next := runes[i+1]
					switch next {
					case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
						sb.WriteRune('\\')
						sb.WriteRune(next)
						i++
						continue
					case 'u':
						if i+5 < n {
							isHex := true
							for k := i + 2; k <= i+5; k++ {
								h := runes[k]
								if !((h >= '0' && h <= '9') || (h >= 'a' && h <= 'f') || (h >= 'A' && h <= 'F')) {
									isHex = false
									break
								}
							}
							if isHex {
								sb.WriteString("\\u")
								for k := i + 2; k <= i+5; k++ {
									sb.WriteRune(runes[k])
								}
								i += 5
								continue
							}
						}
						sb.WriteString("\\\\")
						continue
					default:
						// Invalid escape sequence in JSON string (e.g. \  or \( or \' or \s or \.)
						// Escape the backslash so it parses cleanly as literal backslash
						sb.WriteString("\\\\")
						sb.WriteRune(next)
						i++
						continue
					}
				} else {
					sb.WriteString("\\\\")
					continue
				}
			} else if ch == '\n' {
				sb.WriteString("\\n")
				continue
			} else if ch == '\r' {
				sb.WriteString("\\r")
				continue
			} else if ch == '\t' {
				sb.WriteString("\\t")
				continue
			}
		}

		sb.WriteRune(ch)
	}

	return sb.String()
}

func (e *Executor) safePath(baseDir, rel string) (string, error) {
	if err := surgeon.AllowedPath(rel); err != nil {
		return "", err
	}
	if baseDir == "" {
		baseDir = e.Workspace
	}
	return surgeon.ResolveInRoot(baseDir, rel)
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
