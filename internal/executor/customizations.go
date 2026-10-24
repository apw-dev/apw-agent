package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/process"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

// WorkspaceSkill represents a procedural skill discovered in .apw/skills/<slug>/SKILL.md.
type WorkspaceSkill struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Body        string `json:"-"`
}

// parseFrontmatter extracts simple YAML frontmatter (name, description, trigger, glob) and body.
func parseFrontmatter(raw string) (map[string]string, string) {
	meta := make(map[string]string)
	trimmed := strings.TrimLeft(raw, "\ufeff \t\r\n")
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return meta, strings.TrimSpace(raw)
	}
	lines := strings.Split(trimmed, "\n")
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
		line := strings.TrimSpace(lines[i])
		if colon := strings.IndexByte(line, ':'); colon > 0 {
			k := strings.ToLower(strings.TrimSpace(line[:colon]))
			v := strings.Trim(strings.TrimSpace(line[colon+1:]), `"'`)
			if v != "" && v != ">-" && v != "|" {
				meta[k] = v
			}
		}
	}
	if endIdx < 0 {
		return meta, strings.TrimSpace(raw)
	}
	body := strings.TrimSpace(strings.Join(lines[endIdx+1:], "\n"))
	return meta, body
}

// DiscoverWorkspaceSkills scans .apw/skills/*/SKILL.md using link-safe customization reads.
func DiscoverWorkspaceSkills(root string) []WorkspaceSkill {
	skillsDir := filepath.Join(root, ".apw", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var out []WorkspaceSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		rel := filepath.Join(".apw", "skills", slug, "SKILL.md")
		data, err := surgeon.ReadAPWCustomization(root, rel)
		if err != nil || len(data) == 0 {
			continue
		}
		meta, body := parseFrontmatter(string(data))
		name := meta["name"]
		if name == "" {
			name = slug
		}
		desc := meta["description"]
		if desc == "" {
			for _, line := range strings.Split(body, "\n") {
				line = strings.TrimSpace(strings.TrimLeft(line, "# "))
				if line != "" {
					desc = clip(line, 120)
					break
				}
			}
		}
		out = append(out, WorkspaceSkill{
			Slug:        slug,
			Name:        name,
			Description: desc,
			Path:        filepath.ToSlash(rel),
			Body:        headUTF8(body, 6*1024),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// BuiltinSkills returns the built-in procedural skills always available in APW Desktop & CLI.
func BuiltinSkills() []WorkspaceSkill {
	return []WorkspaceSkill{
		{
			Slug:        "diagram-design",
			Name:        "diagram-design",
			Description: "Architecture, Mermaid flowchart, sequence, data-flow & system overview diagrams",
			Path:        "builtin:diagram-design",
			Body:        "Create clear, well-structured Mermaid diagrams (`flowchart TD/LR`, `sequenceDiagram`, `stateDiagram-v2`, `erDiagram`) and architectural breakdowns covering every subsystem, component boundary, API contract, and data flow. Always quote node labels containing parentheses or special characters.",
		},
		{
			Slug:        "code-audit",
			Name:        "code-audit",
			Description: "Full-stack Backend & Frontend codebase security, concurrency, and quality audit",
			Path:        "builtin:code-audit",
			Body:        "Audit both Backend and Frontend layers systematically: check security boundaries, path/input validation, error handling, concurrency safety, and test coverage. Cite exact file paths and distinguish confirmed defects from suggestions.",
		},
		{
			Slug:        "ux-ui-design",
			Name:        "ux-ui-design",
			Description: "Modern responsive UI/UX components, accessibility, contrast & clean layout systems",
			Path:        "builtin:ux-ui-design",
			Body:        "Follow modern UX/UI engineering standards: consistent spacing/typography tokens, clear interactive states (hover, focus, disabled, loading), high contrast in both light and dark themes, and accessible keyboard navigation.",
		},
		{
			Slug:        "test-generation",
			Name:        "test-generation",
			Description: "Automated unit, edge-case & integration test suite synthesis",
			Path:        "builtin:test-generation",
			Body:        "Synthesize deterministic unit and integration tests covering happy paths, boundary conditions, and failure modes without network flakiness or external side effects.",
		},
		{
			Slug:        "refactor-safe",
			Name:        "refactor-safe",
			Description: "Surgical multi-file refactoring with strict behavior and API preservation",
			Path:        "builtin:refactor-safe",
			Body:        "Perform surgical, behavior-preserving refactorings across files while keeping public signatures, existing tests, and module boundaries intact.",
		},
	}
}

// DiscoverAllSkills combines workspace skills (.apw/skills/*/SKILL.md) with built-in APW skills.
// Workspace skills with the same slug take precedence over built-in skills.
func DiscoverAllSkills(root string) []WorkspaceSkill {
	ws := DiscoverWorkspaceSkills(root)
	seen := make(map[string]bool, len(ws))
	out := make([]WorkspaceSkill, 0, len(ws)+5)
	for _, sk := range ws {
		seen[strings.ToLower(sk.Slug)] = true
		out = append(out, sk)
	}
	for _, b := range BuiltinSkills() {
		if !seen[strings.ToLower(b.Slug)] {
			out = append(out, b)
		}
	}
	return out
}

// ProgressiveSkillsContext implements Antigravity-style progressive disclosure for .apw/skills:
// all discovered skills are listed by name + description, while skills matching the instruction
// (or explicitly pinned via @skill:<slug>) have their full procedure expanded automatically.
func ProgressiveSkillsContext(root, instruction string) string {
	skills := DiscoverWorkspaceSkills(root)
	instrLower := strings.ToLower(instruction)
	for _, b := range BuiltinSkills() {
		slugLower := strings.ToLower(b.Slug)
		if strings.Contains(instrLower, "@skill:"+slugLower) || strings.Contains(instrLower, "[<> "+slugLower+"]") {
			exists := false
			for _, w := range skills {
				if strings.EqualFold(w.Slug, b.Slug) {
					exists = true
					break
				}
			}
			if !exists {
				skills = append(skills, b)
			}
		}
	}
	if len(skills) == 0 {
		return ""
	}
	tokens := extractSearchTokens(instruction)

	type scoredSkill struct {
		skill WorkspaceSkill
		score int
	}
	var scored []scoredSkill
	for _, sk := range skills {
		sc := 0
		slugLower := strings.ToLower(sk.Slug)
		nameLower := strings.ToLower(sk.Name)
		descLower := strings.ToLower(sk.Description)
		if strings.Contains(instrLower, "@skill:"+slugLower) || strings.Contains(instrLower, "@skill:"+nameLower) ||
			(len(slugLower) >= 4 && strings.Contains(instrLower, slugLower)) {
			sc += 50
		}
		for _, tok := range tokens {
			if strings.Contains(slugLower, tok) || strings.Contains(nameLower, tok) {
				sc += 10
			}
			if strings.Contains(descLower, tok) {
				sc += 6
			}
		}
		scored = append(scored, scoredSkill{skill: sk, score: sc})
	}

	var sb strings.Builder
	sb.WriteString("\nWorkspace Skills (.apw/skills — progressive disclosure index):\n")
	for _, s := range scored {
		fmt.Fprintf(&sb, "- %s (%s): %s\n", s.skill.Slug, s.skill.Path, s.skill.Description)
	}

	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	activated := 0
	for _, s := range scored {
		if s.score < 10 || activated >= 2 {
			break
		}
		activated++
		fmt.Fprintf(&sb, "\nActivated Skill Procedure [%s]:\n%s\n[end of skill %s]\n", s.skill.Slug, s.skill.Body, s.skill.Slug)
	}
	return sb.String()
}

// DiscoverModularRules loads .apw/rules/*.md files with deduplication and progressive trigger support.
func DiscoverModularRules(root, instruction string) string {
	rulesDir := filepath.Join(root, ".apw", "rules")
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return ""
	}
	instrLower := strings.ToLower(instruction)
	tokens := extractSearchTokens(instruction)
	seenHash := make(map[string]bool)
	var sb strings.Builder
	budget := 8 * 1024

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		rel := filepath.Join(".apw", "rules", entry.Name())
		data, err := surgeon.ReadAPWCustomization(root, rel)
		if err != nil || len(data) == 0 {
			continue
		}
		meta, body := parseFrontmatter(string(data))
		if body == "" {
			continue
		}
		hash := surgeon.CalculateHash(body)
		if seenHash[hash] {
			continue
		}
		seenHash[hash] = true

		trigger := strings.ToLower(meta["trigger"])
		if trigger == "model_decision" && instruction != "" {
			matched := false
			hay := strings.ToLower(entry.Name() + " " + meta["description"] + " " + meta["glob"])
			for _, tok := range tokens {
				if strings.Contains(hay, tok) || strings.Contains(instrLower, strings.TrimSuffix(strings.ToLower(entry.Name()), ".md")) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		block := fmt.Sprintf("\nRule (%s):\n%s\n", filepath.ToSlash(rel), headUTF8(body, 2*1024))
		if sb.Len()+len(block) > budget {
			break
		}
		sb.WriteString(block)
	}
	return sb.String()
}

// HookEvent identifies a point in the APW Triad execution lifecycle.
type HookEvent string

const (
	HookPrePlan    HookEvent = "PrePlan"
	HookPrePatch   HookEvent = "PrePatch"
	HookPostVerify HookEvent = "PostVerify"
)

// HookHandlerConfig defines a single shell command hook inside .apw/hooks.json.
type HookHandlerConfig struct {
	Command string `json:"command"`
	Matcher string `json:"matcher,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// HookEntryConfig groups lifecycle event handlers under a named hook in .apw/hooks.json.
type HookEntryConfig struct {
	Enabled    *bool               `json:"enabled,omitempty"`
	PrePlan    []HookHandlerConfig `json:"PrePlan,omitempty"`
	PrePatch   []HookHandlerConfig `json:"PrePatch,omitempty"`
	PostVerify []HookHandlerConfig `json:"PostVerify,omitempty"`
}

// HookPayload is sent on stdin to each lifecycle hook command.
type HookPayload struct {
	TaskID     string    `json:"taskId"`
	Workspace  string    `json:"workspace"`
	Event      HookEvent `json:"event"`
	TargetFile string    `json:"targetFile,omitempty"`
	Action     string    `json:"action,omitempty"`
}

// HookDecision is the JSON response a lifecycle hook prints to stdout.
type HookDecision struct {
	Decision          string `json:"decision"` // "allow" (default) or "deny"
	Reason            string `json:"reason,omitempty"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// runLifecycleHooks executes matching hooks from .apw/hooks.json when the workspace is trusted.
// Hooks are policy gates, so they fail closed: an unreadable or malformed hooks.json, an invalid
// matcher, or a hook that cannot start, exits non-zero or times out stops the task. Hooks run on
// the host, so they are refused while a sandbox mode isolates repository commands.
func (e *Executor) runLifecycleHooks(ctx context.Context, event HookEvent, targetFile, action string) (string, error) {
	if !e.Trusted {
		return "", nil
	}
	data, err := surgeon.ReadAPWCustomization(e.Workspace, filepath.Join(".apw", "hooks.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lifecycle hooks: cannot read .apw/hooks.json: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return "", nil
	}
	var rawMap map[string]HookEntryConfig
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return "", fmt.Errorf("lifecycle hooks: invalid .apw/hooks.json: %w", err)
	}
	names := make([]string, 0, len(rawMap))
	for k := range rawMap {
		names = append(names, k)
	}
	sort.Strings(names)

	payloadBytes, _ := json.Marshal(HookPayload{
		TaskID:     e.TaskID,
		Workspace:  e.Workspace,
		Event:      event,
		TargetFile: filepath.ToSlash(targetFile),
		Action:     action,
	})

	var extraContext strings.Builder
	for _, name := range names {
		entry := rawMap[name]
		if entry.Enabled != nil && !*entry.Enabled {
			continue
		}
		var handlers []HookHandlerConfig
		switch event {
		case HookPrePlan:
			handlers = entry.PrePlan
		case HookPrePatch:
			handlers = entry.PrePatch
		case HookPostVerify:
			handlers = entry.PostVerify
		}
		for _, h := range handlers {
			cmdStr := strings.TrimSpace(h.Command)
			if cmdStr == "" {
				continue
			}
			if h.Matcher != "" && h.Matcher != "*" {
				re, regErr := regexp.Compile(h.Matcher)
				if regErr != nil {
					return "", fmt.Errorf("lifecycle hook %q (%s): invalid matcher: %w", name, event, regErr)
				}
				matchTarget := filepath.ToSlash(targetFile)
				if matchTarget == "" {
					matchTarget = action
				}
				if !re.MatchString(matchTarget) {
					continue
				}
			}
			if e.Sandbox.Active() {
				return "", fmt.Errorf("lifecycle hook %q (%s) runs on the host and is refused while sandbox mode %q is active; disable it in .apw/hooks.json or set sandbox mode off", name, event, e.Sandbox.Mode)
			}
			timeout := time.Duration(h.Timeout) * time.Second
			if timeout <= 0 || timeout > 30*time.Second {
				timeout = 10 * time.Second
			}
			hCtx, cancel := context.WithTimeout(ctx, timeout)
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.CommandContext(hCtx, "cmd", "/c", cmdStr)
			} else {
				cmd = exec.CommandContext(hCtx, "sh", "-c", cmdStr)
			}
			cmd.Dir = e.Workspace
			cmd.Stdin = bytes.NewReader(payloadBytes)
			var stdout, stderr process.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			runErr := process.Run(hCtx, cmd)
			timedOut := errors.Is(hCtx.Err(), context.DeadlineExceeded)
			cancel()
			if runErr != nil {
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				detail := headUTF8(strings.TrimSpace(stderr.String()), 1024)
				if timedOut {
					detail = fmt.Sprintf("timed out after %s", timeout)
				} else if detail == "" {
					detail = runErr.Error()
				}
				return "", fmt.Errorf("lifecycle hook %q (%s) failed: %s", name, event, detail)
			}
			outBytes := []byte(strings.TrimSpace(stdout.String()))
			if len(outBytes) == 0 {
				continue
			}
			var dec HookDecision
			if json.Unmarshal(outBytes, &dec) == nil {
				if strings.EqualFold(dec.Decision, "deny") {
					reason := strings.TrimSpace(dec.Reason)
					if reason == "" {
						reason = fmt.Sprintf("blocked by lifecycle hook %q (%s)", name, event)
					}
					return "", errors.New(reason)
				}
				if extra := strings.TrimSpace(dec.AdditionalContext); extra != "" {
					fmt.Fprintf(&extraContext, "\n[Hook %s (%s)]: %s\n", name, event, headUTF8(extra, 2*1024))
				}
			}
		}
	}
	return extraContext.String(), nil
}
