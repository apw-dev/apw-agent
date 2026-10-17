package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/apiwat/apw-agent/internal/core"
)

func runVerify(args []string, stdout, stderr io.Writer) int {
	o, code := parseOptions("verify", args, stderr)
	if code != 0 {
		return code
	}
	if len(o.Args) > 1 || o.Explicit && len(o.Args) > 0 {
		fmt.Fprintln(stderr, "Usage: apw verify [dir] OR apw verify --config <path>")
		return 2
	}
	if len(o.Args) == 1 {
		info, err := os.Stat(o.Args[0])
		if err != nil || !info.IsDir() {
			fmt.Fprintln(stderr, "verification directory does not exist")
			return 1
		}
		o.Config = filepath.Join(o.Args[0], "apw.json")
	}
	o.Args = []string{"Verify project without repairs"}
	return runSubmitOptions(core.ModeVerify, o, stdout, stderr)
}
