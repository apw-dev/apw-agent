package executor

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/apiwat/apw-agent/internal/surgeon"
)

// quotedPattern finds text the user wrote in double quotes or backticks.
var quotedPattern = regexp.MustCompile("\"([^\"\\n]{2,80})\"|`([^`\\n]{2,80})`")

// quotedLiterals returns the distinct quoted values of an instruction, in order.
func quotedLiterals(instruction string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range quotedPattern.FindAllStringSubmatch(instruction, -1) {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		if strings.TrimSpace(v) == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// isTestFile recognizes test files across the supported languages.
func isTestFile(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	base := p[strings.LastIndex(p, "/")+1:]
	switch {
	case strings.HasSuffix(base, "_test.go"), strings.HasSuffix(base, "_test.py"), strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return true
	case strings.Contains(base, ".test.") || strings.Contains(base, ".spec."):
		return true
	case strings.HasSuffix(base, ".rs") && (strings.HasPrefix(p, "tests/") || strings.Contains(p, "/tests/")):
		return true
	}
	return strings.HasPrefix(p, "tests/") || strings.Contains(p, "/tests/") || strings.Contains(p, "/__tests__/")
}

// mirrorWarnings flags tests that may only mirror the implementation: the task changed test files,
// yet a value the instruction quotes appears in none of them (neither literally nor as an escaped
// string literal). Quoted text can also be an example or a placeholder, so this is a warning for
// the reviewer, not a failure.
func mirrorWarnings(instruction, root string, changed []string) []string {
	literals := quotedLiterals(instruction)
	if len(literals) == 0 {
		return nil
	}
	var tests []string
	for _, f := range changed {
		if !isTestFile(f) {
			continue
		}
		data, _, err := surgeon.ReadInRoot(root, filepath.FromSlash(f))
		if err == nil {
			tests = append(tests, string(data))
		}
	}
	if len(tests) == 0 {
		return nil
	}
	var missing []string
	for _, lit := range literals {
		escaped := strings.Trim(strconv.Quote(lit), `"`)
		found := false
		for _, src := range tests {
			if strings.Contains(src, lit) || strings.Contains(src, escaped) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, strconv.Quote(lit))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("the changed tests never mention %s from the instruction; check that they assert the requested values rather than mirroring the implementation", strings.Join(missing, ", "))}
}
