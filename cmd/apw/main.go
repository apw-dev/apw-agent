package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/apiwat/apw-agent/internal/architect"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/desktop"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/provider/factory"
	"github.com/apiwat/apw-agent/internal/reflex"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/tui"
)

const (
	version              = "0.1.0-alpha"
	maxOverviewSizeBytes = 20 * 1024
)

func main() {
	os.Exit(runWithStdin(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithStdin(args, strings.NewReader(""), stdout, stderr)
}

func runWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		return runInteractiveShell(stdin, stdout, stderr)
	}

	switch args[0] {
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "apw version %s (Triad Engine: Architect + Surgeon + Reflex)\n", version)
		return 0

	case "help", "-h", "--help":
		printBanner(stdout, ".")
		fmt.Fprintln(stdout)
		printHelp(stdout)
		return 0

	case "shell":
		return runInteractiveShell(stdin, stdout, stderr)

	case "verify":
		workspace := "."
		if len(args) > 1 {
			workspace = args[1]
		}
		return executeVerify(workspace, stdout)

	case "symbols":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: apw symbols <path/to/file.go>")
			return 1
		}
		return executeSymbols(args[1], stdout, stderr)

	case "plan":
		return runPlan(args[1:], stdout, stderr)

	case "run":
		return runExecute(args[1:], stdout, stderr)

	case "daemon":
		return runDaemonCmd(args[1:], stdout, stderr)

	case "model", "models", "config":
		return runModelCmdWithStdin(args[1:], stdin, stdout, stderr)

	case "skills":
		return runSkillsCmd(".", stdout)

	case "ui", "desktop":
		return runDesktopApp(args[1:], stdout, stderr)

	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func printBanner(w io.Writer, workspace string) {
	dState := daemon.EnsureRunning(workspace)
	wsDisplay := formatWorkspacePath(workspace)
	stack := config.DetectStack(workspace)

	fmt.Fprintln(w, "  ▄▀█ █▀█ █░█░█")
	fmt.Fprintf(w, "  █▀█ █▀▀ ▀▄▀▄▀   v%s (Apiwat Agent)\n", version)
	fmt.Fprintln(w, " ───────────────────────────────────────────────────────")
	fmt.Fprintf(w, "  Brain 1 (Reflex) : Reflex Engine [Active - %dms]\n", dState.LatencyMs)
	fmt.Fprintln(w, "  Brain 2 (Deep)   : Architect     [Standby]")
	fmt.Fprintf(w, "  Daemon (apwd)    : Running (PID: %d) | %d Skills Loaded\n", dState.PID, dState.SkillsLoaded)
	fmt.Fprintln(w, " ───────────────────────────────────────────────────────")
	fmt.Fprintf(w, "  Workspace        : %s (%s)\n", wsDisplay, stack)
}

func formatWorkspacePath(workspace string) string {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return workspace
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(strings.ToLower(abs), strings.ToLower(home)) {
		rel := abs[len(home):]
		return "~" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

func runInteractiveShell(stdin io.Reader, stdout, stderr io.Writer) int {
	sess := tui.NewSession(".", version)
	if stdin == os.Stdin {
		tui.RenderDashboard(stdout, ".", version, sess.SessionID)
	} else {
		printBanner(stdout, ".")
		fmt.Fprintln(stdout)
	}

	var scanner *bufio.Scanner
	for {
		var line string
		if stdin == os.Stdin {
			l, err := sess.ReadInteractiveLine(stdout)
			if err == nil {
				line = strings.TrimSpace(l)
			} else {
				// Fallback to standard scanner if raw terminal mode is not supported
				if scanner == nil {
					scanner = bufio.NewScanner(stdin)
				}
				fmt.Fprint(stdout, "apw > ")
				if !scanner.Scan() {
					fmt.Fprintln(stdout)
					return 0
				}
				line = strings.TrimSpace(scanner.Text())
			}
		} else {
			if scanner == nil {
				scanner = bufio.NewScanner(stdin)
			}
			fmt.Fprint(stdout, "apw > ")
			if !scanner.Scan() {
				fmt.Fprintln(stdout)
				return 0
			}
			line = strings.TrimSpace(scanner.Text())
		}

		if line == "" {
			continue
		}

		cmdLine := strings.TrimPrefix(line, "/")
		fields := strings.Fields(cmdLine)
		head := strings.ToLower(fields[0])
		rest := ""
		if len(fields) > 1 {
			rest = strings.TrimSpace(cmdLine[len(fields[0]):])
		}

		switch head {
		case "exit", "quit", "q":
			fmt.Fprintln(stdout, "Shutting down interactive session. Daemon (apwd) remains active in background.")
			return 0

		case "help", "?":
			printHelp(stdout)

		case "status":
			printBanner(stdout, ".")

		case "new", "reset":
			sess.ResetSession()
			fmt.Fprintf(stdout, "Started fresh session: %s\n", sess.SessionID)

		case "clear", "cls", "redraw":
			fmt.Fprint(stdout, "\033[H\033[2J")
			if stdin == os.Stdin {
				tui.RenderDashboard(stdout, ".", version, sess.SessionID)
			} else {
				printBanner(stdout, ".")
			}

		case "skills":
			_ = runSkillsCmd(".", stdout)

		case "verify":
			ws := "."
			if rest != "" {
				ws = rest
			}
			_ = executeVerify(ws, stdout)

		case "symbols":
			if rest == "" {
				fmt.Fprintln(stderr, "Usage: symbols <path/to/file.go>")
				continue
			}
			_ = executeSymbols(rest, stdout, stderr)

		case "plan":
			if rest == "" {
				fmt.Fprintln(stderr, "Usage: plan <task instruction>")
				continue
			}
			_ = runPlan([]string{rest}, stdout, stderr)

		case "run":
			if rest == "" {
				fmt.Fprintln(stderr, "Usage: run <task instruction>")
				continue
			}
			_ = runExecute([]string{rest}, stdout, stderr)

		case "daemon":
			var sub []string
			if rest != "" {
				sub = strings.Fields(rest)
			}
			_ = runDaemonCmd(sub, stdout, stderr)

		case "model", "models", "config":
			var sub []string
			if rest != "" {
				sub = strings.Fields(rest)
			}
			_ = runModelCmdWithStdin(sub, stdin, stdout, stderr)

		case "ui", "desktop":
			srv := desktop.NewServer("./apw.json", version)
			url, err := srv.StartAndOpen(context.Background(), "127.0.0.1:0", true)
			if err != nil {
				fmt.Fprintf(stderr, "Failed to start Desktop UI: %v\n", err)
			} else {
				fmt.Fprintf(stdout, "Launched Apiwat Agent Desktop App at %s\n", url)
			}

		default:
			// Natural language instruction typed directly at `apw > _`:
			// Execute full Triad loop (Architect -> Surgeon -> Reflex)
			_ = runExecute([]string{line}, stdout, stderr)
		}
		fmt.Fprintln(stdout)
	}
}

func runSkillsCmd(workspace string, stdout io.Writer) int {
	groups := daemon.DiscoverSkills(workspace)
	cats := []string{"triad-core", "runtime", "extensions", "system", "workspace"}
	fmt.Fprintln(stdout, "Available APW Skills & Modules:")
	for _, cat := range cats {
		items := groups[cat]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(stdout, "  %-12s : %s\n", cat, strings.Join(items, ", "))
	}
	return 0
}

func executeVerify(workspace string, stdout io.Writer) int {
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
}

func executeSymbols(filePath string, stdout, stderr io.Writer) int {
	s := surgeon.NewSurgeon()
	syms, err := s.InspectGoSymbols(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error inspecting symbols: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Found %d symbols in %s:\n", len(syms), filePath)
	for _, sym := range syms {
		fmt.Fprintf(stdout, "  - [%s] %s (lines %d-%d)\n", sym.Kind, sym.Name, sym.StartLine, sym.EndLine)
	}
	return 0
}

func runDaemonCmd(args []string, stdout, stderr io.Writer) int {
	action := "status"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}
	switch action {
	case "status", "start":
		st := daemon.EnsureRunning(".")
		fmt.Fprintf(stdout, "Daemon (apwd): Running (PID: %d) | %d Skills Loaded | Reflex Latency: %dms\n", st.PID, st.SkillsLoaded, st.LatencyMs)
		return 0
	case "stop":
		if err := daemon.Stop("."); err != nil {
			fmt.Fprintf(stderr, "Failed to stop daemon: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "Daemon (apwd) stopped.")
		return 0
	default:
		fmt.Fprintln(stderr, "Usage: apw daemon [status|start|stop]")
		return 2
	}
}

func runModelCmdWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfgPath := "./apw.json"
	if len(args) == 0 && stdin == os.Stdin {
		if rc := tui.RunInteractiveModelSelector(cfgPath, stdin, stdout, stderr); rc >= 0 {
			return rc
		}
	}

	cfg, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	if len(args) == 0 || strings.ToLower(args[0]) == "status" || strings.ToLower(args[0]) == "list" {
		archProv := cfg.Providers[cfg.Roles.Architect]
		reflexProv := cfg.Providers[cfg.Roles.Reflex]
		fmt.Fprintln(stdout, "Active Dual-Brain Model Routing:")
		fmt.Fprintf(stdout, "  Brain 1 (Reflex)    : %s -> %s (%s)\n", cfg.Roles.Reflex, reflexProv.Model, reflexProv.Type)
		fmt.Fprintf(stdout, "  Brain 2 (Architect) : %s -> %s (%s)\n", cfg.Roles.Architect, archProv.Model, archProv.Type)
		fmt.Fprintln(stdout, "\nConfigured Providers:")
		for name, p := range cfg.Providers {
			endpointInfo := p.Endpoint
			if endpointInfo == "" {
				endpointInfo = "built-in"
			}
			fmt.Fprintf(stdout, "  - %-15s | type=%-6s | model=%-24s | endpoint=%s\n", name, p.Type, p.Model, endpointInfo)
		}
		fmt.Fprintln(stdout, "\nCommands:")
		fmt.Fprintln(stdout, "  apw model                                Open interactive model selector")
		fmt.Fprintln(stdout, "  apw model init                           Generate ./apw.json in current workspace")
		fmt.Fprintln(stdout, "  apw model set <architect|reflex> <name>  Switch active provider for a brain role")
		return 0
	}

	action := strings.ToLower(args[0])
	switch action {
	case "init":
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(stderr, "Error saving %s: %v\n", cfgPath, err)
			return 1
		}
		fmt.Fprintf(stdout, "Saved model configuration to %s\n", cfgPath)
		return 0

	case "set":
		if len(args) < 3 {
			fmt.Fprintln(stderr, "Usage: apw model set <architect|reflex|all> <provider_name>")
			return 2
		}
		role := strings.ToLower(args[1])
		provName := args[2]
		if _, ok := cfg.Providers[provName]; !ok {
			fmt.Fprintf(stderr, "Unknown provider %q. Run 'apw model' to see available providers.\n", provName)
			return 1
		}
		switch role {
		case "architect", "brain2":
			cfg.Roles.Architect = provName
		case "reflex", "brain1":
			cfg.Roles.Reflex = provName
		case "all", "both":
			cfg.Roles.Architect = provName
			cfg.Roles.Reflex = provName
		default:
			fmt.Fprintf(stderr, "Unknown role %q (use: architect, reflex, or all)\n", role)
			return 2
		}
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(stderr, "Error saving config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Updated role %q -> %q in %s\n", role, provName, cfgPath)
		return 0

	default:
		fmt.Fprintln(stderr, "Usage: apw model [status|init|set <architect|reflex|all> <provider>]")
		return 2
	}
}

func runDesktopApp(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("desktop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "./apw.json", "path to configuration file")
	addr := fs.String("addr", "127.0.0.1:0", "listen address")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := desktop.NewServer(*cfgPath, version)
	url, err := srv.StartAndOpen(ctx, *addr, true)
	if err != nil {
		fmt.Fprintf(stderr, "Error starting Desktop UI: %v\n", err)
		return 1
	}

	printBanner(stdout, ".")
	fmt.Fprintf(stdout, "\n  Desktop App      : %s (Window Launched)\n", url)
	fmt.Fprintln(stdout, "  Press Ctrl+C to close Desktop service.")
	<-ctx.Done()
	return 0
}

func parseConfigAndTaskArgs(cmdName string, args []string, stderr io.Writer) (string, string, bool, int) {
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "./apw.json", "path to configuration file")

	var positional []string
	var flagArgs []string
	explicitConfig := false
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			if strings.Contains(args[i], "config") {
				explicitConfig = true
			}
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
		return "", "", false, 2
	}
	positional = append(positional, fs.Args()...)
	instruction := strings.TrimSpace(strings.Join(positional, " "))
	return *cfgPath, instruction, explicitConfig, 0
}

func loadConfigForCLI(cfgPath string, explicit bool) (*config.Config, error) {
	if explicit {
		return config.Load(cfgPath)
	}
	return config.LoadOrDefault(cfgPath)
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	cfgPath, instruction, explicit, errCode := parseConfigAndTaskArgs("plan", args, stderr)
	if errCode != 0 {
		return errCode
	}
	if instruction == "" {
		fmt.Fprintln(stderr, "Usage: apw plan [--config path] \"<task instruction>\"")
		return 1
	}

	cfg, err := loadConfigForCLI(cfgPath, explicit)
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

func runExecute(args []string, stdout, stderr io.Writer) int {
	cfgPath, instruction, explicit, errCode := parseConfigAndTaskArgs("run", args, stderr)
	if errCode != 0 {
		return errCode
	}
	if instruction == "" {
		fmt.Fprintln(stderr, "Usage: apw run [--config path] \"<task instruction>\"")
		return 1
	}

	cfg, err := loadConfigForCLI(cfgPath, explicit)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	archCfg := cfg.Providers[cfg.Roles.Architect]
	archProv, err := factory.New(archCfg)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating architect provider: %v\n", err)
		return 1
	}
	defer archProv.Close()

	reflexCfg := cfg.Providers[cfg.Roles.Reflex]
	reflexProv, err := factory.New(reflexCfg)
	if err != nil {
		reflexProv = archProv
	} else {
		defer reflexProv.Close()
	}

	overview := buildCodebaseOverview(cfg.Workspace)
	timeout := time.Duration(cfg.Budget.TaskTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Fprintf(stdout, "  [Brain 2 · Architect] Analyzing task & formulating ExecutionPlan...\n")
	ex := executor.New(cfg.Workspace, archProv, reflexProv, archCfg.Model, reflexCfg.Model, cfg.Budget.MaxRepairs)
	report, err := ex.Execute(ctx, instruction, overview)
	if err != nil {
		fmt.Fprintf(stderr, "Error executing task: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "  [Plan %s] Objective: %s (%d steps)\n", report.Plan.PlanID, report.Plan.Objective, len(report.Steps))
	for _, st := range report.Steps {
		fmt.Fprintf(stdout, "    ✓ [%s] %s — %s\n", st.Action, st.SubtaskID, st.Message)
	}
	if report.Passed {
		fmt.Fprintf(stdout, "  [Brain 1 · Reflex] PASS (%d attempt(s), %v)\n", len(report.ReflexAttempts), report.Duration.Round(time.Millisecond))
		return 0
	}
	fmt.Fprintf(stdout, "  [Brain 1 · Reflex] FAIL after %d attempt(s)\n", len(report.ReflexAttempts))
	return 1
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
	fmt.Fprintln(w, "apw — Apiwat Agent CLI & Desktop (Next-Gen Triad Architecture)")
	fmt.Fprintln(w, "Usage: apw [command] [arguments]")
	fmt.Fprintln(w, "\nInteractive & Desktop:")
	fmt.Fprintln(w, "  apw                            Launch interactive Triad CLI shell (apw > _)")
	fmt.Fprintln(w, "  apw desktop (or apw ui)        Launch Claude Code Desktop-style Windows App")
	fmt.Fprintln(w, "\nCommands:")
	fmt.Fprintln(w, "  run [--config path] \"<task>\"   Execute full Triad loop (Architect -> Surgeon -> Reflex)")
	fmt.Fprintln(w, "  plan [--config path] \"<task>\"  Generate an ExecutionPlan and save to .apw/plans/")
	fmt.Fprintln(w, "  verify [dir]                   Run Reflex closed-loop verification (fmt -> vet -> build -> test)")
	fmt.Fprintln(w, "  symbols <file.go>              Inspect AST code symbols using Surgeon inspector")
	fmt.Fprintln(w, "  model [status|init|set ...]    View or configure Brain 1 (Reflex) & Brain 2 (Architect) models")
	fmt.Fprintln(w, "  daemon [status|start|stop]     Manage apwd background agent daemon")
	fmt.Fprintln(w, "  version                        Show version and architecture info")
}
