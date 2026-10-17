package executor

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

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
	if len(entry) > 32*1024 {
		entry = entry[:32*1024] + "\n[read truncated]\n"
	}
	e.ReadContext += entry
	if len(e.ReadContext) > 96*1024 {
		e.ReadContext = e.ReadContext[len(e.ReadContext)-96*1024:]
	}
}
