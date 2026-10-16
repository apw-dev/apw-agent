package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/daemon"
)

// ANSI color & style constants
const (
	cReset     = "\x1b[0m"
	cBold      = "\x1b[1m"
	cDim       = "\x1b[2m"
	cCyan      = "\x1b[38;2;86;182;242m"
	cBlue      = "\x1b[38;2;100;149;237m"
	cGold      = "\x1b[38;2;229;169;60m"
	cAmber     = "\x1b[38;2;212;136;42m"
	cGreen     = "\x1b[38;2;74;222;128m"
	cPurple    = "\x1b[38;2;168;85;247m"
	cGray      = "\x1b[38;2;130;138;150m"
	cWhite     = "\x1b[38;2;235;240;245m"
	bgPill     = "\x1b[48;2;28;33;43m"
	bgSelected = "\x1b[48;2;30;45;68m"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// SlashCommand defines an interactive slash command in the APW TUI.
type SlashCommand struct {
	Name        string
	Usage       string
	Description string
	TakesArgs   bool
}

// DefaultCommands returns the ordered list of slash commands available in the APW CLI.
func DefaultCommands() []SlashCommand {
	return []SlashCommand{
		{Name: "/model", Usage: "/model [set <role> <provider>]", Description: "Configure Brain 1 (Reflex) & Brain 2 (Architect) models or add endpoints", TakesArgs: false},
		{Name: "/plan", Usage: "/plan <instruction>", Description: "Spec-First Planning: create ExecutionPlan (.apw/plans/) before touching code", TakesArgs: true},
		{Name: "/run", Usage: "/run <instruction>", Description: "Execute full Triad loop (Architect -> Surgeon -> Reflex) on a task", TakesArgs: true},
		{Name: "/verify", Usage: "/verify [dir]", Description: "Run Reflex closed-loop verification (gofmt -> go vet -> go build -> go test)", TakesArgs: false},
		{Name: "/symbols", Usage: "/symbols <file.go>", Description: "Inspect Go AST symbols (functions, structs, methods) using Surgeon", TakesArgs: true},
		{Name: "/skills", Usage: "/skills", Description: "List all loaded skills and Triad engines by category", TakesArgs: false},
		{Name: "/daemon", Usage: "/daemon [status|start|stop]", Description: "Manage 24/7 apwd background daemon process", TakesArgs: false},
		{Name: "/desktop", Usage: "/desktop", Description: "Launch Apiwat Agent Desktop App window (Claude Code Desktop style)", TakesArgs: false},
		{Name: "/new", Usage: "/new", Description: "Start a new session (fresh session ID + reset context)", TakesArgs: false},
		{Name: "/clear", Usage: "/clear", Description: "Clear screen and redraw the full APW dashboard", TakesArgs: false},
		{Name: "/status", Usage: "/status", Description: "Show system health, active Dual-Brain models, and daemon PID", TakesArgs: false},
		{Name: "/help", Usage: "/help", Description: "Show all slash commands and keyboard shortcuts", TakesArgs: false},
		{Name: "/quit", Usage: "/quit", Description: "Exit interactive CLI (apwd continues running in background)", TakesArgs: false},
	}
}

// FilterCommands returns slash commands matching the typed prefix (e.g. "/mo").
func FilterCommands(input string) []SlashCommand {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	// If the user already typed a space after the command name, hide autocomplete dropdown
	if strings.Contains(trimmed, " ") {
		return nil
	}
	query := strings.ToLower(trimmed)
	all := DefaultCommands()
	var matches []SlashCommand
	for _, cmd := range all {
		if strings.HasPrefix(cmd.Name, query) {
			matches = append(matches, cmd)
		}
	}
	for _, cmd := range all {
		if !strings.HasPrefix(cmd.Name, query) && strings.Contains(cmd.Name, strings.TrimPrefix(query, "/")) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// Session holds the interactive TUI state.
type Session struct {
	Workspace string
	Version   string
	SessionID string
	ModeIndex int // 0 = auto-triad, 1 = plan-first, 2 = fast-reflex
	History   []string
}

var modeNames = []string{
	"auto-triad (Architect -> Surgeon -> Reflex)",
	"plan-only (Spec-First Architect)",
	"fast-reflex (Brain 1 Verify & Repair)",
}

// NewSession creates a new TUI session.
func NewSession(workspace, version string) *Session {
	return &Session{
		Workspace: workspace,
		Version:   version,
		SessionID: fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff),
		ModeIndex: 0,
	}
}

// ResetSession generates a new session ID and clears history.
func (s *Session) ResetSession() {
	s.SessionID = fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	s.History = nil
}

// RenderDashboard draws the two-column framed APW header box with emblem, brains, daemon, and categorized skills.
func RenderDashboard(w io.Writer, workspace, version, sessionID string) {
	dState := daemon.EnsureRunning(workspace)
	cfg, _ := config.LoadOrDefault("./apw.json")
	stack := config.DetectStack(workspace)
	wsPath := formatWorkspace(workspace)

	reflexModel := "APW Engine"
	archModel := "APW Engine"
	if cfg != nil {
		if p, ok := cfg.Providers[cfg.Roles.Reflex]; ok && p.Model != "" {
			reflexModel = p.Model
		}
		if p, ok := cfg.Providers[cfg.Roles.Architect]; ok && p.Model != "" {
			archModel = p.Model
		}
	}

	leftColWidth := 38
	rightColWidth := 48
	totalInner := leftColWidth + 3 + rightColWidth

	leftLines := []string{
		"",
		fmt.Sprintf("   %s▄▀█%s %s█▀█%s %s█░█░█%s", "\x1b[38;2;255;140;66m", cReset, "\x1b[38;2;96;165;250m", cReset, "\x1b[38;2;52;211;153m", cReset),
		fmt.Sprintf("   %s█▀█%s %s█▀▀%s %s▀▄▀▄▀%s   %sv%s%s", "\x1b[38;2;251;191;36m", cReset, "\x1b[38;2;129;140;248m", cReset, "\x1b[38;2;45;212;191m", cReset, cBold+cCyan, version, cReset),
		"",
		fmt.Sprintf(" %sBrain 1 (Reflex)%s : %s%s%s", cGray, cReset, cGreen, truncatePlain(reflexModel, 18), cReset),
		fmt.Sprintf(" %sBrain 2 (Deep)%s   : %s%s%s", cGray, cReset, cCyan, truncatePlain(archModel, 18), cReset),
		fmt.Sprintf(" %sDaemon (apwd)%s    : %sPID %d · %dms%s", cGray, cReset, cGold, dState.PID, dState.LatencyMs, cReset),
		fmt.Sprintf(" %sWorkspace%s        : %s%s (%s)%s", cGray, cReset, cWhite, truncatePlain(wsPath, 15), stack, cReset),
		fmt.Sprintf(" %sSession%s          : %s%s%s", cGray, cReset, cGray, sessionID, cReset),
	}

	skillGroups := daemon.DiscoverSkills(workspace)
	catOrder := []string{"triad-core", "runtime", "extensions", "system", "workspace"}
	var rightLines []string
	rightLines = append(rightLines, fmt.Sprintf("%s%sTriad Engines & Active Modules%s", cBold, cGold, cReset))
	rightLines = append(rightLines, fmt.Sprintf("%sarchitect%s · %ssurgeon%s · %sreflex%s · %ssqlite%s", cCyan, cGray, cGreen, cGray, cGold, cGray, cPurple, cReset))
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, fmt.Sprintf("%s%sAvailable Skills%s", cBold, cGold, cReset))

	totalSkills := 0
	for _, cat := range catOrder {
		items := skillGroups[cat]
		totalSkills += len(items)
		if len(items) == 0 {
			continue
		}
		display := formatSkillList(items, 2)
		rightLines = append(rightLines, fmt.Sprintf("%s%s:%s %s%s%s", cAmber, cat, cReset, cWhite, truncatePlain(display, rightColWidth-len(cat)-3), cReset))
	}
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, fmt.Sprintf("%s4 engines · %d skills · /help or /model%s", cGray, totalSkills, cReset))

	maxRows := len(leftLines)
	if len(rightLines) > maxRows {
		maxRows = len(rightLines)
	}

	topBorder := fmt.Sprintf("%s╭%s╮%s", cAmber, strings.Repeat("─", totalInner), cReset)
	botBorder := fmt.Sprintf("%s╰%s╯%s", cAmber, strings.Repeat("─", totalInner), cReset)

	fmt.Fprintln(w, topBorder)
	for i := 0; i < maxRows; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lPadded := padRightANSI(l, leftColWidth)
		rPadded := padRightANSI(r, rightColWidth)
		fmt.Fprintf(w, "%s│%s%s %s│%s %s%s│%s\n", cAmber, cReset, lPadded, cDim+cAmber, cReset, rPadded, cAmber, cReset)
	}
	fmt.Fprintln(w, botBorder)
	fmt.Fprintf(w, "\n%sWelcome to Apiwat Agent!%s Type your instruction or %s/%s for commands (%s/model%s to switch models).\n",
		cBold+cWhite, cReset, cCyan, cReset, cCyan, cReset)
}

func formatSkillList(items []string, maxShow int) string {
	if len(items) <= maxShow {
		return strings.Join(items, ", ")
	}
	shown := strings.Join(items[:maxShow], ", ")
	return fmt.Sprintf("%s, +%d more", shown, len(items)-maxShow)
}

// ReadInteractiveLine reads a command line with live `/` dropdown autocomplete, arrow navigation, and mode toggling.
// If the terminal does not support raw mode, it returns an error so the caller can fall back to line scanner.
func (s *Session) ReadInteractiveLine(stdout io.Writer) (string, error) {
	raw, err := enableRawTerminal()
	if err != nil {
		return "", err
	}
	defer raw.restore()

	width := terminalWidth()
	if width > 92 {
		width = 92
	}

	cfg, _ := config.LoadOrDefault("./apw.json")
	dState := daemon.Status(s.Workspace)
	reflexModel := "APW Engine"
	archModel := "APW Engine"
	if cfg != nil {
		if p, ok := cfg.Providers[cfg.Roles.Reflex]; ok && p.Model != "" {
			reflexModel = p.Model
		}
		if p, ok := cfg.Providers[cfg.Roles.Architect]; ok && p.Model != "" {
			archModel = p.Model
		}
	}

	hr := fmt.Sprintf("%s%s%s", cDim+cGray, strings.Repeat("─", width), cReset)
	pill := fmt.Sprintf(" %s ⚡ %s %s│%s 🧠 %s %s│%s ctx [░░░░░░░░░░] %s│%s ⏱ %dms %s",
		bgPill+cGold, reflexModel, cGray, cCyan, archModel, cGray, cWhite, cGray, cGreen, dState.LatencyMs, cReset)

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, pill)
	fmt.Fprintln(stdout, hr)

	var buf []rune
	selectedIdx := 0
	histIdx := len(s.History)

	redraw := func(final bool) {
		inputStr := string(buf)
		// Clear from start of current input line to bottom of screen
		fmt.Fprint(stdout, "\r\x1b[J")

		if final {
			fmt.Fprintf(stdout, "%s>%s %s\r\n", cBold+cCyan, cReset, inputStr)
			fmt.Fprintf(stdout, "%s\r\n", hr)
			return
		}

		// Line 0: Input line
		fmt.Fprintf(stdout, "%s>%s %s\r\n", cBold+cCyan, cReset, inputStr)
		// Line 1: Bottom horizontal rule
		fmt.Fprintf(stdout, "%s\r\n", hr)
		linesBelow := 2

		matches := FilterCommands(inputStr)
		if len(matches) > 0 {
			if selectedIdx >= len(matches) {
				selectedIdx = 0
			}
			if selectedIdx < 0 {
				selectedIdx = len(matches) - 1
			}

			maxShow := 7
			startIdx := 0
			if selectedIdx >= maxShow {
				startIdx = selectedIdx - maxShow + 1
			}
			endIdx := startIdx + maxShow
			if endIdx > len(matches) {
				endIdx = len(matches)
			}

			for i := startIdx; i < endIdx; i++ {
				m := matches[i]
				if i == selectedIdx {
					line := fmt.Sprintf("%s%s> %-22s%s %s%s%s", bgSelected+cBold+cCyan, "", m.Name, cReset+bgSelected, cWhite, truncatePlain(m.Description, width-26), cReset)
					fmt.Fprintf(stdout, "%s\r\n", line)
				} else {
					fmt.Fprintf(stdout, "  %s%-22s%s %s%s%s\r\n", cWhite, m.Name, cReset, cGray, truncatePlain(m.Description, width-26), cReset)
				}
				linesBelow++
			}
			if len(matches) > maxShow {
				fmt.Fprintf(stdout, "   %s↓ %d more commands%s\r\n", cDim+cGray, len(matches)-maxShow, cReset)
				linesBelow++
			}
			fmt.Fprintf(stdout, "\r\n")
			linesBelow++

			leftHint := fmt.Sprintf(" %s↑/↓%s Navigate · %senter%s Select · %stab%s Complete · %sesc%s Cancel",
				cBold+cCyan, cGray, cBold+cCyan, cGray, cBold+cCyan, cGray, cBold+cCyan, cGray)
			rightHint := fmt.Sprintf("%saccept-edits%s · %s%s%s", cGreen, cGray, cWhite, archModel, cReset)
			fmt.Fprint(stdout, leftHint+"    "+rightHint)
		} else {
			modeLabel := modeNames[s.ModeIndex%len(modeNames)]
			fmt.Fprintf(stdout, " %s▸▸ %s%s %s(shift+tab to cycle · / for commands)%s",
				cGold, modeLabel, cReset, cGray, cReset)
		}

		// Move cursor back up to Line 0 and position after `> <input>`
		fmt.Fprintf(stdout, "\x1b[%dA\r", linesBelow)
		colOffset := 2 + utf8.RuneCountInString(inputStr)
		if colOffset > 0 {
			fmt.Fprintf(stdout, "\x1b[%dC", colOffset)
		}
	}

	redraw(false)

	var inBuf [32]byte
	for {
		n, rErr := os.Stdin.Read(inBuf[:])
		if rErr != nil || n == 0 {
			return "", io.EOF
		}
		b := inBuf[:n]

		// Handle ANSI escape sequences (arrows, Shift+Tab, Esc)
		if b[0] == 0x1b {
			if n == 1 {
				// Bare Esc key: clear current input
				buf = nil
				selectedIdx = 0
				redraw(false)
				continue
			}
			if n >= 3 && b[1] == '[' {
				switch b[2] {
				case 'A': // Up arrow
					matches := FilterCommands(string(buf))
					if len(matches) > 0 {
						selectedIdx--
						if selectedIdx < 0 {
							selectedIdx = len(matches) - 1
						}
					} else if len(s.History) > 0 && histIdx > 0 {
						histIdx--
						buf = []rune(s.History[histIdx])
					}
					redraw(false)
				case 'B': // Down arrow
					matches := FilterCommands(string(buf))
					if len(matches) > 0 {
						selectedIdx++
						if selectedIdx >= len(matches) {
							selectedIdx = 0
						}
					} else if histIdx < len(s.History)-1 {
						histIdx++
						buf = []rune(s.History[histIdx])
					} else {
						histIdx = len(s.History)
						buf = nil
					}
					redraw(false)
				case 'Z': // Shift+Tab -> cycle execution mode
					s.ModeIndex = (s.ModeIndex + 1) % len(modeNames)
					redraw(false)
				}
			}
			continue
		}

		// Ctrl+C or Ctrl+D
		if b[0] == 3 || b[0] == 4 {
			fmt.Fprint(stdout, "\r\x1b[J\r\n")
			return "/quit", nil
		}

		// Ctrl+L -> clear screen
		if b[0] == 12 {
			redraw(true)
			return "/clear", nil
		}

		// Tab -> autocomplete selected slash command
		if b[0] == '\t' {
			matches := FilterCommands(string(buf))
			if len(matches) > 0 {
				if selectedIdx >= len(matches) {
					selectedIdx = 0
				}
				chosen := matches[selectedIdx]
				if chosen.TakesArgs {
					buf = []rune(chosen.Name + " ")
				} else {
					buf = []rune(chosen.Name)
				}
				selectedIdx = 0
				redraw(false)
			}
			continue
		}

		// Enter (\r or \n)
		if b[0] == '\r' || b[0] == '\n' {
			matches := FilterCommands(string(buf))
			if len(matches) > 0 {
				if selectedIdx >= len(matches) {
					selectedIdx = 0
				}
				chosen := matches[selectedIdx]
				// If user typed partial slash command (e.g. "/mo") and pressed Enter:
				if string(buf) != chosen.Name && chosen.TakesArgs {
					buf = []rune(chosen.Name + " ")
					selectedIdx = 0
					redraw(false)
					continue
				}
				buf = []rune(chosen.Name)
			}
			finalLine := strings.TrimSpace(string(buf))
			redraw(true)
			if finalLine != "" {
				s.History = append(s.History, finalLine)
			}
			// If mode is plan-only or fast-reflex and user typed natural language:
			if finalLine != "" && !strings.HasPrefix(finalLine, "/") {
				switch s.ModeIndex % len(modeNames) {
				case 1:
					finalLine = "/plan " + finalLine
				case 2:
					finalLine = "/verify"
				}
			}
			return finalLine, nil
		}

		// Backspace (0x7f or 0x08)
		if b[0] == 0x7f || b[0] == 0x08 {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				selectedIdx = 0
				redraw(false)
			}
			continue
		}

		// Printable UTF-8 characters
		text := string(b)
		for _, r := range text {
			if r >= 32 {
				buf = append(buf, r)
			}
		}
		selectedIdx = 0
		redraw(false)
	}
}

// RunInteractiveModelSelector opens an arrow-key interactive menu to switch Brain 1 / Brain 2 models or add a custom provider.
func RunInteractiveModelSelector(cfgPath string, stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := enableRawTerminal()
	if err != nil {
		// Non-TTY fallback: return -1 so caller prints standard model status
		return -1
	}

	cfg, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		raw.restore()
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	type menuOption struct {
		Label       string
		Description string
		Apply       func(*config.Config) string
		IsCustom    bool
	}

	// Build options from existing providers + presets
	var provNames []string
	for k := range cfg.Providers {
		provNames = append(provNames, k)
	}
	sort.Strings(provNames)

	options := []menuOption{
		{
			Label:       "Preset: Hybrid Pro (Recommended)",
			Description: "Brain 2 (Architect) = claude-sonnet (API) · Brain 1 (Reflex) = ollama-local (Fast Local)",
			Apply: func(c *config.Config) string {
				if _, ok := c.Providers["claude-sonnet"]; ok {
					c.Roles.Architect = "claude-sonnet"
				}
				if _, ok := c.Providers["ollama-local"]; ok {
					c.Roles.Reflex = "ollama-local"
				}
				return "Switched to Hybrid Pro (Architect: claude-sonnet, Reflex: ollama-local)"
			},
		},
		{
			Label:       "Preset: 100% Local (Zero API Key)",
			Description: "Brain 1 & Brain 2 both use ollama-local (qwen2.5-coder:7b @ 127.0.0.1:11434)",
			Apply: func(c *config.Config) string {
				if _, ok := c.Providers["ollama-local"]; ok {
					c.Roles.Architect = "ollama-local"
					c.Roles.Reflex = "ollama-local"
				}
				return "Switched both brains to ollama-local (qwen2.5-coder:7b)"
			},
		},
		{
			Label:       "Preset: Cloud API (Anthropic)",
			Description: "Brain 2 = claude-sonnet-4 · Brain 1 = claude-3-5-haiku (uses ANTHROPIC_API_KEY)",
			Apply: func(c *config.Config) string {
				if _, ok := c.Providers["claude-sonnet"]; ok {
					c.Roles.Architect = "claude-sonnet"
				}
				if _, ok := c.Providers["claude-haiku"]; ok {
					c.Roles.Reflex = "claude-haiku"
				}
				return "Switched to Cloud API (claude-sonnet + claude-haiku)"
			},
		},
		{
			Label:       "Preset: Built-in Offline Engine",
			Description: "Brain 1 & Brain 2 use apw-builtin (APW Engine)",
			Apply: func(c *config.Config) string {
				if _, ok := c.Providers["apw-builtin"]; !ok {
					c.Providers["apw-builtin"] = config.ProviderConfig{
						Type:       config.ProviderMock,
						Model:      "APW Engine",
						TimeoutSec: 30,
					}
				}
				c.Roles.Architect = "apw-builtin"
				c.Roles.Reflex = "apw-builtin"
				return "Switched both brains to apw-builtin (APW Engine)"
			},
		},
	}

	for _, name := range provNames {
		pName := name
		pCfg := cfg.Providers[pName]
		options = append(options, menuOption{
			Label:       fmt.Sprintf("Set Brain 2 (Architect) -> %s", pName),
			Description: fmt.Sprintf("model=%s (%s)", pCfg.Model, pCfg.Type),
			Apply: func(c *config.Config) string {
				c.Roles.Architect = pName
				return fmt.Sprintf("Set Brain 2 (Architect) -> %s (%s)", pName, pCfg.Model)
			},
		})
		options = append(options, menuOption{
			Label:       fmt.Sprintf("Set Brain 1 (Reflex)    -> %s", pName),
			Description: fmt.Sprintf("model=%s (%s)", pCfg.Model, pCfg.Type),
			Apply: func(c *config.Config) string {
				c.Roles.Reflex = pName
				return fmt.Sprintf("Set Brain 1 (Reflex) -> %s (%s)", pName, pCfg.Model)
			},
		})
	}

	options = append(options, menuOption{
		Label:       "+ Add / Configure Custom Model Provider...",
		Description: "Add a new Ollama, LM Studio, vLLM, or Cloud API model into apw.json",
		IsCustom:    true,
	})

	selectedIdx := 0
	renderMenu := func() {
		fmt.Fprint(stdout, "\r\x1b[J")
		archP := cfg.Providers[cfg.Roles.Architect]
		refP := cfg.Providers[cfg.Roles.Reflex]
		fmt.Fprintf(stdout, "%s%sSelect Dual-Brain Model Configuration%s\r\n", cBold, cCyan, cReset)
		fmt.Fprintf(stdout, "  Current Brain 1 (Reflex)    : %s%s%s (%s)\r\n", cGreen, cfg.Roles.Reflex, cReset, refP.Model)
		fmt.Fprintf(stdout, "  Current Brain 2 (Architect) : %s%s%s (%s)\r\n\r\n", cCyan, cfg.Roles.Architect, cReset, archP.Model)

		linesPrinted := 4
		for i, opt := range options {
			if i == selectedIdx {
				fmt.Fprintf(stdout, "%s> %-42s%s %s%s%s\r\n", bgSelected+cBold+cCyan, opt.Label, cReset+bgSelected, cWhite, opt.Description, cReset)
			} else {
				fmt.Fprintf(stdout, "  %s%-42s%s %s%s%s\r\n", cWhite, opt.Label, cReset, cGray, opt.Description, cReset)
			}
			linesPrinted++
		}
		fmt.Fprintf(stdout, "\r\n %s↑/↓%s Navigate · %senter%s Apply & Save to apw.json · %sesc%s Cancel",
			cBold+cCyan, cGray, cBold+cCyan, cGray, cBold+cCyan, cReset)
		linesPrinted++
		fmt.Fprintf(stdout, "\x1b[%dA\r", linesPrinted)
	}

	renderMenu()

	var inBuf [16]byte
	for {
		n, rErr := os.Stdin.Read(inBuf[:])
		if rErr != nil || n == 0 {
			raw.restore()
			return 0
		}
		b := inBuf[:n]

		if b[0] == 0x1b {
			if n == 1 {
				fmt.Fprint(stdout, "\r\x1b[J")
				raw.restore()
				fmt.Fprintln(stdout, "Model selection cancelled.")
				return 0
			}
			if n >= 3 && b[1] == '[' {
				switch b[2] {
				case 'A':
					selectedIdx--
					if selectedIdx < 0 {
						selectedIdx = len(options) - 1
					}
					renderMenu()
				case 'B':
					selectedIdx++
					if selectedIdx >= len(options) {
						selectedIdx = 0
					}
					renderMenu()
				}
			}
			continue
		}

		if b[0] == 3 || b[0] == 4 || b[0] == 'q' {
			fmt.Fprint(stdout, "\r\x1b[J")
			raw.restore()
			return 0
		}

		if b[0] == '\r' || b[0] == '\n' {
			fmt.Fprint(stdout, "\r\x1b[J")
			raw.restore()

			chosen := options[selectedIdx]
			if chosen.IsCustom {
				return promptCustomProvider(cfg, cfgPath, stdin, stdout, stderr)
			}

			msg := chosen.Apply(cfg)
			if err := cfg.Save(cfgPath); err != nil {
				fmt.Fprintf(stderr, "Error saving %s: %v\n", cfgPath, err)
				return 1
			}
			fmt.Fprintf(stdout, "%s✓ %s%s (saved to %s)\n", cGreen, msg, cReset, cfgPath)
			return 0
		}
	}
}

func promptCustomProvider(cfg *config.Config, cfgPath string, stdin io.Reader, stdout, stderr io.Writer) int {
	reader := bufio.NewReader(stdin)
	ask := func(label, defVal string) string {
		if defVal != "" {
			fmt.Fprintf(stdout, "  %s [%s]: ", label, defVal)
		} else {
			fmt.Fprintf(stdout, "  %s: ", label)
		}
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defVal
		}
		return line
	}

	fmt.Fprintf(stdout, "%sConfigure Custom Model Provider%s\n", cBold+cCyan, cReset)
	name := ask("Provider alias (e.g. my-local, deepseek-api)", "custom-model")
	pType := strings.ToLower(ask("Provider type (local / api)", "local"))
	var pc config.ProviderConfig
	if pType == "api" {
		pc.Type = config.ProviderAPI
		pc.Endpoint = ask("API Endpoint URL", "https://api.anthropic.com")
		pc.Model = ask("Model ID", "claude-sonnet-4-20250514")
		pc.APIKeyEnv = ask("Environment Variable Name for API Key", "ANTHROPIC_API_KEY")
	} else {
		pc.Type = config.ProviderLocal
		pc.Endpoint = ask("Local Server URL (Ollama / vLLM / LM Studio)", "http://127.0.0.1:11434")
		pc.Model = ask("Model ID", "qwen2.5-coder:7b")
	}
	pc.TimeoutSec = 120

	cfg.Providers[name] = pc
	roleChoice := strings.ToLower(ask("Assign to which Brain? (architect / reflex / both)", "both"))
	switch roleChoice {
	case "architect", "brain2":
		cfg.Roles.Architect = name
	case "reflex", "brain1":
		cfg.Roles.Reflex = name
	default:
		cfg.Roles.Architect = name
		cfg.Roles.Reflex = name
	}

	if err := cfg.Save(cfgPath); err != nil {
		fmt.Fprintf(stderr, "Error saving config: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s✓ Added provider %q (%s) and saved to %s%s\n", cGreen, name, pc.Model, cfgPath, cReset)
	return 0
}

func visibleLen(s string) int {
	plain := ansiRegex.ReplaceAllString(s, "")
	return utf8.RuneCountInString(plain)
}

func padRightANSI(s string, width int) string {
	vLen := visibleLen(s)
	if vLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vLen)
}

func truncatePlain(s string, maxRunes int) string {
	if maxRunes <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-3]) + "..."
}

func formatWorkspace(workspace string) string {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return workspace
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(strings.ToLower(abs), strings.ToLower(home)) {
		rel := abs[len(home):]
		if rel == "" {
			return "~"
		}
		return "~" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}
