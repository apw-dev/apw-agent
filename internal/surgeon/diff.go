package surgeon

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	// maxDiffBytes bounds the rendered diff kept in a task report.
	maxDiffBytes = 1024 * 1024
	// diffContext is the number of unchanged lines shown around each change.
	diffContext = 3
	// maxDiffEdits bounds Myers' search (time and memory grow with the square
	// of the edit distance); beyond it the changed region is shown as a
	// whole-block replacement.
	maxDiffEdits = 1000
	// noEOLMarker is appended to the comparison key of a final line that lacks
	// a newline, so adding or removing the final newline is a visible change.
	noEOLMarker = "\x00"
)

type sourceLine struct {
	text  string // without the line ending
	key   string // text plus noEOLMarker when the line has no newline
	crlf  bool
	noEOL bool
}

type diffOp struct {
	kind byte // ' ', '-' or '+'
	a, b int  // line index in the old / new file (unused side is -1)
}

// splitSourceLines splits data into lines, stripping "\n" and "\r\n" so that
// line content is compared independently of the line-ending convention.
func splitSourceLines(data []byte) []sourceLine {
	if len(data) == 0 {
		return nil
	}
	parts := strings.SplitAfter(string(data), "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	lines := make([]sourceLine, len(parts))
	for i, part := range parts {
		line := sourceLine{text: part}
		if text, ok := strings.CutSuffix(part, "\n"); ok {
			line.text = text
			if text, ok := strings.CutSuffix(text, "\r"); ok {
				line.text, line.crlf = text, true
			}
		} else {
			line.noEOL = true
		}
		line.key = line.text
		if line.noEOL {
			line.key += noEOLMarker
		}
		lines[i] = line
	}
	return lines
}

// eolStyle names the line-ending convention of lines: "", "LF", "CRLF" or
// "mixed".
func eolStyle(lines []sourceLine) string {
	var lf, crlf bool
	for _, line := range lines {
		switch {
		case line.noEOL:
		case line.crlf:
			crlf = true
		default:
			lf = true
		}
	}
	switch {
	case lf && crlf:
		return "mixed"
	case crlf:
		return "CRLF"
	case lf:
		return "LF"
	}
	return ""
}

// lineDiff returns an edit script turning a into b. It trims the common prefix
// and suffix, then runs Myers' O((N+M)D) algorithm on the remainder; if the
// remainder needs more than maxEdits edits it is reported as deleted and
// re-added. Within each changed run, deletions precede additions.
func lineDiff(a, b []string, maxEdits int) []diffOp {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	ops := make([]diffOp, 0, len(a)+len(b)-prefix-suffix)
	for i := 0; i < prefix; i++ {
		ops = append(ops, diffOp{' ', i, i})
	}
	midA, midB := a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]
	middle, ok := myers(midA, midB, maxEdits)
	if !ok {
		middle = middle[:0]
		for i := range midA {
			middle = append(middle, diffOp{'-', i, -1})
		}
		for j := range midB {
			middle = append(middle, diffOp{'+', -1, j})
		}
	}
	for _, op := range middle {
		if op.a >= 0 {
			op.a += prefix
		}
		if op.b >= 0 {
			op.b += prefix
		}
		ops = append(ops, op)
	}
	for i := 0; i < suffix; i++ {
		ops = append(ops, diffOp{' ', len(a) - suffix + i, len(b) - suffix + i})
	}
	return deletionsFirst(ops)
}

// myers computes a shortest edit script, or reports false when it needs more
// than maxEdits edits.
func myers(a, b []string, maxEdits int) ([]diffOp, bool) {
	n, m := len(a), len(b)
	if maxEdits > n+m {
		maxEdits = n + m
	}
	offset := maxEdits + 1
	v := make([]int, 2*maxEdits+3)
	var trace [][]int
	for d := 0; d <= maxEdits; d++ {
		// Round d reads only diagonals -d-1..d+1, so keep just that window.
		trace = append(trace, append([]int(nil), v[offset-d-1:offset+d+2]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return myersBacktrack(trace, n, m), true
			}
		}
	}
	return nil, false
}

// myersBacktrack walks trace (trace[d][i] holds diagonal i-d-1 before round d)
// back from (x, y) to the origin.
func myersBacktrack(trace [][]int, x, y int) []diffOp {
	var rev []diffOp
	for d := len(trace) - 1; d >= 0; d-- {
		v, offset := trace[d], d+1
		k := x - y
		prevK := k - 1
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			rev = append(rev, diffOp{' ', x, y})
		}
		if d > 0 {
			if x == prevX {
				y--
				rev = append(rev, diffOp{'+', -1, y})
			} else {
				x--
				rev = append(rev, diffOp{'-', x, -1})
			}
		}
	}
	ops := make([]diffOp, len(rev))
	for i, op := range rev {
		ops[len(rev)-1-i] = op
	}
	return ops
}

func deletionsFirst(ops []diffOp) []diffOp {
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		j := i
		for j < len(ops) && ops[j].kind != ' ' {
			j++
		}
		run := append([]diffOp(nil), ops[i:j]...)
		pos := i
		for _, kind := range []byte{'-', '+'} {
			for _, op := range run {
				if op.kind == kind {
					ops[pos] = op
					pos++
				}
			}
		}
		i = j
	}
	return ops
}

// renderFileDiff renders one changed file as a unified diff with diffContext
// lines of context. Notes about new/deleted files, mode changes, binary
// content and line-ending changes precede the ---/+++ headers.
func renderFileDiff(path string, before, after FileState, original, data []byte) string {
	var b strings.Builder
	switch {
	case !before.Exists:
		b.WriteString("# new file\n")
	case !after.Exists:
		b.WriteString("# deleted file\n")
	case before.Mode != after.Mode:
		fmt.Fprintf(&b, "# mode changed: %04o -> %04o\n", before.Mode.Perm(), after.Mode.Perm())
	}
	if bytes.IndexByte(original, 0) >= 0 || bytes.IndexByte(data, 0) >= 0 {
		fmt.Fprintf(&b, "# binary file changed\n--- a/%s\n+++ b/%s\n", path, path)
		return b.String()
	}
	oldLines, newLines := splitSourceLines(original), splitSourceLines(data)
	oldKeys := make([]string, len(oldLines))
	for i, line := range oldLines {
		oldKeys[i] = line.key
	}
	newKeys := make([]string, len(newLines))
	for i, line := range newLines {
		newKeys[i] = line.key
	}
	ops := lineDiff(oldKeys, newKeys, maxDiffEdits)

	oldStyle, newStyle := eolStyle(oldLines), eolStyle(newLines)
	if before.Exists && after.Exists && oldStyle != "" && newStyle != "" && oldStyle != newStyle {
		fmt.Fprintf(&b, "# line endings changed: %s -> %s\n", oldStyle, newStyle)
	} else {
		eolChanges := 0
		for _, op := range ops {
			if op.kind == ' ' && oldLines[op.a].crlf != newLines[op.b].crlf {
				eolChanges++
			}
		}
		if eolChanges == 1 {
			b.WriteString("# line endings changed on 1 unchanged line\n")
		} else if eolChanges > 1 {
			fmt.Fprintf(&b, "# line endings changed on %d unchanged lines\n", eolChanges)
		}
	}
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", path, path)
	writeHunks(&b, ops, oldLines, newLines)
	return b.String()
}

func writeHunks(b *strings.Builder, ops []diffOp, oldLines, newLines []sourceLine) {
	// oldPos[i] / newPos[i] count the old / new lines before ops[i].
	oldPos := make([]int, len(ops)+1)
	newPos := make([]int, len(ops)+1)
	for i, op := range ops {
		oldPos[i+1], newPos[i+1] = oldPos[i], newPos[i]
		if op.kind != '+' {
			oldPos[i+1]++
		}
		if op.kind != '-' {
			newPos[i+1]++
		}
	}
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := max(i-diffContext, 0)
		j := i
		for {
			for j < len(ops) && ops[j].kind != ' ' {
				j++
			}
			k := j
			for k < len(ops) && ops[k].kind == ' ' {
				k++
			}
			if k == len(ops) || k-j > 2*diffContext {
				break
			}
			j = k
		}
		end := min(j+diffContext, len(ops))
		fmt.Fprintf(b, "@@ -%s +%s @@\n",
			hunkRange(oldPos[start], oldPos[end]-oldPos[start]),
			hunkRange(newPos[start], newPos[end]-newPos[start]))
		for _, op := range ops[start:end] {
			var line sourceLine
			if op.kind == '+' {
				line = newLines[op.b]
			} else {
				line = oldLines[op.a]
			}
			b.WriteByte(op.kind)
			b.WriteString(line.text)
			b.WriteByte('\n')
			if line.noEOL {
				b.WriteString("\\ No newline at end of file\n")
			}
		}
		i = end
	}
}

// hunkRange formats a hunk side as "start,count"; an empty side names the
// line it follows, as in GNU and git unified diffs.
func hunkRange(before, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", before)
	}
	return fmt.Sprintf("%d,%d", before+1, count)
}
