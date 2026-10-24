package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/apiwat/apw-agent/internal/buildinfo"
	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/retention"
	"github.com/apiwat/apw-agent/internal/transport"
)

type State struct {
	Build        buildinfo.Info `json:"build"`
	ConfigPath   string         `json:"config_path,omitempty"`
	Running      bool           `json:"running"`
	PID          int            `json:"pid"`
	SkillsLoaded int            `json:"skills_loaded"`
	LatencyMs    int64          `json:"latency_ms"`
	Workspace    string         `json:"workspace"`
	StartedAt    time.Time      `json:"started_at"`
	Heartbeat    time.Time      `json:"heartbeat"`
	Address      string         `json:"address"`
	Protocol     string         `json:"protocol"`
}
type stateFile struct {
	State
	Token string `json:"token"`
}

func stateFilePath(workspace string) string { return filepath.Join(workspace, ".apw", "daemon.json") }
func readState(root string) (stateFile, error) {
	data, err := os.ReadFile(stateFilePath(root))
	if err != nil {
		return stateFile{}, err
	}
	var st stateFile
	err = json.Unmarshal(data, &st)
	return st, err
}

func Status(workspace string) State {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return State{}
	}
	st, err := readState(abs)
	if err != nil {
		return State{Workspace: abs}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	var health State
	start := time.Now()
	err = transport.NewClient(st.Address, st.Token).Call(ctx, "GET", "/v1/health", nil, &health)
	if err != nil || health.Protocol != transport.Version || health.PID != st.PID || health.Workspace != st.Workspace {
		return State{Workspace: abs}
	}
	health.Running = true
	health.LatencyMs = time.Since(start).Milliseconds()
	return health
}

// A status read never starts a daemon or advertises an embedded fake process.
func EnsureRunning(workspace string) State { return Status(workspace) }
func Stop(workspace string) error {
	st, err := readState(workspace)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return transport.NewClient(st.Address, st.Token).Call(ctx, "POST", "/v1/shutdown", map[string]string{}, nil)
}
func Connect(ctx context.Context, cfgPath string) (*transport.Client, error) {
	cfg, absCfg, err := LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	if st := Status(cfg.Workspace); st.Running {
		staleBin := false
		if flag.Lookup("test.v") == nil && !st.StartedAt.IsZero() {
			if bin, bErr := daemonExecutable(); bErr == nil {
				if fi, sErr := os.Stat(bin); sErr == nil && fi.ModTime().After(st.StartedAt.Add(2*time.Second)) {
					staleBin = true
					_ = Stop(cfg.Workspace)
					for i := 0; i < 30; i++ {
						time.Sleep(100 * time.Millisecond)
						if !Status(cfg.Workspace).Running {
							break
						}
					}
				}
			}
		}
		if !staleBin && Status(cfg.Workspace).Running {
			if err := compatibleBuild(st); err != nil {
				return nil, err
			}
			if !sameConfig(st.ConfigPath, absCfg) {
				return nil, fmt.Errorf("daemon uses another or older configuration (%s); stop it for workspace %s before starting with %s", st.ConfigPath, cfg.Workspace, absCfg)
			}
			raw, err := readState(cfg.Workspace)
			if err != nil {
				return nil, err
			}
			return transport.NewClient(raw.Address, raw.Token), nil
		}
	}
	if flag.Lookup("test.v") != nil {
		return nil, errors.New("daemon is not running; tests must inject an application backend")
	}
	bin, err := daemonExecutable()
	if err != nil {
		return nil, errors.New("apwd is not installed; build/install cmd/apwd with cmd/apw")
	}
	cmd := exec.Command(bin, "--config", absCfg)
	cmd.Dir = cfg.Workspace
	configureBackground(cmd)
	if err := os.MkdirAll(filepath.Join(cfg.Workspace, ".apw"), 0700); err != nil {
		return nil, err
	}
	// No healthy daemon owns the log here. Rotation is best effort: a failure keeps appending.
	_, _ = retention.RotateLog(cfg.Workspace, retention.Resolve(cfg.Retention).LogMaxBytes, false)
	logFile, err := os.OpenFile(filepath.Join(cfg.Workspace, ".apw", "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	_ = cmd.Process.Release()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("daemon did not become healthy; inspect .apw/daemon.log")
		case <-ticker.C:
			if st := Status(cfg.Workspace); st.Running {
				if err := compatibleBuild(st); err != nil {
					return nil, err
				}
				if !sameConfig(st.ConfigPath, absCfg) {
					return nil, errors.New("another daemon started with a different config; select its config or stop it first")
				}
				raw, err := readState(cfg.Workspace)
				if err != nil {
					return nil, err
				}
				return transport.NewClient(raw.Address, raw.Token), nil
			}
		}
	}
}

func sameConfig(first, second string) bool {
	if first == "" {
		return false
	}
	if first == second {
		return true
	}
	a, e1 := os.Stat(first)
	b, e2 := os.Stat(second)
	return e1 == nil && e2 == nil && os.SameFile(a, b)
}
func LoadConfig(path string) (*config.Config, string, error) {
	if path == "" {
		path = "./apw.json"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.LoadOrDefault(abs)
	if err != nil {
		return nil, "", err
	}
	if !filepath.IsAbs(cfg.Workspace) {
		cfg.Workspace = filepath.Join(filepath.Dir(abs), cfg.Workspace)
	}
	cfg.Workspace, err = filepath.Abs(cfg.Workspace)
	return cfg, abs, err
}

// CountSkills counts available skills from user/project skill directories (plus built-in Triad skills).
func CountSkills(workspace string) int {
	groups := DiscoverSkills(workspace)
	total := 0
	for _, list := range groups {
		total += len(list)
	}
	return total
}

// DiscoverSkills returns available skills grouped by category.
func DiscoverSkills(workspace string) map[string][]string {
	groups := map[string][]string{
		"triad-core": {"architect-planner", "surgical-patcher", "reflex-verifier", "ast-symbol-inspector", "call-impact-graph"},
		"runtime":    {"sqlite-memory", "apwd-daemon", "telegram-gateway", "desktop-ui", "autonomy-24x7"},
	}

	scanRoot := func(category, root string) {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.EqualFold(d.Name(), "SKILL.md") {
				name := filepath.Base(filepath.Dir(path))
				if name != "" && name != "." && name != "skills" {
					groups[category] = append(groups[category], name)
				}
			}
			return nil
		})
	}
	if workspace != "" {
		scanRoot("workspace", filepath.Join(workspace, ".apw", "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		scanRoot("user", filepath.Join(home, ".apw", "skills"))
	}
	return groups
}

func compatibleBuild(st State) error {
	if buildinfo.SourceHash != "unrecorded" && st.Build.SourceHash != buildinfo.SourceHash {
		return errors.New("daemon build differs from CLI; stop the running daemon and restart with the matching apwd")
	}
	return nil
}
func daemonExecutable() (string, error) {
	if exe, err := os.Executable(); err == nil {
		name := "apwd"
		if strings.EqualFold(filepath.Ext(exe), ".exe") {
			name += ".exe"
		}
		sibling := filepath.Join(filepath.Dir(exe), name)
		if info, err := os.Stat(sibling); err == nil && info.Mode().IsRegular() {
			return sibling, nil
		}
	}
	return exec.LookPath("apwd")
}
