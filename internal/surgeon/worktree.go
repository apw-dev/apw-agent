package surgeon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/apiwat/apw-agent/internal/process"
)

var (
	ErrNotGitRepo        = errors.New("worktree: directory is not a git repository root")
	ErrWorktreeExists    = errors.New("worktree: path or branch already exists")
	ErrWorkspaceConflict = errors.New("worktree: workspace changed since task snapshot")
)
var branchSanitizeRegex = regexp.MustCompile("[^a-zA-Z0-9_-]")

type FileState struct {
	Exists bool        `json:"exists"`
	Hash   string      `json:"hash,omitempty"`
	Mode   os.FileMode `json:"mode,omitempty"`
}
type Worktree struct {
	Workspace    string               `json:"workspace"`
	Path         string               `json:"path"`
	Branch       string               `json:"branch"`
	BaseCommit   string               `json:"base_commit"`
	TaskID       string               `json:"task_id"`
	Baseline     map[string]FileState `json:"baseline"`
	BaseContents map[string][]byte    `json:"base_contents"`
	mu           sync.Mutex
	cleanedUp    bool
}
type WorktreeOptions struct {
	TaskID, BranchPrefix string
	SyncDirtyFiles       bool
	Timeout              time.Duration
}

var (
	sharedHooksMu  sync.Mutex
	sharedHooksDir string
)

// emptyHooksDir returns an empty directory (created by this process with
// os.MkdirTemp) to use as core.hooksPath. The directory is shared process-wide
// and recreated if something (for example a temp cleaner) removed it.
func emptyHooksDir() (string, error) {
	sharedHooksMu.Lock()
	defer sharedHooksMu.Unlock()
	if sharedHooksDir != "" {
		if info, err := os.Stat(sharedHooksDir); err == nil && info.IsDir() {
			return sharedHooksDir, nil
		}
		sharedHooksDir = ""
	}
	dir, err := os.MkdirTemp("", "apw-git-hooks-")
	if err != nil {
		return "", err
	}
	sharedHooksDir = dir
	return dir, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	hooks, err := emptyHooksDir()
	if err != nil {
		return nil, err
	}
	safeArgs := append([]string{"-c", "core.hooksPath=" + hooks, "-c", "core.fsmonitor=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", safeArgs...)
	cmd.Dir = dir
	var stderr process.Buffer
	stdout := process.Buffer{Limit: 4 * 1024 * 1024}
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	err = process.Run(ctx, cmd)
	if stdout.Truncated() || stderr.Truncated() {
		return nil, errors.New("git output exceeds limit; refusing incomplete workspace inventory")
	}
	out := []byte(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), stderr.String(), err)
	}
	return out, nil
}
func IsGitRepo(workspace string) bool {
	out, err := gitOutput(context.Background(), workspace, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}
func GetHeadCommit(ctx context.Context, workspace string) (string, error) {
	out, err := gitOutput(ctx, workspace, "rev-parse", "HEAD")
	return strings.TrimSpace(string(out)), err
}

func shouldSkipSync(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "vendor" || part == "node_modules" {
			return true
		}
	}
	return AllowedPath(rel) != nil
}
func workspaceFiles(ctx context.Context, dir string) ([]string, error) {
	out, err := gitOutput(ctx, dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	head, err := gitOutput(ctx, dir, "ls-tree", "-r", "-z", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	out = append(out, head...)
	seen := map[string]bool{}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		p = filepath.Clean(filepath.FromSlash(p))
		if !seen[p] && !shouldSkipSync(p) {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Porcelain -z handles renames, deleted paths, spaces, Unicode, and individual untracked files.
func GetDirtyFiles(ctx context.Context, workspace string) ([]string, error) {
	out, err := gitOutput(ctx, workspace, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	entries := strings.Split(string(out), "\x00")
	seen := map[string]bool{}
	var result []string
	add := func(p string) {
		p = filepath.FromSlash(p)
		if p != "" && !seen[p] && !shouldSkipSync(p) {
			seen[p] = true
			result = append(result, p)
		}
	}
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		add(entry[3:])
		if strings.ContainsAny(entry[:2], "RC") && i+1 < len(entries) {
			i++
			add(entries[i])
		}
	}
	sort.Strings(result)
	return result, nil
}
func readState(base, rel string) (FileState, []byte, error) {
	data, mode, err := ReadInRoot(base, rel)
	if errors.Is(err, os.ErrNotExist) {
		return FileState{}, nil, nil
	}
	if err != nil {
		return FileState{}, nil, err
	}
	return FileState{Exists: true, Hash: CalculateHash(string(data)), Mode: mode}, data, nil
}

func StateInRoot(base, rel string) (FileState, error) {
	state, _, err := readState(base, rel)
	return state, err
}
func CreateWorktree(ctx context.Context, workspace string, opts WorktreeOptions) (*Worktree, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	top, err := gitOutput(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNotGitRepo
	}
	a, e1 := os.Stat(abs)
	b, e2 := os.Stat(strings.TrimSpace(string(top)))
	if e1 != nil || e2 != nil || !os.SameFile(a, b) {
		return nil, ErrNotGitRepo
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	head, err := GetHeadCommit(ctx, abs)
	if err != nil {
		return nil, fmt.Errorf("repository needs an initial commit: %w", err)
	}
	task := opts.TaskID
	if task == "" {
		task = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	id := branchSanitizeRegex.ReplaceAllString(task, "-")
	if id == "" || len(id) > 120 {
		return nil, errors.New("invalid worktree task ID")
	}
	prefix := opts.BranchPrefix
	if prefix == "" {
		prefix = "apw/"
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := root.Mkdir(".worktrees", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	stateDir, err := root.Lstat(".worktrees")
	if err != nil {
		return nil, err
	}
	if !stateDir.IsDir() || stateDir.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("worktree directory must not be a link")
	}
	path := filepath.Join(abs, ".worktrees", id)
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return nil, ErrWorktreeExists
	}
	ensureGitExclude(abs, ".worktrees/")
	ensureGitExclude(abs, ".apw/")
	paths, err := workspaceFiles(ctx, abs)
	if err != nil {
		return nil, err
	}
	baseline := map[string]FileState{}
	contents := map[string][]byte{}
	snapshotBytes := 0
	for _, p := range paths {
		state, data, err := readState(abs, p)
		snapshotBytes += len(data)
		if snapshotBytes > 128*1024*1024 || len(paths) > 10000 {
			return nil, errors.New("workspace snapshot exceeds limits")
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", p, err)
		}
		baseline[p] = state
		contents[p] = data
	}
	wt := &Worktree{Workspace: abs, Path: path, Branch: prefix + id, BaseCommit: head, TaskID: task, Baseline: baseline, BaseContents: contents}
	var addArgs []string
	filterKeys, _ := gitOutput(ctx, abs, "config", "--name-only", "--get-regexp", "^filter\\..*\\.(process|smudge|clean|required)$")
	for _, key := range strings.Split(strings.TrimSpace(string(filterKeys)), "\n") {
		if key == "" {
			continue
		}
		value := ""
		if strings.HasSuffix(key, ".required") {
			value = "false"
		}
		addArgs = append(addArgs, "-c", key+"="+value)
	}
	addArgs = append(addArgs, "worktree", "add", "-b", wt.Branch, path, "HEAD")
	if _, err = gitOutput(ctx, abs, addArgs...); err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = wt.Cleanup(context.Background())
		}
	}()
	if opts.SyncDirtyFiles {
		for _, p := range paths {
			state := baseline[p]
			if state.Exists {
				err = WriteInRoot(path, p, contents[p], state.Mode, false)
			} else {
				err = RemoveInRoot(path, p)
			}
			if err != nil {
				return nil, fmt.Errorf("sync %s: %w", p, err)
			}
			now, _, err := readState(abs, p)
			if err != nil || now != state {
				return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, p)
			}
		}
	}
	if err = wt.SaveManifest(); err != nil {
		return nil, err
	}
	ok = true
	return wt, nil
}
func (w *Worktree) SaveManifest() error {
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(w.Path, ".apw", "worktree.json"), data, 0600)
}
func LoadWorktree(path string) (*Worktree, error) {
	data, err := os.ReadFile(filepath.Join(path, ".apw", "worktree.json"))
	if err != nil {
		return nil, fmt.Errorf("worktree snapshot manifest missing; refusing unsafe apply: %w", err)
	}
	var w Worktree
	if err = json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	actual, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	recorded, err := filepath.Abs(w.Path)
	if err != nil || actual != recorded {
		return nil, errors.New("worktree manifest path mismatch")
	}
	if w.Baseline == nil {
		return nil, errors.New("worktree baseline missing")
	}
	return &w, nil
}

// ChangedFiles compares actual worktree files with the dirty source snapshot.
func (w *Worktree) ChangedFiles(ctx context.Context) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	paths, err := workspaceFiles(ctx, w.Path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, path := range paths {
		seen[path] = true
	}
	for path := range w.Baseline {
		seen[path] = true
	}
	var changed []string
	for path := range seen {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current, err := StateInRoot(w.Path, path)
		if err != nil {
			return nil, err
		}
		if current != w.Baseline[path] {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

func (w *Worktree) Diff(ctx context.Context) (string, error) {
	paths, err := workspaceFiles(ctx, w.Path)
	if err != nil {
		return "", err
	}
	for path := range w.Baseline {
		paths = append(paths, path)
	}
	return w.DiffFiles(ctx, paths)
}
func (w *Worktree) DiffFiles(ctx context.Context, paths []string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cleanedUp {
		return "", errors.New("worktree already cleaned up")
	}
	sorted := append([]string{}, paths...)
	sort.Strings(sorted)
	seen := map[string]bool{}
	var b strings.Builder
	for _, path := range sorted {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		after, data, err := readState(w.Path, path)
		if err != nil {
			return "", err
		}
		before := w.Baseline[path]
		if before == after {
			continue
		}
		original := w.BaseContents[path]
		if before.Exists && CalculateHash(string(original)) != before.Hash {
			return "", errors.New("baseline contents unavailable for diff")
		}
		if !before.Exists {
			original = nil
		}
		fileDiff := renderFileDiff(filepath.ToSlash(path), before, after, original, data)
		if b.Len()+len(fileDiff) > maxDiffBytes {
			b.WriteString("\n[diff truncated]\n")
			break
		}
		b.WriteString(fileDiff)
	}
	return b.String(), nil
}

func (w *Worktree) Cleanup(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cleanedUp {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Git removes only a registered worktree; never recursively delete an unchecked path.
	if _, err := gitOutput(ctx, w.Workspace, "worktree", "remove", "--force", w.Path); err != nil {
		return err
	}
	if w.Branch != "" {
		if _, err := gitOutput(ctx, w.Workspace, "branch", "-D", w.Branch); err != nil {
			return err
		}
	}
	w.cleanedUp = true
	return nil
}
func copyFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return WriteFileAtomic(dst, data, fi.Mode().Perm())
}
func ensureGitExclude(workspace, pattern string) {
	out, err := gitOutput(context.Background(), workspace, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	data, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pattern {
			return
		}
	}
	_ = WriteFileAtomic(path, append(data, []byte("\n"+pattern+"\n")...), 0644)
}
