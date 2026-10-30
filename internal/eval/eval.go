// Package eval measures APW on a suite of small, real coding tasks. Each task is a fixture
// repository, an instruction and hidden tests. A run copies the fixture into a fresh Git
// workspace, drives the normal in-process service (plan, patch, verify, apply) and then adds the
// hidden tests and runs the task's check. The decisive number is the false-positive rate: tasks
// APW verified and applied that the hidden tests still reject.
package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/process"
	"github.com/apiwat/apw-agent/internal/store"
)

// Outcome classifies one task run.
const (
	Solved        = "solved"         // applied and the hidden check passed
	FalsePositive = "false_positive" // applied (APW verified it) but the hidden check failed
	Unsolved      = "unsolved"       // APW produced no applicable change (a safe failure)
	Errored       = "error"          // the harness or the environment failed
)

// Task is one suite entry, read from <suite>/<name>/task.json with repo/ and hidden/ beside it.
type Task struct {
	Name        string   `json:"-"`
	Dir         string   `json:"-"`
	Instruction string   `json:"instruction"`
	Check       []string `json:"check"`
	TimeoutSec  int      `json:"timeout_sec,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// LoadSuite reads every task directory under dir, sorted by name.
func LoadSuite(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		taskDir := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(filepath.Join(taskDir, "task.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("%s/task.json: %w", e.Name(), err)
		}
		if strings.TrimSpace(t.Instruction) == "" || len(t.Check) == 0 {
			return nil, fmt.Errorf("%s/task.json needs instruction and check", e.Name())
		}
		if _, err := os.Stat(filepath.Join(taskDir, "repo")); err != nil {
			return nil, fmt.Errorf("%s: missing repo/: %w", e.Name(), err)
		}
		t.Name, t.Dir = e.Name(), taskDir
		tasks = append(tasks, t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Name < tasks[j].Name })
	return tasks, nil
}

// Options controls a run.
type Options struct {
	// Base supplies providers, roles, failover, sandbox, research, planning and browser settings.
	// Workspace, trust, worktree root and verify stages are set per task.
	Base *config.Config
	// CredentialRoot is the workspace whose .apw/auth.json holds saved API keys (the user's
	// configured workspace); keys found there are exported to this process's environment.
	CredentialRoot string
	Only           []string
	Keep           bool
	// Factory builds executors (tests); nil uses the configured providers.
	Factory app.Factory
	// Progress, when set, receives one line per notable step.
	Progress func(task, message string)
}

// Result is one task's outcome.
type Result struct {
	Task         string        `json:"task"`
	Outcome      string        `json:"outcome"`
	JobStatus    string        `json:"job_status,omitempty"`
	Applied      bool          `json:"applied"`
	HiddenPassed bool          `json:"hidden_passed"`
	Unverified   bool          `json:"unverified,omitempty"`
	Warnings     []string      `json:"warnings,omitempty"`
	Steps        int           `json:"steps,omitempty"`
	Repairs      int           `json:"repairs"`
	Files        []string      `json:"files,omitempty"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
	CheckOutput  string        `json:"check_output,omitempty"`
	Workspace    string        `json:"workspace,omitempty"`
}

// Report summarizes a run.
type Report struct {
	StartedAt     time.Time     `json:"started_at"`
	Duration      time.Duration `json:"duration"`
	Architect     string        `json:"architect"`
	Reflex        string        `json:"reflex"`
	Results       []Result      `json:"results"`
	Solved        int           `json:"solved"`
	FalsePositive int           `json:"false_positive"`
	Unsolved      int           `json:"unsolved"`
	Errors        int           `json:"errors"`
}

// Run executes the selected tasks one after another.
func Run(ctx context.Context, tasks []Task, opts Options) (*Report, error) {
	if opts.Base == nil {
		return nil, errors.New("eval: base configuration required")
	}
	exportSavedKeys(opts.Base, opts.CredentialRoot)
	report := &Report{StartedAt: time.Now(), Architect: modelOf(opts.Base, opts.Base.Roles.Architect), Reflex: modelOf(opts.Base, opts.Base.Roles.Reflex)}
	selected := map[string]bool{}
	for _, name := range opts.Only {
		selected[strings.TrimSpace(name)] = true
	}
	for _, t := range tasks {
		if len(selected) > 0 && !selected[t.Name] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		r := runTask(ctx, t, opts)
		report.Results = append(report.Results, r)
		switch r.Outcome {
		case Solved:
			report.Solved++
		case FalsePositive:
			report.FalsePositive++
		case Unsolved:
			report.Unsolved++
		default:
			report.Errors++
		}
		if opts.Progress != nil {
			opts.Progress(t.Name, fmt.Sprintf("%s in %v", r.Outcome, r.Duration.Round(time.Second)))
		}
	}
	report.Duration = time.Since(report.StartedAt)
	return report, nil
}

func modelOf(cfg *config.Config, role string) string {
	if p, ok := cfg.Providers[role]; ok && p.Model != "" {
		return role + " (" + p.Model + ")"
	}
	return role
}

// exportSavedKeys makes keys saved with `apw auth` in the user's workspace visible to the
// providers of the per-task temporary workspaces, through this process's environment only.
func exportSavedKeys(cfg *config.Config, root string) {
	if root == "" {
		return
	}
	for _, p := range cfg.Providers {
		name := strings.TrimSpace(p.APIKeyEnv)
		if name == "" || os.Getenv(name) != "" {
			continue
		}
		if key := config.LoadSavedKey(root, name); key != "" {
			_ = os.Setenv(name, key)
		}
	}
}

func runTask(ctx context.Context, t Task, opts Options) Result {
	start := time.Now()
	r := Result{Task: t.Name}
	fail := func(err error) Result {
		r.Outcome, r.Error, r.Duration = Errored, err.Error(), time.Since(start)
		return r
	}
	progress := func(msg string) {
		if opts.Progress != nil {
			opts.Progress(t.Name, msg)
		}
	}
	tmp, err := os.MkdirTemp("", "apw-eval-"+t.Name+"-")
	if err != nil {
		return fail(err)
	}
	if opts.Keep {
		r.Workspace = tmp
	} else {
		defer os.RemoveAll(tmp)
	}
	ws := filepath.Join(tmp, "ws")
	if err := copyTree(filepath.Join(t.Dir, "repo"), ws); err != nil {
		return fail(err)
	}
	if err := initRepo(ctx, ws); err != nil {
		return fail(err)
	}
	cfg := *opts.Base
	cfg.Workspace = ws
	cfg.Policy.Trusted = true
	cfg.WorktreeRoot = filepath.Join(tmp, "wt")
	cfg.Verify.Stages = nil
	cfg.Retention = config.RetentionConfig{}
	cfg.Providers = map[string]config.ProviderConfig{}
	for k, v := range opts.Base.Providers {
		cfg.Providers[k] = v
	}
	timeout := time.Duration(t.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	svc, err := app.Open(taskCtx, &cfg, opts.Factory)
	if err != nil {
		return fail(err)
	}
	defer svc.Close()

	progress("running: " + firstLine(t.Instruction))
	job, err := svc.Submit(taskCtx, "local", app.Request{RequestID: app.NewID(), Instruction: t.Instruction, Mode: core.ModeExecute})
	if err != nil {
		return fail(err)
	}
	job, err = wait(taskCtx, svc, job.ID)
	if err != nil {
		_ = svc.Cancel(context.Background(), job.ID, "local")
		return fail(err)
	}
	r.JobStatus = string(job.Status)
	readReport(job, &r)
	if job.Status == core.StatusReady && len(r.Files) == 0 {
		// Verifying an unchanged tree proves nothing about the task: count it as unsolved.
		r.Outcome, r.Error, r.Duration = Unsolved, "verified without changing any file", time.Since(start)
		return r
	}
	if job.Status == core.StatusReady {
		progress("verified; applying")
		if _, err := svc.Apply(taskCtx, job.ID, "local"); err != nil {
			return fail(fmt.Errorf("apply: %w", err))
		}
		if job, err = wait(taskCtx, svc, job.ID); err != nil {
			return fail(err)
		}
		r.JobStatus = string(job.Status)
		r.Applied = job.Status == core.StatusSucceeded
	}
	if !r.Applied {
		r.Outcome, r.Duration = Unsolved, time.Since(start)
		if job.Error != "" {
			r.Error = job.Error
		}
		return r
	}
	if err := copyTree(filepath.Join(t.Dir, "hidden"), ws); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	progress("running hidden check")
	out, err := runCheck(taskCtx, ws, t.Check)
	r.CheckOutput = tail(out, 4000)
	r.HiddenPassed = err == nil
	r.Outcome = FalsePositive
	if r.HiddenPassed {
		r.Outcome = Solved
	}
	r.Duration = time.Since(start)
	return r
}

func wait(ctx context.Context, svc *app.Service, id string) (*store.Job, error) {
	for {
		job, err := svc.Get(ctx, id, "local")
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case core.StatusQueued, core.StatusAnalyzing, core.StatusPlanning, core.StatusPatching, core.StatusVerifying, core.StatusApplying:
		default:
			return job, nil
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func readReport(job *store.Job, r *Result) {
	if len(job.Report) == 0 {
		return
	}
	var rep executor.ExecutionReport
	if json.Unmarshal(job.Report, &rep) != nil {
		return
	}
	if rep.Plan != nil {
		r.Steps = len(rep.Plan.Subtasks)
	}
	if n := len(rep.ReflexAttempts); n > 1 {
		r.Repairs = n - 1
	}
	r.Files = rep.ModifiedFiles
	r.Unverified = rep.Unverified
	r.Warnings = rep.Warnings
}

func runCheck(ctx context.Context, dir string, command []string) (string, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = dir
	var out process.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := process.Run(ctx, cmd)
	return out.String(), err
}

func initRepo(ctx context.Context, dir string) error {
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.name", "APW Eval"}, {"config", "user.email", "eval@apw.local"},
		{"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"},
		{"add", "-A"}, {"commit", "-q", "-m", "fixture"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		process.HideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	return os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte(".apw/\n"), 0644)
}

// copyTree copies regular files from src into dst, creating directories; links are skipped.
func copyTree(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if len(line) > 100 {
		line = line[:100] + "..."
	}
	return line
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// Markdown renders the report as a table, with changes against a previous report when given.
func (r *Report) Markdown(baseline *Report) string {
	var b strings.Builder
	total := len(r.Results)
	fmt.Fprintf(&b, "APW eval — %s · architect %s · reflex %s · %d tasks in %v\n\n", r.StartedAt.Format("2006-01-02 15:04"), r.Architect, r.Reflex, total, r.Duration.Round(time.Second))
	prev := map[string]string{}
	if baseline != nil {
		for _, res := range baseline.Results {
			prev[res.Task] = res.Outcome
		}
	}
	b.WriteString("| Task | Outcome | Steps | Repairs | Warnings | Time | Change |\n|---|---|---|---|---|---|---|\n")
	for _, res := range r.Results {
		change := ""
		if before, ok := prev[res.Task]; ok && before != res.Outcome {
			change = before + " → " + res.Outcome
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %v | %s |\n", res.Task, res.Outcome, res.Steps, res.Repairs, len(res.Warnings), res.Duration.Round(time.Second), change)
	}
	if total > 0 {
		fmt.Fprintf(&b, "\nSolved %d/%d (%.0f%%) · false positives %d · unsolved %d · errors %d\n",
			r.Solved, total, 100*float64(r.Solved)/float64(total), r.FalsePositive, r.Unsolved, r.Errors)
	}
	if baseline != nil && len(baseline.Results) > 0 {
		fmt.Fprintf(&b, "Baseline: solved %d/%d · false positives %d\n", baseline.Solved, len(baseline.Results), baseline.FalsePositive)
	}
	return b.String()
}
