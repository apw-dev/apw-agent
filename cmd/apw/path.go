package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// The installer adds and removes the CLI folder on the user PATH through `apw path add|remove
// <dir>`: NSIS reads at most 1024 characters of a registry string and returns "" for a longer
// PATH, so an installer that rewrites PATH itself can wipe it.

// pathWithEntry returns path with dir appended unless an equivalent entry is already present.
func pathWithEntry(path, dir string) (string, bool) {
	for _, e := range strings.Split(path, ";") {
		if samePathEntry(e, dir) {
			return path, false
		}
	}
	if strings.TrimSpace(path) == "" {
		return dir, true
	}
	return strings.TrimRight(path, ";") + ";" + dir, true
}

// pathWithoutEntry returns path without the entries equivalent to dir, keeping the others as
// written (including unexpanded %VARIABLES%).
func pathWithoutEntry(path, dir string) (string, bool) {
	parts := strings.Split(path, ";")
	kept := parts[:0:0]
	removed := false
	for _, e := range parts {
		if samePathEntry(e, dir) {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	return strings.Join(kept, ";"), removed
}

func samePathEntry(a, b string) bool {
	norm := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		return strings.ToLower(strings.TrimRight(filepath.Clean(s), `\/`))
	}
	return norm(a) != "" && norm(a) == norm(b)
}

func runPathCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "add" && args[0] != "remove") || strings.TrimSpace(args[1]) == "" {
		fmt.Fprintln(stderr, "usage: apw path add|remove <dir>   (user PATH; used by the Windows installer)")
		return 2
	}
	dir, err := filepath.Abs(args[1])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	changed, err := editUserPath(dir, args[0] == "add")
	if err != nil {
		fmt.Fprintln(stderr, "user PATH not changed:", err)
		return 1
	}
	switch {
	case !changed:
		fmt.Fprintf(stdout, "user PATH already %s %s\n", map[bool]string{true: "contains", false: "omits"}[args[0] == "add"], dir)
	case args[0] == "add":
		fmt.Fprintf(stdout, "added %s to the user PATH\n", dir)
	default:
		fmt.Fprintf(stdout, "removed %s from the user PATH\n", dir)
	}
	return 0
}
