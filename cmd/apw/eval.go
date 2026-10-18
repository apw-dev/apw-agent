package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/eval"
)

// runEval measures APW on the task suite: apw eval [--config path] [--suite dir] [--only a,b]
// [--out report.json] [--baseline previous.json] [--keep] [--json].
func runEval(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "./apw.json", "configuration whose providers, roles and settings are evaluated")
	suite := fs.String("suite", filepath.Join("eval", "testdata"), "task suite directory")
	only := fs.String("only", "", "comma-separated task names to run")
	out := fs.String("out", "", "write the JSON report here (default .apw/eval/<time>.json)")
	baseline := fs.String("baseline", "", "previous JSON report to compare against")
	keep := fs.Bool("keep", false, "keep each task's temporary workspace for inspection")
	jsonMode := fs.Bool("json", false, "print the JSON report instead of the table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "unexpected arguments:", strings.Join(fs.Args(), " "))
		return 2
	}
	cfg, _, err := daemon.LoadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	tasks, err := eval.LoadSuite(*suite)
	if err != nil {
		fmt.Fprintln(stderr, "load suite:", err)
		return 1
	}
	var prev *eval.Report
	if *baseline != "" {
		data, err := os.ReadFile(*baseline)
		if err == nil {
			prev = &eval.Report{}
			err = json.Unmarshal(data, prev)
		}
		if err != nil {
			fmt.Fprintln(stderr, "baseline:", err)
			return 1
		}
	}
	var names []string
	if *only != "" {
		names = strings.Split(*only, ",")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(stdout, "Evaluating %d task(s) from %s with %s\n", len(tasks), *suite, *cfgPath)
	report, err := eval.Run(ctx, tasks, eval.Options{
		Base: cfg, CredentialRoot: cfg.Workspace, Only: names, Keep: *keep,
		Progress: func(task, msg string) { fmt.Fprintf(stdout, "[%s] %s\n", task, msg) },
	})
	if report == nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	data, _ := json.MarshalIndent(report, "", "  ")
	path := *out
	if path == "" {
		path = filepath.Join(".apw", "eval", report.StartedAt.Format("20060102-150405")+".json")
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0700); mkErr == nil {
		if wErr := os.WriteFile(path, data, 0600); wErr != nil {
			fmt.Fprintln(stderr, "write report:", wErr)
		}
	}
	if *jsonMode {
		fmt.Fprintln(stdout, string(data))
	} else {
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, report.Markdown(prev))
		fmt.Fprintf(stdout, "Report: %s (%v)\n", path, time.Since(report.StartedAt).Round(time.Second))
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if report.FalsePositive > 0 || report.Errors > 0 {
		return 1
	}
	return 0
}
