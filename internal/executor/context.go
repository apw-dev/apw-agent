package executor

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

// CodeContext includes actual source, with deterministic ordering and explicit truncation.
func CodeContext(root string) string {
	var paths []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".apw", ".worktrees", "vendor", "node_modules", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if surgeon.AllowedPath(rel) != nil || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".go" || rel == "go.mod" || rel == "README.md" {
			paths = append(paths, rel)
		}
		return nil
	})
	sort.Strings(paths)
	var b strings.Builder
	for _, rel := range paths {
		data, _, err := surgeon.ReadInRoot(root, rel)
		if err != nil {
			continue
		}
		if b.Len()+len(data) > 96*1024 {
			fmt.Fprintf(&b, "\n[context budget reached; omitted %s and later files]\n", rel)
			break
		}
		fmt.Fprintf(&b, "\nFile: %s\n```\n%s\n```\n", filepath.ToSlash(rel), data)
	}
	return b.String()
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
