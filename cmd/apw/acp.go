package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/apiwat/apw-agent/internal/acp"
)

// runACP serves the Agent Client Protocol on stdin/stdout: apw acp [--config path]. Only ACP
// messages go to stdout; diagnostics go to stderr.
func runACP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "", "configuration (default: apw.json in the session's working directory)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &acp.Server{ConfigPath: *cfgPath, Version: version}
	if err := server.Serve(ctx, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	return 0
}
