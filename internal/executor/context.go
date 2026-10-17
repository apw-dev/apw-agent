package executor

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/project"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

const (
	contextBudget    = 96 * 1024
	contextFileLimit = 48 * 1024
	contextIndexMax  = 8 * 1024
)

// sourceExts are text source files worth showing a model, across languages.
var sourceExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".mts": true, ".cts": true,
	".vue": true, ".svelte": true, ".astro": true, ".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true, ".less": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".kts": true, ".scala": true, ".cs": true, ".fs": true, ".rb": true, ".php": true,
	".swift": true, ".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true, ".m": true, ".dart": true, ".lua": true,
	".ex": true, ".exs": true, ".erl": true, ".clj": true, ".sql": true, ".sh": true, ".ps1": true, ".graphql": true, ".proto": true, ".prisma": true,
}

// manifestFiles describe the project; they are shown first, in this order.
var manifestFiles = []string{
	"README.md", "go.mod", "package.json", "tsconfig.json", "tsconfig.app.json", "tsconfig.node.json",
	"vite.config.ts", "vite.config.mts", "vite.config.js", "vitest.config.ts", "vitest.config.js",
	"Cargo.toml", "pyproject.toml", "requirements.txt", "index.html",
}

// skipContextDirs hold dependencies, build output or tool state, never source to edit.
var skipContextDirs = map[string]bool{
	".git": true, ".apw": true, ".worktrees": true, "vendor": true, "node_modules": true, "dist": true, "build": true, "out": true,
	".next": true, ".nuxt": true, ".svelte-kit": true, ".output": true, "coverage": true, "target": true, "__pycache__": true,
	".venv": true, "venv": true, ".turbo": true, ".cache": true, ".parcel-cache": true, ".idea": true, ".vscode": true,
}

// CodeContext includes actual source for any language: project manifests first, then
// source files in path order, each bounded, with an index of files that did not fit
// so the model knows they exist and can plan a read step for them.
func CodeContext(root string) string {
	manifest := map[string]bool{}
	for _, name := range manifestFiles {
		manifest[name] = true
	}
	var sources []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if path != root && skipContextDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if surgeon.AllowedPath(rel) != nil || d.Type()&fs.ModeSymlink != 0 || manifest[filepath.ToSlash(rel)] {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".min.js") || strings.HasSuffix(name, ".min.css") {
			return nil
		}
		if sourceExts[strings.ToLower(filepath.Ext(name))] {
			sources = append(sources, rel)
		}
		return nil
	})
	sort.Strings(sources)
	var ordered []string
	for _, name := range manifestFiles {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
			ordered = append(ordered, name)
		}
	}
	ordered = append(ordered, sources...)

	var b strings.Builder
	var omitted []string
	for _, rel := range ordered {
		if b.Len() >= contextBudget {
			omitted = append(omitted, rel)
			continue
		}
		data, _, err := surgeon.ReadInRoot(root, rel)
		if err != nil {
			continue
		}
		if len(data) > contextFileLimit || b.Len()+len(data) > contextBudget {
			omitted = append(omitted, rel)
			continue
		}
		fmt.Fprintf(&b, "\nFile: %s\n```\n%s\n```\n", filepath.ToSlash(rel), data)
	}
	if len(omitted) > 0 {
		b.WriteString("\n[context budget reached; these files exist but were not included (plan a read step to see one):\n")
		written := 0
		for i, rel := range omitted {
			line := filepath.ToSlash(rel) + "\n"
			if written+len(line) > contextIndexMax {
				fmt.Fprintf(&b, "... and %d more]\n", len(omitted)-i)
				return b.String()
			}
			b.WriteString(line)
			written += len(line)
		}
		b.WriteString("]\n")
	}
	return b.String()
}

// ProjectOverview prefixes the source context with the detected project profile
// and verification pipeline, so plans name real stages and conventions.
func ProjectOverview(root string, override []project.Stage) string {
	return project.Detect(root, override).Describe() + "\n" + CodeContext(root)
}

func (e *Executor) rememberRead(path, content string) {
	entry := fmt.Sprintf("\nRead %s:\n```\n%s\n```\n", path, content)
	if len(entry) > maxReadEntry {
		entry = headUTF8(entry, maxReadEntry) + "\n[read truncated]\n"
	}
	e.ReadContext += entry
	if len(e.ReadContext) > maxReadContext {
		e.ReadContext = tailUTF8(e.ReadContext, maxReadContext)
	}
}

const (
	maxReadEntry   = 32 * 1024
	maxReadContext = 96 * 1024
)

// headUTF8 returns at most n bytes of s without splitting a multi-byte rune
// (Thai text is 3 bytes per character; a byte cut would send invalid UTF-8).
func headUTF8(s string, n int) string {
	if n >= len(s) {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// tailUTF8 returns at most the last n bytes of s, starting on a rune boundary.
func tailUTF8(s string, n int) string {
	if n >= len(s) {
		return s
	}
	start := len(s) - n
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}
