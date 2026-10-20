package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/core"
	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/desktop"
	"github.com/apiwat/apw-agent/internal/session"
	"github.com/apiwat/apw-agent/internal/store"
	"github.com/apiwat/apw-agent/internal/tui"
)

// readLineUnbuffered reads one line byte by byte, so nothing typed after it is consumed from a
// terminal that the interactive line editor reads next.
func readLineUnbuffered(r io.Reader) string {
	var line []byte
	b := make([]byte, 1)
	for len(line) < 4096 {
		n, err := r.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				break
			}
			line = append(line, b[0])
		}
		if err != nil {
			break
		}
	}
	return strings.TrimRight(string(line), "\r")
}

func runInteractiveShell(stdin io.Reader, stdout, stderr io.Writer) int {
	return runShellConfig("./apw.json", stdin, stdout, stderr)
}

func runShellConfig(configPath string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, _, err := daemon.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, closeShell := context.WithCancel(context.Background())
	defer closeShell()
	conversation := &session.Session{}
	sess := tui.NewSession(cfg.Workspace, version)
	sess.ConfigPath = configPath
	sess.SessionID = "local-default"
	if cfg.Approval != "" {
		sess.SetApprovalMode(cfg.Approval)
	}
	rawInput := stdin == os.Stdin
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	if rawInput {
		tui.RenderDashboardConfig(stdout, cfg.Workspace, configPath, version, sess.SessionID)
	} else {
		printBanner(stdout, cfg.Workspace)
	}
	// readAnswer asks a yes/no question; anything but y/yes (including end of input) is no.
	readAnswer := func(question string) bool {
		fmt.Fprint(stdout, question)
		var answer string
		if rawInput {
			answer = readLineUnbuffered(stdin)
		} else if scanner.Scan() {
			answer = scanner.Text()
		} else {
			fmt.Fprintln(stdout)
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
	withConfig := func(values []string) []string {
		for _, value := range values {
			if value == "--config" || value == "-config" || strings.HasPrefix(value, "--config=") {
				return values
			}
		}
		if _, err := os.Stat(configPath); err == nil {
			return append([]string{"--config", configPath}, values...)
		}
		return values
	}
	// ensureTrust explains what trust allows and asks for it; it reports whether the workspace is
	// trusted afterwards.
	ensureTrust := func() bool {
		if fresh, _, err := daemon.LoadConfig(configPath); err == nil {
			cfg = fresh
		}
		if cfg.Policy.Trusted {
			return true
		}
		fmt.Fprintf(stdout, "\nWorkspace %s is not trusted.\n", cfg.Workspace)
		fmt.Fprintln(stdout, "To verify changes, APW runs this project's own install, build and test commands in an isolated worktree with your user permissions.")
		fmt.Fprintln(stdout, "Trust only folders whose code you trust. Without trust you can still plan, review, preview and use /mode unverified.")
		if cfg.Sandbox.Mode == "" || cfg.Sandbox.Mode == "off" {
			fmt.Fprintln(stdout, `Tip: "sandbox": {"mode": "restricted"} in apw.json limits what project code can write.`)
		}
		if !readAnswer("Trust this folder? [y/N]: ") {
			fmt.Fprintln(stdout, "Not trusted. Use /trust later, or /mode plan or /mode unverified.")
			return false
		}
		if runTaskCommand("trust", withConfig(nil), stdout, stderr) != 0 {
			return false
		}
		if fresh, _, err := daemon.LoadConfig(configPath); err == nil {
			cfg = fresh
		}
		return cfg.Policy.Trusted
	}
	if rawInput && !cfg.Policy.Trusted {
		ensureTrust()
	}
	fmt.Fprintf(stdout, "Mode: %s\n", sess.ApprovalModeLabel())
	approve := func(job *store.Job) bool {
		what := "these verified changes"
		if core.TaskMode(job.Mode) == core.ModeUnverified {
			what = "these UNVERIFIED changes (no build or test ran)"
		}
		return readAnswer(fmt.Sprintf("Apply %s to %s? [y/N]: ", what, cfg.Workspace))
	}
	// request runs a task the way the current (or given) approval mode says.
	request := func(args []string, approval string) {
		if approval == "plan" {
			_ = runPlan(withConfig(args), stdout, stderr)
			return
		}
		var flags []string
		switch approval {
		case "auto":
			flags = append(flags, "--auto")
		case "unverified":
			flags = append(flags, "--no-verify")
		}
		if approval != "unverified" && !ensureTrust() {
			return
		}
		if conversation.ID != "" {
			flags = append(flags, "--session", conversation.ID)
		}
		options, code := parseOptions("execute", withConfig(append(flags, args...)), stderr)
		if code != 0 {
			return
		}
		if approval != "auto" {
			options.Approve = approve
		}
		_ = runSubmitOptions(core.ModeExecute, options, stdout, stderr)
	}
	for {
		var line string
		if rawInput {
			line, err = sess.ReadInteractiveLine(stdout)
			if errors.Is(err, io.EOF) {
				return 0
			}
			if err != nil {
				rawInput = false
			}
		}
		if !rawInput {
			fmt.Fprint(stdout, "apw > ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					fmt.Fprintln(stderr, "shell input:", err)
					return 1
				}
				fmt.Fprintln(stdout)
				return 0
			}
			line = scanner.Text()
		}
		line = strings.TrimSpace(line)
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "/")))
		if len(fields) == 0 {
			continue
		}
		head := strings.ToLower(fields[0])
		commandLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
		rest := strings.TrimSpace(commandLine[len(fields[0]):])
		instructionCommand := head == "run" || head == "review" || head == "preview" || head == "plan"
		parseArgs := false
		switch head {
		case "verify", "symbols", "search", "resume", "apply", "task", "tasks", "cancel", "trust", "watch", "diff", "recover", "cleanup", "daemon", "model", "models", "config", "auth", "login", "mode":
			parseArgs = true
		}
		var args []string
		if !parseArgs && (!instructionCommand || !strings.HasPrefix(rest, "-")) {
			if rest != "" {
				args = []string{rest}
			}
		} else {
			args, err = splitCommand(rest)
			if err != nil {
				fmt.Fprintln(stderr, err)
				continue
			}
		}
		switch head {
		case "exit", "quit", "q":
			fmt.Fprintln(stdout, "Interactive session closed. Task state remains in .apw.")
			return 0
		case "help", "?":
			printHelp(stdout)
		case "status":
			printBanner(stdout, cfg.Workspace)
		case "new", "reset":
			conversation = &session.Session{ID: app.NewID()}
			sess.ResetSession()
			sess.SessionID = conversation.ID
			fmt.Fprintf(stdout, "Started fresh conversation: %s\n", conversation.ID)
		case "clear", "cls", "redraw":
			fmt.Fprint(stdout, "\033[H\033[2J")
			if rawInput {
				tui.RenderDashboardConfig(stdout, cfg.Workspace, configPath, version, sess.SessionID)
			} else {
				printBanner(stdout, cfg.Workspace)
			}
		case "skills":
			_ = runSkillsCmd(cfg.Workspace, stdout)
		case "symbols":
			if len(args) != 1 {
				fmt.Fprintln(stderr, "Usage: symbols <file>")
				break
			}
			path := args[0]
			if !filepath.IsAbs(path) {
				path = filepath.Join(cfg.Workspace, path)
			}
			_ = executeSymbols(path, stdout, stderr)
		case "search":
			if len(args) == 0 {
				fmt.Fprintln(stderr, "Usage: search <query>")
				break
			}
			_ = executeSearch(cfg.Workspace, strings.Join(args, " "), stdout, stderr)
		case "plan":
			_ = runPlan(withConfig(args), stdout, stderr)
		case "mode":
			if len(args) == 0 {
				fmt.Fprintf(stdout, "Mode: %s\nAvailable: ask, auto, plan, unverified\n", sess.ApprovalModeLabel())
				break
			}
			if !sess.SetApprovalMode(args[0]) {
				fmt.Fprintln(stderr, "unknown mode:", args[0], "(use ask, auto, plan or unverified)")
				break
			}
			fmt.Fprintf(stdout, "Mode: %s\n", sess.ApprovalModeLabel())
		case "run":
			// An explicit /run always verifies; it applies automatically only in auto mode.
			approval := "ask"
			if sess.ApprovalMode() == "auto" {
				approval = "auto"
			}
			request(args, approval)
		case "review", "preview":
			mode := core.ModeReview
			if head == "preview" {
				mode = core.ModePreview
			}
			if conversation.ID != "" {
				args = append([]string{"--session", conversation.ID}, args...)
			}
			_ = runSubmit(mode, withConfig(args), stdout, stderr)
		case "resume", "apply", "task", "tasks", "cancel", "trust", "watch", "diff", "recover":
			_ = runTaskCommand(head, withConfig(args), stdout, stderr)
		case "cleanup":
			_ = runCleanup(withConfig(args), stdout, stderr)
		case "verify":
			if len(args) == 0 {
				args = withConfig(args)
			}
			_ = runVerify(args, stdout, stderr)
		case "daemon":
			_ = runDaemonCmd(withConfig(args), stdout, stderr)
		case "model", "models", "config":
			_ = runModelCmdWithStdin(withConfig(args), stdin, stdout, stderr)
		case "auth", "login":
			_ = runAuthCmdWithStdin(withConfig(args), stdin, stdout, stderr)
		case "desktop", "ui":
			url, err := desktop.NewServer(configPath, version).StartAndOpen(ctx, "127.0.0.1:0", true)
			if err != nil {
				fmt.Fprintln(stderr, err)
			} else {
				fmt.Fprintln(stdout, "Desktop:", url)
			}
		default:
			if strings.HasPrefix(line, "/") {
				fmt.Fprintln(stderr, "unknown slash command:", fields[0])
				continue
			}
			request([]string{"--", line}, sess.ApprovalMode())
		}
		fmt.Fprintln(stdout)
	}
}
