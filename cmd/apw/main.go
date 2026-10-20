package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/desktop"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/tui"
)

const (
	maxOverviewSizeBytes = 20 * 1024
)

var version = buildinfo.Version

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
		if len(args) > 1 && args[1] == "--json" {
			_ = json.NewEncoder(stdout).Encode(buildinfo.Current())
			return 0
		}
		fmt.Fprintf(stdout, "apw version %s (Triad Engine: Architect + Surgeon + Reflex)\n", version)
		fmt.Fprintf(stdout, "source %s | revision %s | built %s\n", buildinfo.SourceHash, buildinfo.Revision, buildinfo.BuiltAt)
		return 0

	case "help", "-h", "--help":
		printBanner(stdout, ".")
		fmt.Fprintln(stdout)
		printHelp(stdout)
		return 0

	case "shell":
		o, code := parseOptions("shell", args[1:], stderr)
		if code != 0 {
			return code
		}
		if len(o.Args) > 0 {
			fmt.Fprintln(stderr, "unexpected shell arguments")
			return 2
		}
		return runShellConfig(o.Config, stdin, stdout, stderr)

	case "verify":
		return runVerify(args[1:], stdout, stderr)

	case "symbols":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: apw symbols <path/to/file>")
			return 1
		}
		return executeSymbols(args[1], stdout, stderr)

	case "search":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "Usage: apw search <query>")
			return 1
		}
		return executeSearch(".", strings.Join(args[1:], " "), stdout, stderr)

	case "plan":
		return runPlan(args[1:], stdout, stderr)

	case "review":
		return runSubmit(core.ModeReview, args[1:], stdout, stderr)
	case "preview":
		return runSubmit(core.ModePreview, args[1:], stdout, stderr)
	case "apply", "task", "tasks", "watch", "diff", "cancel", "trust", "recover":
		return runTaskCommand(args[0], args[1:], stdout, stderr)
	case "run":
		return runExecute(args[1:], stdout, stderr)
	case "cleanup":
		return runCleanup(args[1:], stdout, stderr)
	case "eval":
		return runEval(args[1:], stdout, stderr)
	case "issue":
		return runIssue(args[1:], stdin, stdout, stderr)
	case "acp":
		return runACP(args[1:], stdin, stdout, stderr)

	case "resume":
		return runResume(nil, args[1:], stdout, stderr)

	case "daemon":
		return runDaemonCmd(args[1:], stdout, stderr)

	case "model", "models", "config":
		return runModelCmdWithStdin(args[1:], stdin, stdout, stderr)

	case "auth", "login":
		return runAuthCmdWithStdin(args[1:], stdin, stdout, stderr)

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
	fmt.Fprintln(w, "  Brain 1 (Reflex) : Reflex Engine [Available]")
	fmt.Fprintln(w, "  Brain 2 (Deep)   : Architect     [Standby]")
	state := "Stopped"
	if dState.Running {
		state = "Running"
	}
	fmt.Fprintf(w, "  Daemon (apwd)    : %s (PID: %d) | %d Skills Loaded\n", state, dState.PID, daemon.CountSkills(workspace))
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
	return runVerify([]string{workspace}, stdout, stdout)
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

func executeSearch(workspace, query string, stdout, stderr io.Writer) int {
	query = strings.TrimSpace(query)
	if query == "" {
		fmt.Fprintln(stderr, "Usage: apw search <query>")
		return 1
	}
	graph, err := surgeon.InspectWorkspaceGraph(workspace)
	if err != nil {
		fmt.Fprintf(stderr, "Error inspecting workspace graph: %v\n", err)
		return 1
	}
	defs := graph.FindSymbols(query)
	fmt.Fprintf(stdout, "Symbol matches for %q (%d found):\n", query, len(defs))
	for i, d := range defs {
		if i >= 20 {
			fmt.Fprintf(stdout, "  ... and %d more\n", len(defs)-i)
			break
		}
		fmt.Fprintf(stdout, "  - [%s] %s (%s:%d-%d)\n", d.Kind, d.Name, d.File, d.StartLine, d.EndLine)
	}
	return 0
}

func runDaemonCmd(args []string, stdout, stderr io.Writer) int {
	options, code := parseOptions("daemon", args, stderr)
	if code != 0 {
		return code
	}
	if len(options.Args) > 1 {
		fmt.Fprintln(stderr, "Usage: apw daemon [--config path] [status|start|stop]")
		return 2
	}
	action := "status"
	if len(options.Args) > 0 {
		action = strings.ToLower(options.Args[0])
	}
	if action != "status" && action != "start" && action != "stop" {
		fmt.Fprintln(stderr, "unknown daemon action:", action)
		return 2
	}
	cfg, _, err := daemon.LoadConfig(options.Config)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if action == "stop" {
		if err := daemon.Stop(cfg.Workspace); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "Daemon (apwd) shutdown requested.")
		return 0
	}
	if action == "start" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := daemon.Connect(ctx, options.Config); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	st := daemon.Status(cfg.Workspace)
	fmt.Fprintf(stdout, "Daemon running=%v PID=%d health latency=%dms workspace=%s\n", st.Running, st.PID, st.LatencyMs, cfg.Workspace)
	return 0
}

func runAuthCmdWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, code := parseOptions("model", args, stderr)
	if code != 0 {
		return code
	}
	args = options.Args
	cfgPath := options.Config
	if _, err := config.LoadOrDefault(cfgPath); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(args) == 0 && stdin == os.Stdin {
		if rc := tui.RunInteractiveAuthMenu(cfgPath, stdin, stdout, stderr); rc >= 0 {
			return rc
		}
	}

	cfg, statuses, err := tui.LoadSyncedConfig(".", cfgPath, true)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := cfg.Save(cfgPath); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintln(stdout, "APW Authentication & Live Model Discovery:")
	for _, st := range statuses {
		mark := "○"
		if st.Connected {
			mark = "✓"
		}
		fmt.Fprintf(stdout, "  %s %-32s : %s\n", mark, st.Name, st.Detail)
		for _, m := range st.Models {
			fmt.Fprintf(stdout, "      - %-28s | %s (%s)\n", m.ProviderKey, m.DisplayName, m.ProviderType)
		}
	}
	return 0
}

func runModelCmdWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, code := parseOptions("model", args, stderr)
	if code != 0 {
		return code
	}
	args = options.Args
	cfgPath := options.Config
	if _, err := config.LoadOrDefault(cfgPath); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(args) == 0 && stdin == os.Stdin {
		if rc := tui.RunInteractiveModelSelector(cfgPath, stdin, stdout, stderr); rc >= 0 {
			return rc
		}
	}

	cfg, statuses, err := tui.LoadSyncedConfig(".", cfgPath, len(args) > 0 && strings.ToLower(args[0]) == "refresh")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if cfg == nil {
		fmt.Fprintln(stderr, "Error loading config")
		return 1
	}
	if len(args) > 0 && strings.ToLower(args[0]) == "refresh" {
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if len(args) == 0 || strings.ToLower(args[0]) == "status" || strings.ToLower(args[0]) == "list" || strings.ToLower(args[0]) == "refresh" {
		archProv := cfg.Providers[cfg.Roles.Architect]
		reflexProv := cfg.Providers[cfg.Roles.Reflex]
		fmt.Fprintln(stdout, "Active Dual-Brain Model Routing:")
		fmt.Fprintf(stdout, "  Brain 1 (Reflex)    : %s -> %s (%s)\n", cfg.Roles.Reflex, reflexProv.Model, reflexProv.Type)
		fmt.Fprintf(stdout, "  Brain 2 (Architect) : %s -> %s (%s)\n", cfg.Roles.Architect, archProv.Model, archProv.Type)
		fmt.Fprintln(stdout, "\nAuthenticated Sources:")
		for _, st := range statuses {
			mark := "○"
			if st.Connected {
				mark = "✓"
			}
			fmt.Fprintf(stdout, "  %s %-32s : %s\n", mark, st.Name, st.Detail)
		}
		fmt.Fprintln(stdout, "\nCommands:")
		fmt.Fprintln(stdout, "  apw model                                Open interactive live model selector")
		fmt.Fprintln(stdout, "  apw auth                                 Connect ChatGPT Sub/API, Claude Sub/API, or AGY CLI")
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
			fmt.Fprintf(stderr, "Unknown provider %q. Run 'apw auth' or 'apw model' to see available providers.\n", provName)
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
		fmt.Fprintln(stderr, "Usage: apw model [status|refresh|init|set <architect|reflex|all> <provider>]")
		return 2
	}
}

func runDesktopApp(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("desktop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "./apw.json", "path to configuration file")
	addr := fs.String("addr", "127.0.0.1:0", "listen address")
	noOpen := fs.Bool("no-open", false, "serve without opening a browser")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if !*noOpen && *addr == "127.0.0.1:0" {
		if launched, wsDir := tryLaunchNativeDesktop(*cfgPath); launched {
			fmt.Fprintf(stdout, "Launched APW.exe for workspace %s\n", wsDir)
			return 0
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := desktop.NewServer(*cfgPath, version)
	url, err := srv.StartAndOpen(ctx, *addr, !*noOpen)
	if err != nil {
		fmt.Fprintf(stderr, "Error starting Desktop UI: %v\n", err)
		return 1
	}

	printBanner(stdout, ".")
	fmt.Fprintf(stdout, "\n  Desktop App      : %s\n", url)
	fmt.Fprintln(stdout, "  Press Ctrl+C to close Desktop service.")
	<-ctx.Done()
	return 0
}

// findSiblingNativeDesktop returns the path to the native desktop executable
// (desktop/APW.exe, apw-desktop.exe, ../APW.exe, or APW.exe when distinct from exePath)
// when installed alongside exePath on Windows.
func findSiblingNativeDesktop(goos, exePath string) (string, bool) {
	if goos != "windows" || exePath == "" {
		return "", false
	}
	dir := filepath.Dir(exePath)
	selfInfo, _ := os.Stat(exePath)
	candidates := []string{
		filepath.Join(dir, "desktop", "APW.exe"),
		filepath.Join(dir, "apw-desktop.exe"),
		filepath.Join(dir, "APW.exe"),
		filepath.Join(filepath.Dir(dir), "APW.exe"),
	}
	for _, candidate := range candidates {
		fi, err := os.Stat(candidate)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if selfInfo != nil && os.SameFile(selfInfo, fi) {
			continue
		}
		if strings.EqualFold(filepath.Clean(candidate), filepath.Clean(exePath)) {
			continue
		}
		return candidate, true
	}
	return "", false
}

func resolveDesktopWorkspace(cfgPath string) string {
	if cfg, _, err := daemon.LoadConfig(cfgPath); err == nil && cfg.Workspace != "" {
		if abs, err := filepath.Abs(cfg.Workspace); err == nil {
			return abs
		}
		return cfg.Workspace
	}
	if abs, err := filepath.Abs(filepath.Dir(cfgPath)); err == nil {
		return abs
	}
	return "."
}

func tryLaunchNativeDesktop(cfgPath string) (bool, string) {
	exe, err := os.Executable()
	if err != nil {
		return false, ""
	}
	nativeBin, ok := findSiblingNativeDesktop(runtime.GOOS, exe)
	if !ok {
		return false, ""
	}
	wsDir := resolveDesktopWorkspace(cfgPath)
	cmd := exec.Command(nativeBin, wsDir)
	if err := cmd.Start(); err != nil {
		return false, ""
	}
	_ = cmd.Process.Release()
	return true, wsDir
}

func parseConfigAndTaskArgs(cmdName string, args []string, stderr io.Writer) (string, string, bool, int) {
	options, code := parseOptions(cmdName, args, stderr)
	return options.Config, strings.TrimSpace(strings.Join(options.Args, " ")), options.Explicit, code
}
func loadConfigForCLI(cfgPath string, explicit bool) (*config.Config, error) {
	cfg, _, err := daemon.LoadConfig(cfgPath)
	return cfg, err
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	timeout := time.Duration(cfg.Budget.TaskTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	backend, err := daemon.Connect(ctx, cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	plan, err := backend.Plan(ctx, "local", instruction)
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

type triadSpinner struct {
	w       io.Writer
	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	msg     string
	started time.Time
}

func startSpinner(w io.Writer, initialMsg string) *triadSpinner {
	s := &triadSpinner{
		w:       w,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		msg:     initialMsg,
		started: time.Now(),
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	go func() {
		defer close(s.done)
		idx := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				fmt.Fprint(s.w, "\r\x1b[K")
				return
			case <-ticker.C:
				s.mu.Lock()
				elapsed := time.Since(s.started).Round(100 * time.Millisecond)
				fmt.Fprintf(s.w, "\r  \x1b[38;2;86;182;242m%s\x1b[0m %s \x1b[2m[%v]\x1b[0m", frames[idx%len(frames)], s.msg, elapsed)
				s.mu.Unlock()
				idx++
			}
		}
	}()
	return s
}

func (s *triadSpinner) Update(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msg = msg
}

func (s *triadSpinner) Stop() {
	if s == nil {
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
		<-s.done
	}
}

func runExecute(args []string, stdout, stderr io.Writer) int {
	return runSubmit(core.ModeExecute, args, stdout, stderr)
}
func runExecuteWithSession(sess *session.Session, args []string, stdout, stderr io.Writer) int {
	if sess != nil && sess.ID != "" {
		args = append([]string{"--session", sess.ID}, args...)
	}
	return runSubmit(core.ModeExecute, args, stdout, stderr)
}
func runResume(_ *session.Session, args []string, stdout, stderr io.Writer) int {
	return runTaskCommand("resume", args, stdout, stderr)
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
	fmt.Fprintln(w, "  apw desktop (or apw ui)        Launch the APW Desktop window")
	fmt.Fprintln(w, "  Ctrl+C twice (within 2s)       Safely exit the interactive shell (Ctrl+D exits immediately)")
	fmt.Fprintln(w, "\nCommands:")
	fmt.Fprintln(w, "  review [--config path] <task>   Read source and report findings")
	fmt.Fprintln(w, "  preview [--config path] <task>  Prepare a diff without executing repository code")
	fmt.Fprintln(w, "  trust [--config path]          Allow build/test for this workspace")
	fmt.Fprintln(w, "  tasks | task <task-id>         List jobs or inspect a saved JSON report")
	fmt.Fprintln(w, "  diff <task-id>                 Show the saved source diff")
	fmt.Fprintln(w, "  recover <legacy-task-id>       Export old work safely for review; never apply")
	fmt.Fprintln(w, "  cleanup [--dry-run]            Remove old finished task state under the retention policy")
	fmt.Fprintln(w, "  eval [--suite eval/testdata]   Measure APW on fixture tasks with hidden tests (--only, --baseline, --keep)")
	fmt.Fprintln(w, "  issue <number|url> [--yes]     Turn a GitHub issue into a verified change on a branch and a draft PR (gh)")
	fmt.Fprintln(w, "  acp [--config path]            Serve the Agent Client Protocol on stdio (for ACP editors and agent hubs)")
	fmt.Fprintln(w, "  watch <task-id>                Reattach to progress; Ctrl+C detaches")
	fmt.Fprintln(w, "  review/preview/run flags: --request-id <id> --session <id> --config <path>")
	fmt.Fprintln(w, "  cancel <task-id>               Cancel a task and its subprocess tree")
	fmt.Fprintln(w, "  apply <task-id>                Apply a verified patch with conflict checks")
	fmt.Fprintln(w, "  run [--config path] \"<task>\"   Prepare and verify a patch in an isolated worktree")
	fmt.Fprintln(w, "    --auto                       Apply automatically once verification and acceptance pass")
	fmt.Fprintln(w, "    --no-verify                  Unverified mode: patch without install/build/test (no trust needed)")
	fmt.Fprintln(w, "  shell: /mode ask|auto|plan|unverified (shift+tab cycles); default from apw.json \"approval\"")
	fmt.Fprintln(w, "  resume [task-id]               Resume saved task; preview resumes as execute, unverified stays unverified")
	fmt.Fprintln(w, "  plan [--config path] \"<task>\"  Generate an ExecutionPlan and save to .apw/plans/")
	fmt.Fprintln(w, "  verify [dir]                   Run Reflex closed-loop verification (fmt -> vet -> build -> test)")
	fmt.Fprintln(w, "  symbols <file.go>              Inspect AST code symbols using Surgeon inspector")
	fmt.Fprintln(w, "  model [status|refresh|set ...] Select live models for Brain 1 (Reflex) & Brain 2 (Architect)")
	fmt.Fprintln(w, "  auth                           Connect ChatGPT Sub/API, Claude Sub/API, or AGY CLI")
	fmt.Fprintln(w, "  daemon [status|start|stop]     Manage apwd background agent daemon")
	fmt.Fprintln(w, "  version                        Show version and architecture info")
}

func renderMarkdownAnsi(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "### ") {
			out = append(out, fmt.Sprintf("  \x1b[1m\x1b[38;2;86;182;242m%s\x1b[0m", trimmed))
		} else if strings.HasPrefix(trimmed, "#### ") {
			out = append(out, fmt.Sprintf("  \x1b[1m\x1b[38;2;198;120;221m%s\x1b[0m", trimmed))
		} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "• ") {
			content := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "• ")
			out = append(out, fmt.Sprintf("    \x1b[38;2;97;175;239m•\x1b[0m %s", content))
		} else if trimmed == "---" {
			out = append(out, "  \x1b[2m\x1b[38;2;90;98;110m─────────────────────────────────────────────────────────────\x1b[0m")
		} else if trimmed != "" {
			out = append(out, "  "+l)
		} else {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}
