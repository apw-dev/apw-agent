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
	"github.com/apiwat/apw-agent/internal/tui"
)

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
	rawInput := stdin == os.Stdin
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	if rawInput {
		tui.RenderDashboardConfig(stdout, cfg.Workspace, configPath, version, sess.SessionID)
	} else {
		printBanner(stdout, cfg.Workspace)
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
		case "verify", "symbols", "resume", "apply", "task", "tasks", "cancel", "trust", "watch", "diff", "recover", "cleanup", "daemon", "model", "models", "config", "auth", "login":
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
				fmt.Fprintln(stderr, "Usage: symbols <file.go>")
				break
			}
			path := args[0]
			if !filepath.IsAbs(path) {
				path = filepath.Join(cfg.Workspace, path)
			}
			_ = executeSymbols(path, stdout, stderr)
		case "plan":
			_ = runPlan(withConfig(args), stdout, stderr)
		case "run", "review", "preview":
			mode := core.ModeExecute
			if head == "review" {
				mode = core.ModeReview
			}
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
			args = []string{line}
			if conversation.ID != "" {
				args = append([]string{"--session", conversation.ID}, args...)
			}
			_ = runSubmit(core.ModeExecute, withConfig(args), stdout, stderr)
		}
		fmt.Fprintln(stdout)
	}
}
