package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/provider/factory"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

const (
	version              = "0.1.0-alpha"
	maxOverviewSizeBytes = 20 * 1024
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "apw version %s (Triad Engine: Architect + Surgeon + Reflex)\n", version)
		return 0

	case "verify":
		workspace := "."
		if len(args) > 1 {
			workspace = args[1]
		}
		fmt.Fprintf(stdout, "Running Reflex closed-loop verification on: %s...\n", workspace)
		v := reflex.NewVerifier(workspace)
		res := v.Verify(context.Background())
		if res.Passed {
			fmt.Fprintf(stdout, "PASS: All checks (fmt, vet, build, test) passed in %v!\n", res.Duration)
			return 0
		}
		fmt.Fprintf(stdout, "FAIL: Verification failed at stage: %s\n", res.FailedStage)
		fmt.Fprintln(stdout, reflex.FormatDiagnostics(res))
		return 1

	case "symbols":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: apw symbols <path/to/file.go>")
			return 1
		}
		s := surgeon.NewSurgeon()
		syms, err := s.InspectGoSymbols(args[1])
		if err != nil {
			fmt.Fprintf(stderr, "Error inspecting symbols: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Found %d symbols in %s:\n", len(syms), args[1])
		for _, sym := range syms {
			fmt.Fprintf(stdout, "  - [%s] %s (lines %d-%d)\n", sym.Kind, sym.Name, sym.StartLine, sym.EndLine)
		}
		return 0

	case "plan":
		return runPlan(args[1:], stdout, stderr)

	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "./apw.json", "path to configuration file")

	var positional []string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			if !strings.Contains(args[i], "=") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			positional = append(positional, args[i])
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	positional = append(positional, fs.Args()...)

	instruction := strings.TrimSpace(strings.Join(positional, " "))
	if instruction == "" {
		fmt.Fprintln(stderr, "Usage: apw plan [--config path] \"<task instruction>\"")
		return 1
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	provCfg := cfg.Providers[cfg.Roles.Architect]
	prov, err := factory.New(provCfg)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating architect provider: %v\n", err)
		return 1
	}
	defer prov.Close()

	overview := buildCodebaseOverview(cfg.Workspace)

	timeout := time.Duration(cfg.Budget.TaskTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	arch := architect.NewArchitect(prov)
	arch.Model = provCfg.Model

	task := core.Task{
		ID:          fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Workspace:   cfg.Workspace,
		Instruction: instruction,
		Mode:        "preview",
		Status:      core.StatusPlanning,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	plan, err := arch.Plan(ctx, task, overview)
	if err != nil {
		fmt.Fprintf(stderr, "Error generating plan: %v\n", err)
		return 1
	}

	outBytes, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "Error marshaling plan: %v\n", err)
		return 1
	}

	plansDir := filepath.Join(cfg.Workspace, ".apw", "plans")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		fmt.Fprintf(stderr, "Error creating plans directory: %v\n", err)
		return 1
	}

	planFile := filepath.Join(plansDir, plan.PlanID+".json")
	if err := os.WriteFile(planFile, outBytes, 0644); err != nil {
		fmt.Fprintf(stderr, "Error writing plan file: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, string(outBytes))
	return 0
}

func buildCodebaseOverview(workspace string) string {
	s := surgeon.NewSurgeon()
	var sb strings.Builder

	_ = filepath.WalkDir(workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "testdata" || name == ".apw" || name == ".worktrees" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if sb.Len() >= maxOverviewSizeBytes {
			return filepath.SkipAll
		}

		rel, relErr := filepath.Rel(workspace, path)
		if relErr != nil {
			rel = path
		}
		syms, symErr := s.InspectGoSymbols(path)
		if symErr != nil {
			return nil
		}

		line := fmt.Sprintf("File: %s\n", filepath.ToSlash(rel))
		if sb.Len()+len(line) > maxOverviewSizeBytes {
			return filepath.SkipAll
		}
		sb.WriteString(line)

		for _, sym := range syms {
			entry := fmt.Sprintf("  - [%s] %s (lines %d-%d)\n", sym.Kind, sym.Name, sym.StartLine, sym.EndLine)
			if sb.Len()+len(entry) > maxOverviewSizeBytes {
				return filepath.SkipAll
			}
			sb.WriteString(entry)
		}
		return nil
	})

	if sb.Len() == 0 {
		return "(no Go symbols found)"
	}
	return sb.String()
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "apw — Apiwat Agent CLI (Next-Gen Triad Architecture)")
	fmt.Fprintln(w, "Usage: apw <command> [arguments]")
	fmt.Fprintln(w, "\nAvailable commands:")
	fmt.Fprintln(w, "  version                        Show version and architecture info")
	fmt.Fprintln(w, "  plan [--config path] \"<task>\"  Generate an ExecutionPlan and save to .apw/plans/")
	fmt.Fprintln(w, "  verify [dir]                   Run Reflex closed-loop verification (fmt -> vet -> build -> test)")
	fmt.Fprintln(w, "  symbols <file.go>              Inspect AST code symbols using Surgeon inspector")
	fmt.Fprintln(w, "  daemon                         Manage background agent daemon (not implemented)")
}
