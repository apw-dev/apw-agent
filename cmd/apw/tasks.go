package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/store"
)

func runSubmit(mode core.TaskMode, args []string, stdout, stderr io.Writer) int {
	options, code := parseOptions(string(mode), args, stderr)
	if code != 0 {
		return code
	}
	return runSubmitOptions(mode, options, stdout, stderr)
}
func runSubmitOptions(mode core.TaskMode, options cliOptions, stdout, stderr io.Writer) int {
	path, instruction := options.Config, strings.TrimSpace(strings.Join(options.Args, " "))
	if instruction == "" {
		fmt.Fprintf(stderr, "Usage: apw %s [--config path] <instruction>\n", mode)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client, err := daemon.Connect(ctx, path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	requestID := options.RequestID
	if requestID == "" {
		requestID = app.NewID()
	}
	if options.NoVerify && mode == core.ModeExecute {
		mode = core.ModeUnverified
	}
	job, err := client.Submit(ctx, "local", app.Request{RequestID: requestID, SessionID: options.SessionID, Mode: mode, Instruction: instruction})
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintf(stderr, "Submission ID: %s. Check 'apw tasks' or retry the same instruction with --request-id %s.\n", requestID, requestID)
		return 1
	}
	code, final := followTaskResult(ctx, client, job, stdout, stderr, true, options.Auto || options.Approve != nil)
	if final == nil || final.Status != core.StatusReady {
		return code
	}
	switch {
	case options.Auto:
		fmt.Fprintln(stdout, "Applying automatically (approval mode: auto).")
	case options.Approve != nil && options.Approve(final):
	case options.Approve != nil:
		fmt.Fprintf(stdout, "Not applied. Apply later with 'apw apply %s'.\n", final.ID)
		return code
	default:
		return code
	}
	applyJob, err := client.Apply(ctx, final.ID, "local")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return followTask(ctx, client, applyJob, stdout, stderr)
}
func terminal(status core.TaskStatus) bool {
	switch status {
	case core.StatusQueued, core.StatusPlanning, core.StatusAnalyzing, core.StatusPatching, core.StatusVerifying, core.StatusApplying:
		return false
	}
	return true
}

// followFallbackTicks is how many poll ticks (250ms each, ~5s) may pass without
// a task_status event before the job is re-fetched anyway.
const followFallbackTicks = 20

func followTask(ctx context.Context, backend app.API, job *store.Job, stdout, stderr io.Writer) int {
	return followTaskWithCancel(ctx, backend, job, stdout, stderr, true)
}
func followTaskWithCancel(ctx context.Context, backend app.API, job *store.Job, stdout, stderr io.Writer, cancelOnInterrupt bool) int {
	code, _ := followTaskResult(ctx, backend, job, stdout, stderr, cancelOnInterrupt, false)
	return code
}

// followTaskResult follows a task to a terminal status and returns the exit code and the final job
// (nil when following stopped early). approvalFollows suppresses the "apply explicitly" hint when
// the caller applies or asks next.
func followTaskResult(ctx context.Context, backend app.API, job *store.Job, stdout, stderr io.Writer, cancelOnInterrupt, approvalFollows bool) (int, *store.Job) {
	fmt.Fprintf(stdout, "Task %s (%s)\n", job.ID, job.Mode)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var cursor int64
	for tick := 0; ; tick++ {
		events, err := backend.Events(ctx, job.ID, "local", cursor)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			fmt.Fprintln(stderr, err)
			fmt.Fprintf(stderr, "Task remains tracked by the daemon. Reattach with 'apw watch %s' (use the same --config).\n", job.ID)
			return 1, nil
		}
		statusChanged := false
		for _, event := range events {
			cursor = event.Seq
			if event.Stage == "task_status" {
				statusChanged = true
			}
			fmt.Fprintf(stdout, "[%s] %s\n", event.Stage, event.Message)
		}
		// Get returns the full job including the report blob, so only fetch it
		// when a status change was announced, plus a periodic fallback (and on
		// the first tick, which also covers jobs that are already terminal).
		if statusChanged || tick%followFallbackTicks == 0 {
			fresh, getErr := backend.Get(ctx, job.ID, "local")
			if getErr != nil {
				if ctx.Err() != nil {
					break
				}
				fmt.Fprintln(stderr, getErr)
				fmt.Fprintf(stderr, "Task remains tracked by the daemon. Reattach with 'apw watch %s' (use the same --config).\n", job.ID)
				return 1, nil
			}
			job = fresh
		}
		if terminal(job.Status) {
			var report executor.ExecutionReport
			if len(job.Report) > 0 && json.Unmarshal(job.Report, &report) == nil {
				fmt.Fprintln(stdout, renderMarkdownAnsi(report.Summary))
				if job.Status == core.StatusReady && !approvalFollows {
					fmt.Fprintf(stdout, "Review the diff with 'apw diff %s'. Apply explicitly with 'apw apply %s'.\n", job.ID, job.ID)
				}
			}
			fmt.Fprintf(stdout, "Status: %s\n", job.Status)
			if job.Error != "" {
				fmt.Fprintln(stderr, job.Error)
			}
			if job.Status == core.StatusReady || job.Status == core.StatusPreview || job.Status == core.StatusSucceeded {
				return 0, job
			}
			return 1, job
		}
		select {
		case <-ctx.Done():
			goto cancelled
		case <-ticker.C:
		}
	}
cancelled:
	if !cancelOnInterrupt {
		fmt.Fprintf(stdout, "Detached from %s; task continues.\n", job.ID)
		return 130, nil
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := backend.Cancel(cancelCtx, job.ID, "local"); err != nil {
		fmt.Fprintf(stderr, "Cancellation could not be confirmed: %v\n", err)
	}
	return 130, nil
}

func runTaskCommand(action string, args []string, stdout, stderr io.Writer) int {
	options, code := parseOptions(action, args, stderr)
	path := options.Config
	id := ""
	if len(options.Args) > 0 {
		id = options.Args[0]
	}
	if len(options.Args) > 1 || (action == "tasks" || action == "trust") && id != "" {
		fmt.Fprintln(stderr, "unexpected extra arguments")
		return 2
	}
	if code != 0 {
		return code
	}
	if action == "recover" {
		if id == "" {
			fmt.Fprintln(stderr, "Usage: apw recover --config <path> <legacy-task-id>")
			return 2
		}
		cfg, _, err := daemon.LoadConfig(path)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		report, err := executor.RecoverLegacyTask(ctx, cfg.Workspace, cfg.WorktreeRoot, id)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		_ = json.NewEncoder(stdout).Encode(report)
		return 0
	}
	if action == "trust" {
		cfg, abs, err := daemon.LoadConfig(path)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		cfg.Policy.Trusted = true
		if err = cfg.Save(abs); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "Trusted workspace for build/test: %s\n", cfg.Workspace)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backend, err := daemon.Connect(ctx, path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if action == "tasks" {
		jobs, err := backend.List(ctx, "local")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, job := range jobs {
			fmt.Fprintf(stdout, "%s %s %s\n", job.ID, job.Mode, job.Status)
		}
		return 0
	}
	if id == "" && action == "resume" {
		jobs, err := backend.List(ctx, "local")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, job := range jobs {
			if job.Status == core.StatusInterrupted || job.Status == core.StatusFailed || job.Status == core.StatusPreview {
				cfg, _, loadErr := daemon.LoadConfig(path)
				if loadErr != nil {
					fmt.Fprintln(stderr, loadErr)
					return 1
				}
				if _, err := executor.CheckpointForTask(cfg.Workspace, job.ID); err == nil {
					id = job.ID
					break
				}
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "Usage: apw %s [--config path] <task-id>\n", action)
		return 1
	}
	if action == "watch" || action == "diff" {
		job, err := backend.Get(ctx, id, "local")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if action == "watch" {
			return followTaskWithCancel(ctx, backend, job, stdout, stderr, false)
		}
		var report executor.ExecutionReport
		if len(job.Report) == 0 || json.Unmarshal(job.Report, &report) != nil {
			fmt.Fprintln(stderr, "task has no saved diff yet")
			return 1
		}
		fmt.Fprint(stdout, report.Diff)
		return 0
	}
	if action == "task" {
		job, err := backend.Get(ctx, id, "local")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		data, _ := json.MarshalIndent(job, "", "  ")
		fmt.Fprintln(stdout, string(data))
		return 0
	}
	if action == "cancel" {
		if err := backend.Cancel(ctx, id, "local"); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "Cancellation requested:", id)
		return 0
	}
	var job *store.Job
	if action == "apply" {
		job, err = backend.Apply(ctx, id, "local")
	} else if action == "resume" {
		job, err = backend.Resume(ctx, id, "local")
	} else {
		fmt.Fprintln(stderr, "unknown task operation")
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return followTask(ctx, backend, job, stdout, stderr)
}
