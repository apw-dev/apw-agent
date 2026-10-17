package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

// Code lookup lets the Architect search the source workspace before it plans when the code context
// did not fit the prompt. It only reads: no repository code runs, protected paths (.git, .apw, .env,
// keys) are invisible, links are not followed, and every result is bounded.

const (
	lookupMaxQueries    = 6
	lookupMaxMatches    = 40
	lookupMaxGlobFiles  = 200
	lookupMaxReadLines  = 300
	lookupMaxLineLength = 240
	lookupMaxFileSize   = 1 << 20
	lookupMaxOutput     = 32 * 1024
)

type grepQuery struct {
	Pattern string `json:"pattern"`
	Glob    string `json:"glob,omitempty"`
}

type readRange struct {
	File  string `json:"file"`
	Start int    `json:"start,omitempty"`
	End   int    `json:"end,omitempty"`
}

// globRegexp converts a slash-separated glob (*, ?, ** for any directories) into an anchored regexp.
func globRegexp(glob string) (*regexp.Regexp, error) {
	glob = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(glob)), "./")
	if glob == "" || len(glob) > 200 {
		return nil, fmt.Errorf("glob must be 1-200 characters")
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// lookupFiles lists the workspace's searchable files (slash paths) in order.
func lookupFiles(ctx context.Context, root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if path != root && (skipContextDirs[d.Name()] || surgeon.AllowedPath(rel) != nil) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() || surgeon.AllowedPath(rel) != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files
}

func clipLine(line string) string {
	line = strings.TrimRight(line, "\r")
	if len(line) > lookupMaxLineLength {
		return headUTF8(line, lookupMaxLineLength) + "..."
	}
	return line
}

func (e *Executor) runGlob(files []string, glob string) string {
	re, err := globRegexp(glob)
	if err != nil {
		return "error: " + err.Error()
	}
	var b strings.Builder
	n := 0
	for _, f := range files {
		if !re.MatchString(f) {
			continue
		}
		if n == lookupMaxGlobFiles {
			b.WriteString("[more files not listed]\n")
			break
		}
		b.WriteString(f + "\n")
		n++
	}
	if n == 0 {
		return "no files match\n"
	}
	return b.String()
}

func (e *Executor) runGrep(ctx context.Context, root string, files []string, q grepQuery) string {
	if q.Pattern == "" || len(q.Pattern) > 200 {
		return "error: pattern must be 1-200 characters\n"
	}
	re, err := regexp.Compile(q.Pattern)
	if err != nil {
		return "error: invalid regular expression: " + err.Error() + "\n"
	}
	var only *regexp.Regexp
	if q.Glob != "" {
		if only, err = globRegexp(q.Glob); err != nil {
			return "error: " + err.Error() + "\n"
		}
	}
	var b strings.Builder
	matches := 0
	for _, f := range files {
		if ctx.Err() != nil || matches >= lookupMaxMatches {
			break
		}
		if only != nil && !only.MatchString(f) {
			continue
		}
		data, _, err := surgeon.ReadInRoot(root, filepath.FromSlash(f))
		if err != nil || len(data) > lookupMaxFileSize || bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 64*1024), lookupMaxFileSize)
		for line := 1; scanner.Scan(); line++ {
			if re.MatchString(scanner.Text()) {
				fmt.Fprintf(&b, "%s:%d: %s\n", f, line, clipLine(scanner.Text()))
				if matches++; matches >= lookupMaxMatches {
					b.WriteString("[more matches not listed]\n")
					break
				}
			}
		}
	}
	if matches == 0 {
		return "no matches\n"
	}
	return b.String()
}

func (e *Executor) runRead(root string, r readRange) string {
	rel := filepath.FromSlash(strings.TrimSpace(r.File))
	if err := surgeon.AllowedPath(rel); err != nil {
		return "error: " + err.Error() + "\n"
	}
	data, _, err := surgeon.ReadInRoot(root, rel)
	if err != nil {
		return "error: " + err.Error() + "\n"
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "error: binary file\n"
	}
	lines := strings.Split(string(data), "\n")
	start, end := r.Start, r.End
	if start < 1 {
		start = 1
	}
	if end < start || end > len(lines) {
		end = len(lines)
	}
	if end-start+1 > lookupMaxReadLines {
		end = start + lookupMaxReadLines - 1
	}
	if start > len(lines) {
		return fmt.Sprintf("error: file has %d lines\n", len(lines))
	}
	var b strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i, clipLine(lines[i-1]))
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "[file continues to line %d]\n", len(lines))
	}
	return b.String()
}

// codeLookup runs the requested read-only searches against the source workspace and formats the
// results for the planning prompt. It returns the text and one label per query for reports.
func (e *Executor) codeLookup(ctx context.Context, grep []grepQuery, globs []string, reads []readRange) (string, []string) {
	root := e.Workspace
	var files []string
	if len(grep) > 0 || len(globs) > 0 {
		files = lookupFiles(ctx, root)
	}
	var out strings.Builder
	var labels []string
	queries := 0
	add := func(label, result string) {
		if queries >= lookupMaxQueries || out.Len() >= lookupMaxOutput {
			return
		}
		queries++
		labels = append(labels, label)
		block := "\n--- " + label + " ---\n" + result
		if out.Len()+len(block) > lookupMaxOutput {
			block = headUTF8(block, lookupMaxOutput-out.Len()) + "\n[lookup output truncated]\n"
		}
		out.WriteString(block)
	}
	for _, g := range globs {
		add("glob "+g, e.runGlob(files, g))
	}
	for _, q := range grep {
		label := "grep " + q.Pattern
		if q.Glob != "" {
			label += " in " + q.Glob
		}
		add(label, e.runGrep(ctx, root, files, q))
	}
	for _, r := range reads {
		add(fmt.Sprintf("read %s:%d-%d", r.File, r.Start, r.End), e.runRead(root, r))
	}
	if out.Len() == 0 {
		return "", labels
	}
	return "\nCode lookup results (read-only searches of the source workspace that you requested):" + out.String() + "--- end of code lookup ---\n", labels
}
