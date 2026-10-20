package main

import (
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceArg returns the first non-flag argument as an absolute path, or "" if there is none.
// It is the workspace folder passed to APW.exe (shortcut, Explorer "Open with").
func WorkspaceArg(args []string) string {
	wd, _ := os.Getwd()
	return WorkspaceArgFrom(args, wd)
}

// WorkspaceArgFrom is WorkspaceArg with relative paths resolved against wd, for a second
// launch whose working directory differs from this process's.
func WorkspaceArgFrom(args []string, wd string) string {
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		if !filepath.IsAbs(a) {
			a = filepath.Join(wd, a)
		}
		return filepath.Clean(a)
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
