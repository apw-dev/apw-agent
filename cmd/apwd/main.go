package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/daemon"
	"os"
	"os/signal"
	"syscall"
)

var version = buildinfo.Version

func main() {
	showVersion := flag.Bool("version", false, "print version/build metadata")
	cfg := flag.String("config", "./apw.json", "workspace configuration")
	flag.Parse()
	if *showVersion {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := daemon.Run(ctx, *cfg, version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
