package executor

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

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
	lookupMaxTurns      = 3
)

type grepQuery struct {
	Pattern string `json:"pattern"`
	Glob    string `json:"glob,omitempty"`
	Context int    `json:"context,omitempty"`
	Mode    string `json:"mode,omitempty"` // "content" (default), "files_with_matches", "count"
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

// lookupFiles lists the workspace's searchable files (slash paths) in order, respecting .gitignore.
func lookupFiles(ctx context.Context, root string) []string {
	var files []string
	gitRules := loadGitignore(root)
	_ = filepath.WalkDir(root, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, _ := filepath.Rel(root, fpath)
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if fpath != root && (skipContextDirs[d.Name()] || surgeon.AllowedPath(rel) != nil || gitRules.ignored(relSlash, true)) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() || surgeon.AllowedPath(rel) != nil || gitRules.ignored(relSlash, false) {
			return nil
		}
		files = append(files, relSlash)
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

type fileGrepHit struct {
	file    string
	count   int
	lines   []string
	matches int
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
	ctxLines := q.Context
	if ctxLines < 0 {
		ctxLines = 0
	}
	if ctxLines > 10 {
		ctxLines = 10
	}
	mode := strings.ToLower(strings.TrimSpace(q.Mode))

	var candidates []string
	for _, f := range files {
		if only != nil && !only.MatchString(f) {
			continue
		}
		candidates = append(candidates, f)
	}
	if len(candidates) == 0 {
		return "no matches\n"
	}

	hits := make([]fileGrepHit, len(candidates))
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		workers = 2
	}
	if workers > 8 {
		workers = 8
	}
	if len(candidates) < workers {
		workers = len(candidates)
	}

	jobs := make(chan int, len(candidates))
	for i := range candidates {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					return
				}
				f := candidates[idx]
				data, _, err := surgeon.ReadInRoot(root, filepath.FromSlash(f))
				if err != nil || len(data) > lookupMaxFileSize || bytes.IndexByte(data, 0) >= 0 {
					continue
				}
				rawLines := strings.Split(string(data), "\n")
				var matchedIdxs []int
				for i, line := range rawLines {
					text := strings.TrimRight(line, "\r")
					if re.MatchString(text) {
						matchedIdxs = append(matchedIdxs, i)
					}
				}
				if len(matchedIdxs) == 0 {
					continue
				}
				hit := fileGrepHit{file: f, count: len(matchedIdxs)}
				if mode == "files_with_matches" || mode == "count" {
					hits[idx] = hit
					continue
				}
				isMatch := make(map[int]bool, len(matchedIdxs))
				include := make(map[int]bool, len(matchedIdxs)*(2*ctxLines+1))
				for _, mIdx := range matchedIdxs {
					isMatch[mIdx] = true
					start := mIdx - ctxLines
					if start < 0 {
						start = 0
					}
					end := mIdx + ctxLines
					if end >= len(rawLines) {
						end = len(rawLines) - 1
					}
					for k := start; k <= end; k++ {
						include[k] = true
					}
				}
				for i := 0; i < len(rawLines); i++ {
					if !include[i] {
						continue
					}
					clipped := clipLine(rawLines[i])
					if isMatch[i] {
						hit.lines = append(hit.lines, fmt.Sprintf("%s:%d: %s\n", f, i+1, clipped))
						hit.matches++
					} else {
						hit.lines = append(hit.lines, fmt.Sprintf("%s-%d- %s\n", f, i+1, clipped))
					}
				}
				hits[idx] = hit
			}
		}()
	}
	wg.Wait()

	var b strings.Builder
	matches := 0
	for _, h := range hits {
		if h.count == 0 {
			continue
		}
		switch mode {
		case "files_with_matches":
			b.WriteString(h.file + "\n")
			matches++
			if matches >= lookupMaxMatches {
				b.WriteString("[more matches not listed]\n")
				return b.String()
			}
		case "count":
			fmt.Fprintf(&b, "%s: %d\n", h.file, h.count)
			matches++
			if matches >= lookupMaxMatches {
				b.WriteString("[more matches not listed]\n")
				return b.String()
			}
		default:
			for _, lineStr := range h.lines {
				b.WriteString(lineStr)
				if strings.HasPrefix(lineStr, h.file+":") {
					matches++
					if matches >= lookupMaxMatches {
						b.WriteString("[more matches not listed]\n")
						return b.String()
					}
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

func (e *Executor) runSymbol(root string, graph *surgeon.WorkspaceGraph, query string) string {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 120 {
		return "error: symbol name must be 1-120 characters\n"
	}
	if graph == nil {
		return "no symbol definitions found\n"
	}
	defs := graph.FindSymbols(query)
	if len(defs) == 0 {
		return "no symbol definitions match " + query + "\n"
	}
	var b strings.Builder
	limit := 3
	if len(defs) < limit {
		limit = len(defs)
	}
	for i := 0; i < limit; i++ {
		d := defs[i]
		fmt.Fprintf(&b, "%s:%d-%d [%s %s]\n", d.File, d.StartLine, d.EndLine, d.Kind, d.Name)
		b.WriteString(e.runRead(root, readRange{File: d.File, Start: max(1, d.StartLine-2), End: d.EndLine}))
		if callers := graph.CallersOf(d.Name); len(callers) > 0 {
			b.WriteString("Callers:\n")
			for cIdx, c := range callers {
				if cIdx >= 8 {
					b.WriteString("  [more callers omitted]\n")
					break
				}
				fmt.Fprintf(&b, "  - %s:%d (%s)\n", c.FromFile, c.Line, c.FromSymbol)
			}
		}
	}
	if len(defs) > limit {
		fmt.Fprintf(&b, "[%d more symbol definitions omitted]\n", len(defs)-limit)
	}
	return b.String()
}

// codeLookup runs the requested read-only searches against the source workspace and formats the
// results for the planning prompt. It returns the text and one label per query for reports.
func (e *Executor) codeLookup(ctx context.Context, grep []grepQuery, globs []string, reads []readRange, symbols ...string) (string, []string) {
	root := e.Workspace
	var files []string
	if len(grep) > 0 || len(globs) > 0 {
		files = lookupFiles(ctx, root)
	}
	var graph *surgeon.WorkspaceGraph
	if len(symbols) > 0 {
		graph, _ = surgeon.InspectWorkspaceGraph(root)
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
	for _, s := range symbols {
		if strings.TrimSpace(s) == "" {
			continue
		}
		add("symbol "+strings.TrimSpace(s), e.runSymbol(root, graph, s))
	}
	for _, r := range reads {
		add(fmt.Sprintf("read %s:%d-%d", r.File, r.Start, r.End), e.runRead(root, r))
	}
	if out.Len() == 0 {
		return "", labels
	}
	return "\nCode lookup results (read-only searches of the source workspace that you requested):" + out.String() + "--- end of code lookup ---\n", labels
}
