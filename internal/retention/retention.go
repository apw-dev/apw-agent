// Package retention removes old task state under an explicit, conservative policy.
// It never touches source workspace files: only task worktrees in the workspace's worktree directories
// (the external root or the legacy .worktrees), task plan/checkpoint files, rolled-back apply
// journals, task events/reports, delivered Telegram rows and the daemon log.
package retention

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// LegacyTaskID is the pre-manifest task whose work must be preserved until it is reviewed by hand.
const LegacyTaskID = "task-1792166521189239500"

const day = 24 * time.Hour

// Policy is a resolved RetentionConfig. A zero age or count disables that rule.
type Policy struct {
	SucceededAge  time.Duration
	FailedAge     time.Duration
	OutboxSentAge time.Duration
	InboxKeep     int
	LogMaxBytes   int64
	Protect       map[string]bool
}

// Resolve applies defaults: zero selects the default, a negative value disables the rule.
func Resolve(cfg config.RetentionConfig) Policy {
	pick := func(value, fallback int) int {
		switch {
		case value < 0:
			return 0
		case value == 0:
			return fallback
		}
		return value
	}
	p := Policy{
		SucceededAge:  time.Duration(pick(cfg.SucceededDays, 14)) * day,
		FailedAge:     time.Duration(pick(cfg.FailedDays, 30)) * day,
		OutboxSentAge: time.Duration(pick(cfg.OutboxSentDays, 7)) * day,
		InboxKeep:     pick(cfg.InboxKeep, 1000),
		LogMaxBytes:   int64(pick(cfg.LogMaxMB, 8)) << 20,
		Protect:       map[string]bool{LegacyTaskID: true},
	}
	for _, id := range cfg.ProtectTasks {
		p.Protect[id] = true
	}
	return p
}

type Options struct {
	DryRun bool
	// ExportUnapplied exports a worktree's unapplied changes to .apw/recovery before removing it.
	ExportUnapplied bool
	// Vacuum runs a full VACUUM after pruning.
	Vacuum bool
	// RotateLog rotates .apw/daemon.log; only safe while no daemon writes to it.
	RotateLog bool
	// WorktreeRoot is apw.json worktree_root (empty for the default external root).
	WorktreeRoot string
	Now          time.Time
}

type Item struct {
	TaskID   string   `json:"task_id,omitempty"`
	Status   string   `json:"status,omitempty"`
	Worktree string   `json:"worktree,omitempty"`
	Files    []string `json:"files,omitempty"`
	Reason   string   `json:"reason"`
}

type Report struct {
	DryRun                bool              `json:"dry_run"`
	Removed               []Item            `json:"removed"`
	Exported              []Item            `json:"exported,omitempty"`
	Protected             []Item            `json:"protected"`
	Failed                []Item            `json:"failed,omitempty"`
	Orphans               []Item            `json:"orphans,omitempty"`
	UnreadableCheckpoints []string          `json:"unreadable_checkpoints,omitempty"`
	Database              store.PruneResult `json:"database"`
	Compaction            string            `json:"compaction,omitempty"`
	LogRotated            bool              `json:"log_rotated"`
}

// Run applies the policy to the workspace at root. The caller must hold the workspace service lease
// unless opts.DryRun is set, so no task can be resumed, applied or written while state is removed.
func Run(ctx context.Context, root string, db *store.Store, policy Policy, opts Options) (*Report, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	tasks, err := db.TaskSummaries(ctx)
	if err != nil {
		return nil, err
	}
	checkpoints, unreadable, err := executor.TaskCheckpoints(root)
	if err != nil {
		return nil, err
	}
	r := &runner{root: root, policy: policy, opts: opts, now: now, checkpoints: checkpoints, claimed: map[string]bool{}}
	r.report.DryRun = opts.DryRun
	r.report.UnreadableCheckpoints = unreadable
	var prune []string
	for _, t := range tasks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if r.task(ctx, t) {
			prune = append(prune, t.ID)
		}
	}
	if err := r.orphans(); err != nil {
		return nil, err
	}
	req := store.PruneRequest{TaskIDs: prune, InboxKeep: policy.InboxKeep, DryRun: opts.DryRun}
	if policy.OutboxSentAge > 0 {
		req.SentBefore = now.Add(-policy.OutboxSentAge)
	}
	if r.report.Database, err = db.Prune(ctx, req); err != nil {
		return nil, fmt.Errorf("prune task store (files already removed are listed): %w", err)
	}
	if !opts.DryRun {
		if r.report.Compaction, err = db.Compact(ctx, opts.Vacuum); err != nil {
			return &r.report, fmt.Errorf("compact task store: %w", err)
		}
	}
	if opts.RotateLog && policy.LogMaxBytes > 0 {
		if r.report.LogRotated, err = RotateLog(root, policy.LogMaxBytes, opts.DryRun); err != nil {
			return &r.report, err
		}
	}
	return &r.report, nil
}

type runner struct {
	root        string
	policy      Policy
	opts        Options
	now         time.Time
	checkpoints map[string][]string
	claimed     map[string]bool
	report      Report
}

func (r *runner) protect(t store.TaskSummary, path, reason string) bool {
	r.report.Protected = append(r.report.Protected, Item{TaskID: t.ID, Status: string(t.Status), Worktree: path, Reason: reason})
	return false
}

func (r *runner) failed(t store.TaskSummary, path string, files []string, err error) bool {
	r.report.Failed = append(r.report.Failed, Item{TaskID: t.ID, Status: string(t.Status), Worktree: path, Files: files, Reason: err.Error() + "; task evidence kept"})
	return false
}

// task classifies one task and, unless this is a dry run, removes its files. It reports whether
// the task's database evidence may be pruned.
func (r *runner) task(ctx context.Context, t store.TaskSummary) bool {
	plans := r.checkpoints[t.ID]
	worktree := ""
	if len(plans) == 1 {
		if cp, err := executor.LoadCheckpoint(r.root, plans[0]); err == nil && cp.WorktreePath != "" {
			worktree = cp.WorktreePath
		}
	}
	if worktree == "" {
		worktree = r.findWorktree(t.ID)
	}
	if dir, name, err := surgeon.LocateWorktree(r.root, r.opts.WorktreeRoot, worktree); err == nil {
		r.claimed[pathKey(filepath.Join(dir, name))] = true
	}
	if r.policy.Protect[t.ID] {
		return r.protect(t, "", "protected by policy (legacy or retention.protect_tasks)")
	}
	var age time.Duration
	switch t.Status {
	case core.StatusSucceeded:
		age = r.policy.SucceededAge
	case core.StatusFailed, core.StatusCancelled, core.StatusProviderUnavailable:
		age = r.policy.FailedAge
	case core.StatusReady, core.StatusPreview, core.StatusInterrupted:
		return r.protect(t, "", "resumable or unapplied work")
	default:
		return r.protect(t, "", "task is not finished")
	}
	if age <= 0 {
		return r.protect(t, "", "retention disabled for this status")
	}
	if r.now.Sub(t.UpdatedAt) < age {
		return r.protect(t, "", fmt.Sprintf("newer than %d days", int(age/day)))
	}
	if len(plans) > 1 {
		return r.protect(t, "", "multiple checkpoints; refusing to guess")
	}
	state, err := surgeon.JournalState(r.root, t.ID)
	if err != nil {
		return r.protect(t, "", err.Error())
	}
	if state != "" && state != "committed" && state != "rolled_back" {
		return r.protect(t, "", "apply journal is not finalized ("+state+")")
	}
	hasWorktree := false
	if _, err := os.Lstat(worktree); err == nil {
		hasWorktree = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return r.protect(t, worktree, err.Error())
	}
	if !hasWorktree {
		worktree = ""
	}
	var wt *surgeon.Worktree
	if hasWorktree {
		if _, _, err := surgeon.LocateWorktree(r.root, r.opts.WorktreeRoot, worktree); err != nil {
			return r.protect(t, worktree, err.Error())
		}
		if wt, err = surgeon.LoadWorktree(worktree); err != nil {
			return r.protect(t, worktree, "no snapshot manifest (legacy work); export with apw recover and remove by hand after review")
		}
		if wt.TaskID != t.ID {
			return r.protect(t, worktree, "worktree manifest belongs to another task")
		}
		if !(t.Status == core.StatusSucceeded && surgeon.Applied(r.root, t.ID)) {
			if ok := r.unappliedSafe(ctx, t, wt, len(plans) == 1); !ok {
				return false
			}
		}
	}
	reason := "eligible: " + string(t.Status) + " older than " + fmt.Sprint(int(age/day)) + " days"
	if r.opts.DryRun {
		var files []string
		if len(plans) == 1 {
			files = []string{plans[0] + ".checkpoint.json", plans[0] + ".json"}
		}
		r.report.Removed = append(r.report.Removed, Item{TaskID: t.ID, Status: string(t.Status), Worktree: worktree, Files: files, Reason: "would remove; " + reason})
		return true
	}
	if wt != nil {
		if err := surgeon.RemoveRetainedWorktree(ctx, r.root, r.opts.WorktreeRoot, wt); err != nil {
			return r.failed(t, worktree, nil, err)
		}
	}
	var files []string
	if len(plans) == 1 {
		removed, err := executor.RemoveCheckpointFiles(r.root, plans[0])
		files = append(files, removed...)
		if err != nil {
			return r.failed(t, worktree, files, err)
		}
	}
	if removed, err := surgeon.RemoveRolledBackJournal(r.root, t.ID); err != nil {
		return r.failed(t, worktree, files, err)
	} else if removed {
		files = append(files, t.ID+".json (rolled-back journal)")
	}
	r.report.Removed = append(r.report.Removed, Item{TaskID: t.ID, Status: string(t.Status), Worktree: worktree, Files: files, Reason: reason})
	return true
}

// unappliedSafe reports whether removing a worktree that was not applied loses no work: it has no
// changes relative to its snapshot, or a completed recovery export holds them.
func (r *runner) unappliedSafe(ctx context.Context, t store.TaskSummary, wt *surgeon.Worktree, hasCheckpoint bool) bool {
	changed, err := wt.ChangedFiles(ctx)
	if err != nil {
		r.protect(t, wt.Path, "cannot inspect worktree changes: "+err.Error())
		return false
	}
	if len(changed) == 0 {
		return true
	}
	export, err := executor.CompletedRecoveryExport(r.root, t.ID)
	if err != nil {
		r.protect(t, wt.Path, err.Error())
		return false
	}
	if export != "" {
		return true
	}
	summary := fmt.Sprintf("%d unapplied changed file(s)", len(changed))
	if !r.opts.ExportUnapplied {
		r.protect(t, wt.Path, summary+" without an export; run apw recover "+t.ID+" or cleanup --export-unapplied")
		return false
	}
	if !hasCheckpoint {
		r.protect(t, wt.Path, summary+" and no checkpoint to export from")
		return false
	}
	if r.opts.DryRun {
		r.report.Exported = append(r.report.Exported, Item{TaskID: t.ID, Status: string(t.Status), Worktree: wt.Path, Reason: "would export " + summary})
		return true
	}
	result, err := executor.RecoverLegacyTask(ctx, r.root, r.opts.WorktreeRoot, t.ID)
	if err != nil {
		r.failed(t, wt.Path, nil, fmt.Errorf("export unapplied changes: %w", err))
		return false
	}
	r.report.Exported = append(r.report.Exported, Item{TaskID: t.ID, Status: string(t.Status), Worktree: wt.Path, Files: []string{result.Directory}, Reason: "exported " + summary})
	return true
}

// findWorktree locates a task's worktree when no checkpoint records it: <root>/<short-id> in an
// external root or <workspace>/.worktrees/<task-id> (legacy). It returns "" when none exists.
func (r *runner) findWorktree(id string) string {
	if id == "" || !filepath.IsLocal(id) || filepath.Base(id) != id {
		return ""
	}
	for _, dir := range surgeon.WorktreeDirs(r.root, r.opts.WorktreeRoot) {
		for _, name := range []string{surgeon.TaskWorktreeName(id), id} {
			path := filepath.Join(dir, name)
			if _, err := os.Lstat(path); err == nil {
				return path
			}
		}
	}
	return ""
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

// orphans lists entries of the worktree directories (external root and legacy .worktrees) that no
// task record accounts for. They are never removed.
func (r *runner) orphans() error {
	for _, dir := range surgeon.WorktreeDirs(r.root, r.opts.WorktreeRoot) {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var names []string
		for _, entry := range entries {
			if !r.claimed[pathKey(filepath.Join(dir, entry.Name()))] {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			r.report.Orphans = append(r.report.Orphans, Item{Worktree: filepath.Join(dir, name), Reason: "no task record; kept"})
		}
	}
	return nil
}

// RotateLog renames .apw/daemon.log to daemon.log.1 (replacing the previous rotation) once it exceeds
// maxBytes. Run it only while no daemon writes to the log. A dry run reports whether it would rotate.
func RotateLog(workspace string, maxBytes int64, dryRun bool) (bool, error) {
	if maxBytes <= 0 {
		return false, nil
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return false, err
	}
	defer root.Close()
	dir, err := root.Lstat(".apw")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !dir.IsDir() || dir.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return false, errors.New("workspace state directory must not be a link")
	}
	current := filepath.Join(".apw", "daemon.log")
	info, err := root.Lstat(current)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("daemon log must be a regular file")
	}
	if info.Size() <= maxBytes {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	previous := filepath.Join(".apw", "daemon.log.1")
	if old, err := root.Lstat(previous); err == nil {
		if !old.Mode().IsRegular() {
			return false, errors.New("rotated daemon log must be a regular file")
		}
		if err = root.Remove(previous); err != nil {
			return false, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, root.Rename(current, previous)
}
