// Command e2ehelper serves the real Desktop (desktop.Server: embedded web/dist, CSP, token guard,
// approvals) on a loopback port with a deterministic in-memory backend, for the Playwright
// completion-gate test. It is a test tool, not shipped.
//
//	go run ./internal/desktop/e2ehelper -addr 127.0.0.1:47911
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/apiwat/apw-agent/internal/desktop"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:47911", "loopback listen address")
	flag.Parse()
	if err := run(*addr); err != nil {
		fmt.Fprintln(os.Stderr, "e2ehelper:", err)
		os.Exit(1)
	}
}

func run(addr string) error {
	ws, err := os.MkdirTemp("", "apw-e2e-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(ws)
	cfgPath := filepath.Join(ws, "apw.json")
	cfg := `{"workspace": ` + strconv.Quote(ws) + `, "policy": {"trusted": true}, ` +
		`"providers": {"m": {"type": "mock", "model": "APW Engine"}}, "roles": {"architect": "m", "reflex": "m"}}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		return err
	}

	srv := desktop.NewServer(cfgPath, "e2e")
	srv.Backend = newFlowBackend(ws)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdown)
	}()
	fmt.Printf("APW e2e Desktop listening on http://%s\n", ln.Addr())
	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
