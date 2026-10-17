package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/apiwat/apw-agent/internal/app"
	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/channels/telegram"
	"github.com/apiwat/apw-agent/internal/retention"
	"github.com/apiwat/apw-agent/internal/surgeon"
	"github.com/apiwat/apw-agent/internal/transport"
)

func Run(ctx context.Context, cfgPath, version string) error {
	cfg, absCfg, err := LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	backend, err := app.Open(ctx, cfg, nil)
	if err != nil {
		return err
	}
	defer backend.Close()
	backend.ConfigPath = absCfg
	if cfg.Retention.AutoCleanup {
		// Startup recovery has finished and no request can arrive yet. Never exports or rotates the open log.
		report, cleanupErr := retention.Run(ctx, cfg.Workspace, backend.Store, retention.Resolve(cfg.Retention), retention.Options{})
		if cleanupErr != nil {
			fmt.Fprintln(os.Stderr, "retention: automatic cleanup stopped:", cleanupErr)
		} else {
			fmt.Fprintf(os.Stderr, "retention: removed %d task(s), protected %d, failed %d, events %d, reports %d\n", len(report.Removed), len(report.Protected), len(report.Failed), report.Database.Events, report.Database.Reports)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return err
	}
	st := stateFile{State: State{Build: buildinfo.Current(), ConfigPath: absCfg, Protocol: transport.Version, Running: true, PID: os.Getpid(), Workspace: cfg.Workspace, StartedAt: time.Now(), Heartbeat: time.Now(), Address: "http://" + ln.Addr().String(), SkillsLoaded: CountSkills(cfg.Workspace)}, Token: hex.EncodeToString(secret[:])}
	writeState := func() error {
		st.Heartbeat = time.Now()
		data, err := json.Marshal(st)
		if err != nil {
			return err
		}
		return surgeon.WriteFileAtomic(stateFilePath(cfg.Workspace), data, 0600)
	}
	if err = writeState(); err != nil {
		return err
	}
	defer func() {
		if current, e := readState(cfg.Workspace); e == nil && current.Token == st.Token {
			_ = os.Remove(stateFilePath(cfg.Workspace))
		}
	}()
	srv := &http.Server{Handler: transport.Handler(backend, st.Token, st.State, cancel), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	defer srv.Close()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	var botDone chan struct{}
	defer func() {
		cancel()
		if botDone != nil {
			<-botDone
		}
	}()
	if os.Getenv("APW_TELEGRAM_TOKEN") != "" && os.Getenv("APW_TELEGRAM_ALLOW") != "" {
		bot, e := telegram.NewFromEnv(version)
		if e != nil {
			return e
		}
		bot.Backend = backend
		bot.Store = backend.Store
		botDone = make(chan struct{})
		go func() {
			defer close(botDone)
			if e := bot.Run(ctx); e != nil {
				cancel()
			}
		}()
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-backend.Done():
			if err := backend.Err(); err != nil {
				return err
			}
			cancel()
		case <-ctx.Done():
			shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelShutdown()
			return srv.Shutdown(shutdown)
		case err := <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ticker.C:
			if err := writeState(); err != nil {
				return err
			}
		}
	}
}
