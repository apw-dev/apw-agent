package retention

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

const mainSource = "package main\n\nfunc main() {}\n"

type fixture struct {
	t    *testing.T
	root string
	db   *store.Store
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return string(out)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainSource), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"},
		{"add", "."}, {"commit", "-m", "fixture"},
	} {
		git(t, root, args...)
	}
	if err := os.MkdirAll(filepath.Join(root, ".apw"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, ".apw", "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &fixture{t: t, root: root, db: db}
}

// task records a job in status with a report and one progress event.
func (f *fixture) task(id string, status core.TaskStatus) {
	f.t.Helper()
	ctx := context.Background()
	if _, _, err := f.db.CreateJob(ctx, store.Job{Task: core.Task{ID: id, Workspace: f.root, Instruction: "change", Mode: "execute"}, Owner: "local", RequestID: id}); err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.AddEvent(ctx, id, "step_done", "did work", map[string]string{"k": "v"}); err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.SetJob(ctx, id, status, json.RawMessage(`{"diff":"evidence"}`), ""); err != nil {
		f.t.Fatal(err)
	}
}

// worktree creates a real task worktree with a checkpoint; changed edits main.go inside it.
func (f *fixture) worktree(id string, changed bool) *surgeon.Worktree {
	f.t.Helper()
	wt, err := surgeon.CreateWorktree(context.Background(), f.root, surgeon.WorktreeOptions{TaskID: id, SyncDirtyFiles: true})
	if err != nil {
		f.t.Fatal(err)
	}
	if changed {
		if err := os.WriteFile(filepath.Join(wt.Path, "main.go"), []byte(mainSource+"\nfunc Added() {}\n"), 0644); err != nil {
			f.t.Fatal(err)
		}
	}
	plan := &architect.ExecutionPlan{PlanID: "plan-" + id, TaskID: id, Objective: "change", Subtasks: []architect.Subtask{{ID: "one", Action: architect.ActionPatch, TargetFile: "main.go", Instruction: "add"}}}
	cp := executor.PlanCheckpoint{PlanID: plan.PlanID, TaskID: id, Timestamp: time.Now(), LastSuccessStep: 1, TotalSteps: 1, WorktreeBranch: wt.Branch, WorktreePath: wt.Path, ModifiedFiles: []string{"main.go"}, Plan: plan}
	if err := executor.SaveCheckpoint(f.root, cp); err != nil {
		f.t.Fatal(err)
	}
	return wt
}

func (f *fixture) events(id string) []store.Event {
	f.t.Helper()
	events, err := f.db.Events(context.Background(), id, "local", 0)
	if err != nil {
		f.t.Fatal(err)
	}
	return events
}

func (f *fixture) report(id string) json.RawMessage {
	f.t.Helper()
	job, err := f.db.Job(context.Background(), id, "local")
	if err != nil {
		f.t.Fatal(err)
	}
	return job.Report
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func branchExists(t *testing.T, root, branch string) bool {
	t.Helper()
	return strings.TrimSpace(git(t, root, "branch", "--list", branch)) != ""
}

func ids(items []Item) map[string]string {
	result := map[string]string{}
	for _, item := range items {
		result[item.TaskID] = item.Reason
	}
	return result
}

var later = time.Now().Add(90 * day)

func TestDryRunListsWithoutDeleting(t *testing.T) {
	f := newFixture(t)
	f.task("task-done", core.StatusSucceeded)
	wt := f.worktree("task-done", false)
	before := len(f.events("task-done"))

	report, err := Run(context.Background(), f.root, f.db, Resolve(config.RetentionConfig{}), Options{DryRun: true, Now: later})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ids(report.Removed)["task-done"]; !ok || !report.DryRun {
		t.Fatalf("dry run did not list eligible task: %+v", report)
	}
	if report.Database.Events != int64(before) || report.Database.Reports != 1 {
		t.Fatalf("dry run counts = %+v, want %d events and 1 report", report.Database, before)
	}
	if !exists(t, wt.Path) || !branchExists(t, f.root, wt.Branch) || !exists(t, filepath.Join(f.root, ".apw", "plans", "plan-task-done.checkpoint.json")) {
		t.Fatal("dry run removed files")
	}
	if len(f.events("task-done")) != before || len(f.report("task-done")) == 0 {
		t.Fatal("dry run pruned the database")
	}

	// The same task is protected while it is younger than the window.
	report, err = Run(context.Background(), f.root, f.db, Resolve(config.RetentionConfig{}), Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if reason := ids(report.Protected)["task-done"]; !strings.Contains(reason, "newer than 14 days") {
		t.Fatalf("young task reason = %q", reason)
	}
}

func TestCleanupRemovesEligibleAndKeepsProtected(t *testing.T) {
	f := newFixture(t)
	f.task("task-done", core.StatusSucceeded)
	done := f.worktree("task-done", false)
	f.task("task-failed", core.StatusFailed)
	failed := f.worktree("task-failed", true)
	f.task("task-ready", core.StatusReady)
	ready := f.worktree("task-ready", true)
	f.task("task-paused", core.StatusInterrupted)
	f.task(LegacyTaskID, core.StatusCancelled)
	f.task("task-pinned", core.StatusSucceeded)
	f.task("task-journal", core.StatusFailed)
	f.task("task-rolled", core.StatusCancelled)
	transactions := filepath.Join(f.root, ".apw", "transactions")
	if err := os.MkdirAll(transactions, 0700); err != nil {
		t.Fatal(err)
	}
	for id, state := range map[string]string{"task-journal": "prepared", "task-rolled": "rolled_back"} {
		data, _ := json.Marshal(map[string]string{"task_id": id, "state": state})
		if err := os.WriteFile(filepath.Join(transactions, id+".json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	stray := filepath.Join(f.root, ".worktrees", "stray")
	if err := os.MkdirAll(stray, 0700); err != nil {
		t.Fatal(err)
	}
	source, _ := os.ReadFile(filepath.Join(f.root, "main.go"))

	policy := Resolve(config.RetentionConfig{ProtectTasks: []string{"task-pinned"}})
	report, err := Run(context.Background(), f.root, f.db, policy, Options{Now: later})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) > 0 {
		t.Fatalf("unexpected failures: %+v", report.Failed)
	}
	removed, protected := ids(report.Removed), ids(report.Protected)
	for _, id := range []string{"task-done", "task-rolled"} {
		if _, ok := removed[id]; !ok {
			t.Fatalf("%s not removed: %+v", id, report)
		}
	}
	for id, want := range map[string]string{
		"task-failed":  "unapplied changed file",
		"task-ready":   "resumable",
		"task-paused":  "resumable",
		LegacyTaskID:   "protected by policy",
		"task-pinned":  "protected by policy",
		"task-journal": "not finalized",
	} {
		if !strings.Contains(protected[id], want) {
			t.Fatalf("%s protection = %q, want %q", id, protected[id], want)
		}
	}
	if len(report.Orphans) != 1 || report.Orphans[0].Worktree != stray || !exists(t, stray) {
		t.Fatalf("orphan handling = %+v", report.Orphans)
	}

	if exists(t, done.Path) || branchExists(t, f.root, done.Branch) {
		t.Fatal("eligible worktree or branch kept")
	}
	if exists(t, filepath.Join(f.root, ".apw", "plans", "plan-task-done.checkpoint.json")) || exists(t, filepath.Join(f.root, ".apw", "plans", "plan-task-done.json")) {
		t.Fatal("eligible checkpoint kept")
	}
	if exists(t, filepath.Join(transactions, "task-rolled.json")) || !exists(t, filepath.Join(transactions, "task-journal.json")) {
		t.Fatal("journal retention wrong")
	}
	for _, wt := range []*surgeon.Worktree{failed, ready} {
		if !exists(t, wt.Path) || !branchExists(t, f.root, wt.Branch) {
			t.Fatalf("protected worktree removed: %s", wt.Path)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(f.root, "main.go")); !bytes.Equal(after, source) {
		t.Fatal("source workspace changed")
	}

	// Pruning drops the eligible task's events and report but keeps protected tasks' evidence.
	events := f.events("task-done")
	if len(events) != 1 || events[0].Stage != "retention" || len(f.report("task-done")) != 0 {
		t.Fatalf("eligible task evidence not pruned: %+v", events)
	}
	for _, id := range []string{"task-failed", "task-ready", "task-paused", LegacyTaskID, "task-pinned", "task-journal"} {
		if len(f.events(id)) < 2 || len(f.report(id)) == 0 {
			t.Fatalf("protected task %s lost evidence", id)
		}
	}
	if job, _ := f.db.Job(context.Background(), "task-done", "local"); job.Status != core.StatusSucceeded {
		t.Fatal("task row status changed")
	}

	// A repeated run is stable and does not add another marker.
	again, err := Run(context.Background(), f.root, f.db, policy, Options{Now: later})
	if err != nil {
		t.Fatal(err)
	}
	if again.Database.Events != 0 || again.Database.Reports != 0 || len(f.events("task-done")) != 1 {
		t.Fatalf("second run pruned again: %+v", again.Database)
	}
}

func TestExportUnappliedBeforeRemoval(t *testing.T) {
	f := newFixture(t)
	f.task("task-failed", core.StatusFailed)
	wt := f.worktree("task-failed", true)
	work, _ := os.ReadFile(filepath.Join(wt.Path, "main.go"))

	preview, err := Run(context.Background(), f.root, f.db, Resolve(config.RetentionConfig{}), Options{DryRun: true, ExportUnapplied: true, Now: later})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Exported) != 1 || len(preview.Removed) != 1 || exists(t, filepath.Join(f.root, ".apw", "recovery")) {
		t.Fatalf("dry run export = %+v", preview)
	}

	report, err := Run(context.Background(), f.root, f.db, Resolve(config.RetentionConfig{}), Options{ExportUnapplied: true, Now: later})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Exported) != 1 || len(report.Removed) != 1 || len(report.Failed) > 0 {
		t.Fatalf("export report = %+v", report)
	}
	dir := report.Exported[0].Files[0]
	exported, err := os.ReadFile(filepath.Join(dir, "files", "main.go"))
	if err != nil || !bytes.Equal(exported, work) {
		t.Fatalf("exported work differs: %v", err)
	}
	if exists(t, wt.Path) {
		t.Fatal("worktree kept after export")
	}
	if found, _ := executor.CompletedRecoveryExport(f.root, "task-failed"); found != dir {
		t.Fatalf("export evidence = %q, want %q", found, dir)
	}
}

func TestWorktreeWithCommitsIsKept(t *testing.T) {
	f := newFixture(t)
	f.task("task-done", core.StatusSucceeded)
	wt := f.worktree("task-done", false)
	if err := os.WriteFile(filepath.Join(wt.Path, "extra.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, wt.Path, "add", "extra.txt")
	git(t, wt.Path, "commit", "-m", "work inside worktree")

	report, err := Run(context.Background(), f.root, f.db, Resolve(config.RetentionConfig{}), Options{Now: later})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Removed) != 0 || !exists(t, wt.Path) || !branchExists(t, f.root, wt.Branch) {
		t.Fatalf("worktree with commits removed: %+v", report)
	}
	if len(f.report("task-done")) == 0 {
		t.Fatal("evidence pruned although worktree removal was refused")
	}
}

func TestResolveDefaultsAndDisable(t *testing.T) {
	p := Resolve(config.RetentionConfig{})
	if p.SucceededAge != 14*day || p.FailedAge != 30*day || p.OutboxSentAge != 7*day || p.InboxKeep != 1000 || p.LogMaxBytes != 8<<20 || !p.Protect[LegacyTaskID] {
		t.Fatalf("defaults = %+v", p)
	}
	p = Resolve(config.RetentionConfig{SucceededDays: -1, FailedDays: 3, InboxKeep: -1, LogMaxMB: -1})
	if p.SucceededAge != 0 || p.FailedAge != 3*day || p.InboxKeep != 0 || p.LogMaxBytes != 0 {
		t.Fatalf("overrides = %+v", p)
	}
}

func TestRotateLog(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".apw")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath, rotated := filepath.Join(dir, "daemon.log"), filepath.Join(dir, "daemon.log.1")
	if ok, err := RotateLog(root, 1024, false); ok || err != nil {
		t.Fatalf("missing log: %v %v", ok, err)
	}
	if err := os.WriteFile(rotated, []byte("older"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("small"), 0600); err != nil {
		t.Fatal(err)
	}
	if ok, err := RotateLog(root, 1024, false); ok || err != nil || !exists(t, logPath) {
		t.Fatalf("small log rotated: %v %v", ok, err)
	}
	big := bytes.Repeat([]byte("x"), 2048)
	if err := os.WriteFile(logPath, big, 0600); err != nil {
		t.Fatal(err)
	}
	if ok, err := RotateLog(root, 1024, true); !ok || err != nil || !exists(t, logPath) {
		t.Fatalf("dry run: %v %v", ok, err)
	}
	if ok, err := RotateLog(root, 1024, false); !ok || err != nil {
		t.Fatalf("rotate: %v %v", ok, err)
	}
	if exists(t, logPath) {
		t.Fatal("current log still present")
	}
	if data, _ := os.ReadFile(rotated); !bytes.Equal(data, big) {
		t.Fatal("rotation did not replace the previous file")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected exactly one rotated file, got %d", len(entries))
	}
}
