package main

import (
	"path/filepath"
	"strings"
)

// WorkspaceArg returns the first non-flag argument as an absolute path, or "" if there is none.
// It is the workspace folder passed to APW.exe (shortcut, Explorer "Open with", second launch).
func WorkspaceArg(args []string) string {
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		if abs, err := filepath.Abs(a); err == nil {
			return abs
		}
		return a
	}
	return ""
}

// DevtoolsRequested reports whether --devtools was passed.
func DevtoolsRequested(args []string) bool {
	for _, a := range args {
		if a == "--devtools" {
			return true
		}
	}
	return false
}
