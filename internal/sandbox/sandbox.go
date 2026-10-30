// Package sandbox isolates the repository commands Reflex runs during verification (dependency
// install, lint, build, test). The Git worktree already keeps edits away from the source workspace;
// a sandbox additionally limits what hostile or buggy project code can do to the rest of the machine.
//
// Modes:
//   - off: commands run as the current user with the full environment (previous behavior).
//   - restricted: Windows runs each command with a low-integrity copy of the user's token inside a
//     job object with UI and optional memory limits; it can write only the worktree (labeled low
//     integrity) and its .apw-sandbox caches, never the source workspace, the main repository's .git,
//     the worktree manifest or the user's files. Network access is not blocked on Windows. Linux uses
//     bubblewrap: a read-only root, a writable worktree, a private /tmp and no network except for
//     dependency install.
//   - docker: each stage runs in a fresh container that sees only the worktree, with all
//     capabilities dropped and no network except for dependency install.
//
// Restricted and docker modes pass a scrubbed environment: variables that look like credentials
// (KEY, TOKEN, SECRET, PASSWORD, ...) and APW's own variables are removed unless listed in pass_env.
package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/apiwat/apw-agent/internal/config"
	"github.com/apiwat/apw-agent/internal/process"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

const (
	ModeOff        = "off"
	ModeRestricted = "restricted"
	ModeDocker     = "docker"

	NetworkInstall = "install"
	NetworkNone    = "none"
	NetworkAll     = "all"

	// containerRoot is where docker mode mounts the worktree.
	containerRoot = "/workspace"
)

// Policy is a resolved sandbox configuration.
type Policy struct {
	Mode     string
	Image    string
	Network  string
	MemoryMB int
	PassEnv  []string
}

// FromConfig resolves defaults: mode off, network install.
func FromConfig(c config.SandboxConfig) Policy {
	p := Policy{Mode: c.Mode, Image: c.Image, Network: c.Network, MemoryMB: c.MemoryMB, PassEnv: append([]string{}, c.PassEnv...)}
	if p.Mode == "" {
		p.Mode = ModeOff
	}
	if p.Network == "" {
		p.Network = NetworkInstall
	}
	return p
}

// Active reports whether the policy isolates commands.
func (p Policy) Active() bool { return p.Mode == ModeRestricted || p.Mode == ModeDocker }

// Spec is one command to run.
type Spec struct {
	Args []string
	// Env adds NAME=value entries on top of the (scrubbed) environment.
	Env []string
	// Install marks dependency installation, the only stage that gets network under "install".
	Install bool
	// Optional exits successfully when the tool is not installed (docker checks inside the image).
	Optional bool
}

// Prepared is a command ready for process.RunLimited. Cleanup must run after the process exits.
type Prepared struct {
	Cmd     *exec.Cmd
	Limits  process.Limits
	Cleanup func()
}

// Session prepares one worktree for sandboxed commands.
type Session struct {
	policy   Policy
	root     string
	cache    string
	platform *platformState
}

// Open prepares root (an absolute worktree path) for the policy. For restricted mode on Windows it
// labels the worktree low integrity once and keeps .git and .apw at medium integrity.
func Open(p Policy, root string) (*Session, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if p.Mode == "" {
		p.Mode = ModeOff
	}
	if p.Network == "" {
		p.Network = NetworkInstall
	}
	s := &Session{policy: p, root: abs, cache: filepath.Join(abs, surgeon.SandboxDir)}
	switch p.Mode {
	case ModeOff:
		return s, nil
	case ModeDocker:
		if p.Image == "" {
			return nil, errors.New("sandbox: docker mode requires sandbox.image")
		}
		if _, err := exec.LookPath("docker"); err != nil {
			return nil, errors.New("sandbox: docker mode requires the docker CLI on PATH and a running Docker engine")
		}
	case ModeRestricted:
	default:
		return nil, fmt.Errorf("sandbox: unknown mode %q", p.Mode)
	}
	if err := s.makeCacheDirs(); err != nil {
		return nil, err
	}
	surgeon.EnsureGitExclude(abs, surgeon.SandboxDir+"/")
	if p.Mode == ModeRestricted {
		state, err := openPlatform(abs, s.cache)
		if err != nil {
			return nil, err
		}
		s.platform = state
	}
	return s, nil
}

// Close releases platform resources (the restricted token).
func (s *Session) Close() {
	if s != nil && s.platform != nil {
		s.platform.close()
		s.platform = nil
	}
}

// Describe summarizes the isolation for reports.
func (s *Session) Describe() string {
	if s == nil {
		return "off"
	}
	switch s.policy.Mode {
	case ModeRestricted:
		return restrictedDescription + "; network: " + s.policy.Network
	case ModeDocker:
		return "docker (" + s.policy.Image + ", worktree-only mount, no capabilities); network: " + s.policy.Network
	}
	return "off"
}

func (s *Session) network(spec Spec) bool {
	switch s.policy.Network {
	case NetworkAll:
		return true
	case NetworkNone:
		return false
	}
	return spec.Install
}

func (s *Session) limits() process.Limits {
	l := process.Limits{RestrictUI: s.policy.Mode == ModeRestricted}
	if s.policy.MemoryMB > 0 {
		l.MemoryBytes = uint64(s.policy.MemoryMB) << 20
	}
	return l
}

// Command builds the command for spec under the session's policy, with Dir set to the worktree.
func (s *Session) Command(ctx context.Context, spec Spec) (*Prepared, error) {
	if len(spec.Args) == 0 {
		return nil, errors.New("sandbox: empty command")
	}
	if s == nil || s.policy.Mode == ModeOff {
		cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
		if s != nil {
			cmd.Dir = s.root
		}
		cmd.Env = append(os.Environ(), spec.Env...)
		return &Prepared{Cmd: cmd, Cleanup: func() {}}, nil
	}
	if s.policy.Mode == ModeDocker {
		return s.dockerCommand(ctx, spec)
	}
	return s.restrictedCommand(ctx, spec)
}

// cacheEnv redirects tool caches and temp files into dir (the sandbox directory as the sandboxed
// process sees it) so tools never need to write outside the worktree.
func cacheEnv(dir string, join func(...string) string) []string {
	return []string{
		"TEMP=" + join(dir, "tmp"), "TMP=" + join(dir, "tmp"), "TMPDIR=" + join(dir, "tmp"),
		"GOCACHE=" + join(dir, "go-build"), "GOTMPDIR=" + join(dir, "tmp"),
		"npm_config_cache=" + join(dir, "npm"), "npm_config_update_notifier=false", "YARN_CACHE_FOLDER=" + join(dir, "yarn"),
		"BUN_INSTALL_CACHE_DIR=" + join(dir, "bun"), "PIP_CACHE_DIR=" + join(dir, "pip"),
		"CARGO_HOME=" + join(dir, "cargo"),
		"XDG_CACHE_HOME=" + join(dir, "cache"), "XDG_STATE_HOME=" + join(dir, "state"), "XDG_DATA_HOME=" + join(dir, "data"),
	}
}

var cacheDirs = []string{"tmp", "go-build", "npm", "pnpm-store", "yarn", "bun", "pip", "cargo", "cache", "state", "data", "local"}

func (s *Session) makeCacheDirs() error {
	for _, d := range cacheDirs {
		if err := os.MkdirAll(filepath.Join(s.cache, d), 0700); err != nil {
			return fmt.Errorf("sandbox: create cache directory: %w", err)
		}
	}
	return nil
}

var sensitiveWords = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH", "COOKIE", "SESSION", "PRIVATE"}

// sensitive reports whether an environment variable name looks like a credential or APW state.
func sensitive(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "APW_") {
		return true
	}
	for _, w := range sensitiveWords {
		if strings.Contains(upper, w) {
			return true
		}
	}
	return false
}

// ScrubEnv removes credential-like variables from environ unless their name is in pass.
// Later entries override earlier ones with the same name (case-insensitive on Windows).
func ScrubEnv(environ, pass []string) []string {
	allowed := map[string]bool{}
	for _, name := range pass {
		allowed[envKey(name)] = true
	}
	var out []string
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if sensitive(name) && !allowed[envKey(name)] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func envKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}

// mergeEnv returns base with overrides applied (override wins), sorted for stable commands.
func mergeEnv(base []string, overrides ...[]string) []string {
	values := map[string]string{}
	names := map[string]string{}
	set := func(kv string) {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			return
		}
		values[envKey(name)] = value
		names[envKey(name)] = name
	}
	for _, kv := range base {
		set(kv)
	}
	for _, list := range overrides {
		for _, kv := range list {
			set(kv)
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, names[k]+"="+values[k])
	}
	return out
}

func (s *Session) dockerCommand(ctx context.Context, spec Spec) (*Prepared, error) {
	name := "apw-verify-" + randomHex(6)
	network := "none"
	if s.network(spec) {
		network = "bridge"
	}
	cacheDir := containerRoot + "/" + surgeon.SandboxDir
	slash := func(parts ...string) string { return strings.Join(parts, "/") }
	args := []string{"run", "--rm", "--init", "--name", name, "--network", network,
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "2048",
		"-v", s.root + ":" + containerRoot, "-w", containerRoot}
	if s.policy.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", s.policy.MemoryMB))
	}
	if uid, gid := os.Getuid(), os.Getgid(); runtime.GOOS == "linux" && uid >= 0 {
		args = append(args, "--user", fmt.Sprintf("%d:%d", uid, gid))
	}
	// Only the stage environment, the cache redirects and explicitly passed variables enter the
	// container; the host environment (and its credentials) does not.
	env := mergeEnv([]string{"HOME=" + slash(cacheDir, "tmp"), "CI=true"}, cacheEnv(cacheDir, slash), toolEnv(spec.Args[0], cacheDir, slash), ScrubEnv(spec.Env, s.policy.PassEnv), passed(s.policy.PassEnv))
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	args = append(args, s.policy.Image)
	if spec.Optional {
		args = append(args, "sh", "-c", `command -v "$0" >/dev/null 2>&1 || { echo "$0 is not installed in the image; skipped"; exit 0; }; exec "$0" "$@"`)
	}
	args = append(args, spec.Args...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = s.root
	cmd.Env = os.Environ() // the docker CLI itself needs DOCKER_HOST and friends
	cleanup := func() {
		// Killing the CLI does not stop the container; remove it explicitly (no-op once --rm ran).
		rm := exec.Command("docker", "rm", "-f", name)
		process.HideWindow(rm)
		_ = rm.Run()
	}
	return &Prepared{Cmd: cmd, Limits: process.Limits{}, Cleanup: cleanup}, nil
}

// isPnpm reports whether a command is pnpm (pnpm, pnpm.cmd, pnpm.exe or a path to one).
func isPnpm(command string) bool {
	base := strings.ToLower(filepath.Base(command))
	return strings.TrimSuffix(strings.TrimSuffix(base, ".cmd"), ".exe") == "pnpm"
}

// toolEnv adds settings that only one tool understands (npm warns about unknown keys).
func toolEnv(command, dir string, join func(...string) string) []string {
	if isPnpm(command) {
		return []string{"npm_config_store_dir=" + join(dir, "pnpm-store")}
	}
	return nil
}

// passed returns NAME=value for each pass_env variable present in the host environment.
func passed(names []string) []string {
	var out []string
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+value)
		}
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
