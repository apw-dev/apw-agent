package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/auth"
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
		{Name: "/model", Usage: "/model", Description: "Select live models for Brain 1 (Reflex) & Brain 2 (Architect) from connected accounts", TakesArgs: false},
		{Name: "/auth", Usage: "/auth", Description: "Connect & authenticate ChatGPT Sub/API, Claude Sub/API, and AGY CLI", TakesArgs: false},
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

// LoadSyncedConfig loads apw.json (or default) and merges live authenticated models.
func LoadSyncedConfig(workspace, cfgPath string, forceRefresh bool) (*config.Config, []auth.ConnectionStatus) {
	cfg, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		cfg, _ = config.LoadOrDefault("")
	}
	statuses := auth.SyncConfigWithDiscovered(workspace, cfg, forceRefresh)
	return cfg, statuses
}

// RenderDashboard draws the two-column framed APW header box with emblem, brains, daemon, and categorized skills.
func RenderDashboard(w io.Writer, workspace, version, sessionID string) {
	dState := daemon.EnsureRunning(workspace)
	cfg, statuses := LoadSyncedConfig(workspace, "./apw.json", false)
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

	var connectedSources []string
	for _, st := range statuses {
		if st.Connected {
			connectedSources = append(connectedSources, st.Source)
		}
	}
	authSummary := "Built-in (/auth to connect)"
	if len(connectedSources) > 0 {
		authSummary = strings.Join(connectedSources, " · ")
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
		fmt.Sprintf(" %sConnected Auth%s   : %s%s%s", cGray, cReset, cGold, truncatePlain(authSummary, 18), cReset),
		fmt.Sprintf(" %sDaemon (apwd)%s    : %sPID %d · %dms%s", cGray, cReset, cWhite, dState.PID, dState.LatencyMs, cReset),
		fmt.Sprintf(" %sWorkspace%s        : %s%s (%s)%s", cGray, cReset, cWhite, truncatePlain(wsPath, 14), stack, cReset),
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
	rightLines = append(rightLines, fmt.Sprintf("%s4 engines · %d skills · /model · /auth%s", cGray, totalSkills, cReset))

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
	fmt.Fprintf(w, "\n%sWelcome to Apiwat Agent!%s Type your instruction or %s/%s for commands (%s/model%s · %s/auth%s).\n",
		cBold+cWhite, cReset, cCyan, cReset, cCyan, cReset, cCyan, cReset)
}

func formatSkillList(items []string, maxShow int) string {
	if len(items) <= maxShow {
		return strings.Join(items, ", ")
	}
	shown := strings.Join(items[:maxShow], ", ")
	return fmt.Sprintf("%s, +%d more", shown, len(items)-maxShow)
}

// ReadInteractiveLine reads a command line with live `/` dropdown autocomplete, arrow navigation, and mode toggling.
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

	cfg, _ := LoadSyncedConfig(s.Workspace, "./apw.json", false)
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
		bgPill+cGold, truncatePlain(reflexModel, 22), cGray, cCyan, truncatePlain(archModel, 22), cGray, cWhite, cGray, cGreen, dState.LatencyMs, cReset)

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, pill)
	fmt.Fprintln(stdout, hr)

	var buf []rune
	selectedIdx := 0
	histIdx := len(s.History)

	redraw := func(final bool) {
		inputStr := string(buf)
		fmt.Fprint(stdout, "\r\x1b[J")

		if final {
			fmt.Fprintf(stdout, "%s>%s %s\r\n", cBold+cCyan, cReset, inputStr)
			fmt.Fprintf(stdout, "%s\r\n", hr)
			return
		}

		fmt.Fprintf(stdout, "%s>%s %s\r\n", cBold+cCyan, cReset, inputStr)
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

			descMax := width - 25
			if descMax < 20 {
				descMax = 20
			}
			for i := startIdx; i < endIdx; i++ {
				m := matches[i]
				if i == selectedIdx {
					line := fmt.Sprintf("%s> %-20s%s %s%s%s", bgSelected+cBold+cCyan, m.Name, cReset+bgSelected, cWhite, truncatePlain(m.Description, descMax), cReset)
					fmt.Fprintf(stdout, "%s\r\n", line)
				} else {
					fmt.Fprintf(stdout, "  %s%-20s%s %s%s%s\r\n", cWhite, m.Name, cReset, cGray, truncatePlain(m.Description, descMax), cReset)
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
			rightHint := fmt.Sprintf("%saccept-edits%s · %s%s%s", cGreen, cGray, cWhite, truncatePlain(archModel, 18), cReset)
			fmt.Fprint(stdout, leftHint+"   "+rightHint)
		} else {
			modeLabel := modeNames[s.ModeIndex%len(modeNames)]
			fmt.Fprintf(stdout, " %s▸▸ %s%s %s(shift+tab to cycle · /model · /auth)%s",
				cGold, modeLabel, cReset, cGray, cReset)
		}

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

		if b[0] == 0x1b {
			if n == 1 {
				buf = nil
				selectedIdx = 0
				redraw(false)
				continue
			}
			if n >= 3 && b[1] == '[' {
				switch b[2] {
				case 'A':
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
				case 'B':
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
				case 'Z':
					s.ModeIndex = (s.ModeIndex + 1) % len(modeNames)
					redraw(false)
				}
			}
			continue
		}

		if b[0] == 3 || b[0] == 4 {
			fmt.Fprint(stdout, "\r\x1b[J\r\n")
			return "/quit", nil
		}

		if b[0] == 12 {
			redraw(true)
			return "/clear", nil
		}

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

		if b[0] == '\r' || b[0] == '\n' {
			matches := FilterCommands(string(buf))
			if len(matches) > 0 {
				if selectedIdx >= len(matches) {
					selectedIdx = 0
				}
				chosen := matches[selectedIdx]
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

		if b[0] == 0x7f || b[0] == 0x08 {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				selectedIdx = 0
				redraw(false)
			}
			continue
		}

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

type selectorItem struct {
	Label  string
	Detail string
	Action func() (bool, error) // returns (done, err)
}

func runSingleLineSelector(title string, subtitleLines []string, items []selectorItem, stdout io.Writer) int {
	raw, err := enableRawTerminal()
	if err != nil {
		return -1
	}

	termW := terminalWidth()
	if termW < 68 {
		termW = 68
	}
	labelW := 36
	detailW := termW - labelW - 6
	if detailW < 18 {
		detailW = 18
	}

	selectedIdx := 0
	maxVisible := 9

	render := func() {
		fmt.Fprint(stdout, "\r\x1b[J")
		fmt.Fprintf(stdout, "%s%s%s\r\n", cBold+cCyan, truncatePlain(title, termW-2), cReset)
		linesPrinted := 1
		for _, sub := range subtitleLines {
			fmt.Fprintf(stdout, "%s\r\n", sub)
			linesPrinted++
		}
		fmt.Fprintf(stdout, "\r\n")
		linesPrinted++

		startIdx := 0
		if selectedIdx >= maxVisible {
			startIdx = selectedIdx - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(items) {
			endIdx = len(items)
		}

		for i := startIdx; i < endIdx; i++ {
			it := items[i]
			lbl := truncatePlain(it.Label, labelW)
			det := truncatePlain(it.Detail, detailW)
			if i == selectedIdx {
				fmt.Fprintf(stdout, "%s> %-*s%s %s%s%s\r\n", bgSelected+cBold+cCyan, labelW, lbl, cReset+bgSelected, cWhite, det, cReset)
			} else {
				fmt.Fprintf(stdout, "  %s%-*s%s %s%s%s\r\n", cWhite, labelW, lbl, cReset, cGray, det, cReset)
			}
			linesPrinted++
		}
		if len(items) > maxVisible {
			fmt.Fprintf(stdout, "  %s(%d/%d — use ↑/↓ to scroll)%s\r\n", cDim+cGray, selectedIdx+1, len(items), cReset)
			linesPrinted++
		}
		fmt.Fprintf(stdout, "\r\n %s↑/↓%s Navigate · %senter%s Select · %sesc%s Back/Cancel",
			cBold+cCyan, cGray, cBold+cCyan, cGray, cBold+cCyan, cReset)
		linesPrinted++
		fmt.Fprintf(stdout, "\x1b[%dA\r", linesPrinted)
	}

	render()

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
				return 0
			}
			if n >= 3 && b[1] == '[' {
				switch b[2] {
				case 'A':
					selectedIdx--
					if selectedIdx < 0 {
						selectedIdx = len(items) - 1
					}
					render()
				case 'B':
					selectedIdx++
					if selectedIdx >= len(items) {
						selectedIdx = 0
					}
					render()
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
			done, actErr := items[selectedIdx].Action()
			if actErr != nil {
				fmt.Fprintf(stdout, "Error: %v\n", actErr)
				return 1
			}
			if done {
				return 0
			}
			// Re-enter raw mode if Action opened a nested menu and returned done=false
			raw2, err2 := enableRawTerminal()
			if err2 != nil {
				return 0
			}
			raw = raw2
			render()
		}
	}
}

// RunInteractiveModelSelector opens the live model picker fetched from AGY CLI, Claude Sub/API, ChatGPT Sub/OpenAI API, and Ollama.
func RunInteractiveModelSelector(cfgPath string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, statuses := LoadSyncedConfig(".", cfgPath, true)
	_ = cfg.Save(cfgPath)

	var allLiveModels []auth.DiscoveredModel
	for _, st := range statuses {
		allLiveModels = append(allLiveModels, st.Models...)
	}

	pickModelForRole := func(roleName, roleLabel string) (bool, error) {
		var modelItems []selectorItem
		for _, m := range allLiveModels {
			mod := m
			modelItems = append(modelItems, selectorItem{
				Label:  fmt.Sprintf("[%s] %s", mod.Source, mod.ID),
				Detail: mod.DisplayName,
				Action: func() (bool, error) {
					cfg.Providers[mod.ProviderKey] = config.ProviderConfig{
						Type:       mod.ProviderType,
						Endpoint:   mod.Endpoint,
						Model:      mod.ID,
						APIKeyEnv:  mod.APIKeyEnv,
						TimeoutSec: 120,
					}
					if roleName == "reflex" {
						cfg.Roles.Reflex = mod.ProviderKey
					} else if roleName == "architect" {
						cfg.Roles.Architect = mod.ProviderKey
					} else {
						cfg.Roles.Reflex = mod.ProviderKey
						cfg.Roles.Architect = mod.ProviderKey
					}
					if err := cfg.Save(cfgPath); err != nil {
						return true, err
					}
					fmt.Fprintf(stdout, "%s✓ Set %s -> %s (%s via %s)%s\n", cGreen, roleLabel, mod.DisplayName, mod.ID, mod.Source, cReset)
					return true, nil
				},
			})
		}
		if len(modelItems) == 0 {
			fmt.Fprintln(stdout, "No live models discovered yet. Opening /auth to connect an account...")
			_ = RunInteractiveAuthMenu(cfgPath, stdin, stdout, stderr)
			return true, nil
		}
		sub := []string{
			fmt.Sprintf("  Found %s%d live authenticated models%s from connected providers", cGreen, len(modelItems), cReset),
		}
		runSingleLineSelector("Select Model for "+roleLabel, sub, modelItems, stdout)
		return true, nil
	}

	refProv := cfg.Providers[cfg.Roles.Reflex]
	archProv := cfg.Providers[cfg.Roles.Architect]

	var connSummary []string
	for _, st := range statuses {
		if st.Connected {
			connSummary = append(connSummary, fmt.Sprintf("%s✓ %s (%d)%s", cGreen, st.Source, len(st.Models), cReset))
		} else {
			connSummary = append(connSummary, fmt.Sprintf("%s○ %s%s", cGray, st.Source, cReset))
		}
	}

	subtitles := []string{
		fmt.Sprintf("  Brain 1 (Reflex)    : %s%s%s (%s)", cGreen, refProv.Model, cReset, cfg.Roles.Reflex),
		fmt.Sprintf("  Brain 2 (Architect) : %s%s%s (%s)", cCyan, archProv.Model, cReset, cfg.Roles.Architect),
		fmt.Sprintf("  Connected Sources   : %s", strings.Join(connSummary, " · ")),
	}

	mainItems := []selectorItem{
		{
			Label:  "⚡ Set Brain 1 (Reflex) Model...",
			Detail: fmt.Sprintf("Choose from %d live authenticated models", len(allLiveModels)),
			Action: func() (bool, error) {
				return pickModelForRole("reflex", "Brain 1 (Reflex)")
			},
		},
		{
			Label:  "🧠 Set Brain 2 (Architect) Model...",
			Detail: fmt.Sprintf("Choose from %d live authenticated models", len(allLiveModels)),
			Action: func() (bool, error) {
				return pickModelForRole("architect", "Brain 2 (Architect)")
			},
		},
		{
			Label:  "🔗 Set Both Brains to Same Model...",
			Detail: "Assign one live model to both Brain 1 and Brain 2",
			Action: func() (bool, error) {
				return pickModelForRole("both", "Brain 1 & Brain 2")
			},
		},
		{
			Label:  "🔄 Refresh Live Models Now",
			Detail: "Re-fetch models from AGY CLI, Claude Sub/API, ChatGPT/OpenAI, Ollama",
			Action: func() (bool, error) {
				fmt.Fprintln(stdout, "Fetching live models from connected accounts...")
				cfg, statuses = LoadSyncedConfig(".", cfgPath, true)
				_ = cfg.Save(cfgPath)
				total := 0
				for _, st := range statuses {
					if st.Connected {
						fmt.Fprintf(stdout, "  %s✓ %-28s%s : %s\n", cGreen, st.Name, cReset, st.Detail)
						total += len(st.Models)
					} else {
						fmt.Fprintf(stdout, "  %s○ %-28s%s : %s\n", cGray, st.Name, cReset, st.Detail)
					}
				}
				fmt.Fprintf(stdout, "%s✓ Synced %d live models into %s%s\n", cGreen, total, cfgPath, cReset)
				return true, nil
			},
		},
		{
			Label:  "🔐 Authenticate / Connect Accounts...",
			Detail: "Connect ChatGPT Sub/API, Claude Sub/API, or AGY CLI (/auth)",
			Action: func() (bool, error) {
				_ = RunInteractiveAuthMenu(cfgPath, stdin, stdout, stderr)
				return true, nil
			},
		},
	}

	return runSingleLineSelector("Dual-Brain Live Model Selector (/model)", subtitles, mainItems, stdout)
}

// RunInteractiveAuthMenu opens the authentication & account connection manager (/auth).
func RunInteractiveAuthMenu(cfgPath string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, statuses := LoadSyncedConfig(".", cfgPath, true)

	var subtitles []string
	for _, st := range statuses {
		if st.Connected {
			subtitles = append(subtitles, fmt.Sprintf("  %s✓ %-32s%s : %s", cGreen, st.Name, cReset, st.Detail))
		} else {
			subtitles = append(subtitles, fmt.Sprintf("  %s○ %-32s%s : %s", cGray, st.Name, cReset, st.Detail))
		}
	}

	items := []selectorItem{
		{
			Label:  "1. AGY CLI Connect (Google AI Pro)",
			Detail: "Fetch live Gemini 4 / 3.8 & Claude 5.5 models from agy CLI",
			Action: func() (bool, error) {
				fmt.Fprintln(stdout, "Checking AGY CLI connection and fetching live models (`agy models`)...")
				cfg, statuses = LoadSyncedConfig(".", cfgPath, true)
				_ = cfg.Save(cfgPath)
				for _, st := range statuses {
					if st.Source == "AGY CLI" {
						if st.Connected {
							fmt.Fprintf(stdout, "%s✓ Connected to AGY CLI: %s%s\n", cGreen, st.Detail, cReset)
							for _, m := range st.Models {
								fmt.Fprintf(stdout, "    - %-26s (%s)\n", m.ID, m.DisplayName)
							}
						} else {
							fmt.Fprintf(stdout, "%s! %s%s\n", cGold, st.Detail, cReset)
						}
					}
				}
				return true, nil
			},
		},
		{
			Label:  "2. Claude Subscription (Claude CLI)",
			Detail: "Authenticate via Claude Code OAuth Subscription (`claude auth login`)",
			Action: func() (bool, error) {
				claudePath, err := exec.LookPath("claude")
				if err != nil {
					fmt.Fprintln(stdout, "Claude CLI is not installed in PATH. Use Option 3 to connect via ANTHROPIC_API_KEY.")
					return true, nil
				}
				fmt.Fprintln(stdout, "Launching Claude Subscription OAuth login (`claude auth login`)...")
				cmd := exec.Command(claudePath, "auth", "login")
				cmd.Stdin = os.Stdin
				cmd.Stdout = stdout
				cmd.Stderr = stderr
				_ = cmd.Run()
				cfg, _ = LoadSyncedConfig(".", cfgPath, true)
				_ = cfg.Save(cfgPath)
				fmt.Fprintf(stdout, "%s✓ Claude Subscription synced with APW!%s\n", cGreen, cReset)
				return true, nil
			},
		},
		{
			Label:  "3. Claude API (ANTHROPIC_API_KEY)",
			Detail: "Enter Anthropic API Key & fetch live Claude models from api.anthropic.com",
			Action: func() (bool, error) {
				return promptAndSaveAPIKey("ANTHROPIC_API_KEY", "Anthropic Claude API Key (sk-ant-...)", cfgPath, stdin, stdout)
			},
		},
		{
			Label:  "4. ChatGPT API (OPENAI_API_KEY)",
			Detail: "Enter OpenAI API Key & fetch live GPT-4.1 / o4-mini models from api.openai.com",
			Action: func() (bool, error) {
				return promptAndSaveAPIKey("OPENAI_API_KEY", "OpenAI / ChatGPT API Key (sk-...)", cfgPath, stdin, stdout)
			},
		},
		{
			Label:  "5. ChatGPT Subscription (Codex CLI)",
			Detail: "Authenticate via ChatGPT Plus/Pro Subscription (`codex login`)",
			Action: func() (bool, error) {
				codexPath, err := exec.LookPath("codex")
				if err != nil {
					fmt.Fprintln(stdout, "OpenAI Codex CLI (`codex`) not found. Install via `npm i -g @openai/codex` or use Option 4 (OPENAI_API_KEY).")
					return true, nil
				}
				fmt.Fprintln(stdout, "Launching ChatGPT Subscription OAuth (`codex login`)...")
				cmd := exec.Command(codexPath, "login")
				cmd.Stdin = os.Stdin
				cmd.Stdout = stdout
				cmd.Stderr = stderr
				_ = cmd.Run()
				cfg, _ = LoadSyncedConfig(".", cfgPath, true)
				_ = cfg.Save(cfgPath)
				fmt.Fprintf(stdout, "%s✓ ChatGPT Subscription synced with APW!%s\n", cGreen, cReset)
				return true, nil
			},
		},
		{
			Label:  "6. Refresh & Sync All Live Models",
			Detail: "Re-scan AGY CLI, Claude, ChatGPT/OpenAI, and Ollama and update apw.json",
			Action: func() (bool, error) {
				cfg, statuses = LoadSyncedConfig(".", cfgPath, true)
				_ = cfg.Save(cfgPath)
				fmt.Fprintf(stdout, "%s✓ Refreshed all providers and updated %s%s\n", cGreen, cfgPath, cReset)
				return true, nil
			},
		},
	}

	return runSingleLineSelector("APW Authentication & Live Model Hub (/auth)", subtitles, items, stdout)
}

func promptAndSaveAPIKey(envName, label, cfgPath string, stdin io.Reader, stdout io.Writer) (bool, error) {
	reader := bufio.NewReader(stdin)
	fmt.Fprintf(stdout, "  Enter %s: ", label)
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)
	if key == "" {
		fmt.Fprintln(stdout, "Cancelled (empty key).")
		return true, nil
	}
	if err := config.SaveAuthKey(".", envName, key); err != nil {
		return true, err
	}
	_ = os.Setenv(envName, key)
	cfg, statuses := LoadSyncedConfig(".", cfgPath, true)
	_ = cfg.Save(cfgPath)
	total := 0
	for _, st := range statuses {
		total += len(st.Models)
	}
	fmt.Fprintf(stdout, "%s✓ Saved %s to .apw/auth.json (gitignored) and synced %d live models!%s\n", cGreen, envName, total, cReset)
	return true, nil
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
