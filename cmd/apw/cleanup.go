package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/retention"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/workspace"
)

func runCleanup(args []string, stdout, stderr io.Writer) int {
	options, code := parseOptions("cleanup", args, stderr)
	if code != 0 {
		return code
	}
	if len(options.Args) > 0 {
		fmt.Fprintln(stderr, "Usage: apw cleanup [--config path] [--dry-run] [--export-unapplied] [--vacuum] [--json]")
		return 2
	}
	cfg, _, err := daemon.LoadConfig(options.Config)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	dbPath := filepath.Join(cfg.Workspace, ".apw", "tasks.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(stdout, "No task store in this workspace; nothing to clean up.")
		return 0
	}
	if !options.DryRun {
		// The service lease excludes a running daemon, so no task can be resumed or applied meanwhile.
		lease, err := workspace.Acquire(cfg.Workspace, "service")
		if err != nil {
			fmt.Fprintf(stderr, "%v\nStop the daemon first ('apw daemon stop'), or use --dry-run.\n", err)
			return 1
		}
		defer lease.Close()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	report, err := retention.Run(ctx, cfg.Workspace, db, retention.Resolve(cfg.Retention), retention.Options{
		DryRun: options.DryRun, ExportUnapplied: options.ExportUnapplied, Vacuum: options.Vacuum, RotateLog: true,
	})
	if report != nil {
		if options.JSONMode {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(report)
		} else {
			printCleanupReport(stdout, report)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(report.Failed) > 0 {
		return 1
	}
	return 0
}

func printCleanupReport(w io.Writer, r *retention.Report) {
	verb := "Removed"
	if r.DryRun {
		verb = "Would remove"
		fmt.Fprintln(w, "Dry run: nothing was changed.")
	}
	section := func(title string, items []retention.Item) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(w, "%s (%d):\n", title, len(items))
		for _, item := range items {
			label := item.TaskID
			if label == "" {
				label = item.Worktree
			}
			if item.Status != "" {
				label += " [" + item.Status + "]"
			}
			fmt.Fprintf(w, "  %s: %s\n", label, item.Reason)
			if item.TaskID != "" && item.Worktree != "" {
				fmt.Fprintf(w, "    worktree %s\n", item.Worktree)
			}
			if len(item.Files) > 0 {
				fmt.Fprintf(w, "    files %s\n", strings.Join(item.Files, ", "))
			}
		}
	}
	section(verb, r.Removed)
	section("Exported before removal", r.Exported)
	section("Kept", r.Protected)
	section("Failed", r.Failed)
	section("Orphan worktree directories (never removed)", r.Orphans)
	if len(r.UnreadableCheckpoints) > 0 {
		fmt.Fprintf(w, "Unreadable checkpoints kept: %s\n", strings.Join(r.UnreadableCheckpoints, ", "))
	}
	d := r.Database
	fmt.Fprintf(w, "Database: %s %d event(s), %d report(s), %d delivered outbox row(s), %d old inbox row(s).\n", strings.ToLower(verb), d.Events, d.Reports, d.Outbox, d.Inbox)
	if r.Compaction != "" {
		fmt.Fprintf(w, "Compaction: %s\n", r.Compaction)
	}
	if r.LogRotated {
		fmt.Fprintf(w, "daemon.log: %s to daemon.log.1\n", map[bool]string{true: "would rotate", false: "rotated"}[r.DryRun])
	}
}
