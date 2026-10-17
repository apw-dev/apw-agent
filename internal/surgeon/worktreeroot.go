package surgeon

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Task worktrees live outside the workspace so that project tools run from the workspace (Vitest, Jest,
// `eslint .`) never discover APW's copies, and so that worktree paths stay short on Windows:
//
//	<base>/<workspace-key>/<task-dir>
//
// base is apw.json worktree_root, else $APW_WORKTREE_ROOT, else <user cache dir>/apw/wt
// (%LOCALAPPDATA%\apw\wt on Windows, $XDG_CACHE_HOME/apw/wt or ~/.cache/apw/wt on Linux).
// Worktrees created before this layout live in <workspace>/.worktrees/<task-id> and are still found,
// resumed, applied and cleaned up there; they are never moved.

// WorktreeRootEnv overrides the default worktree base when apw.json has no worktree_root.
const WorktreeRootEnv = "APW_WORKTREE_ROOT"

// LegacyWorktreeDir is the in-workspace directory that held task worktrees before the external root.
const LegacyWorktreeDir = ".worktrees"

var hexTaskID = regexp.MustCompile(`^task-([0-9a-f]{32})$`)

// TaskWorktreeName returns the short directory name of a task worktree. Generated IDs (task-<32 hex>)
// use their first 12 hex digits; short IDs are kept; anything else is hashed. The branch keeps the full ID.
func TaskWorktreeName(taskID string) string {
	if m := hexTaskID.FindStringSubmatch(taskID); m != nil {
		return m[1][:12]
	}
	if id := branchSanitizeRegex.ReplaceAllString(taskID, "-"); id != "" && len(id) <= 16 {
		return id
	}
	sum := sha256.Sum256([]byte(taskID))
	return hex.EncodeToString(sum[:6])
}

// worktreeRoot is a directory that may hold task worktrees: a parent APW does not create (the user cache
// directory, the parent of a configured root, or the workspace) plus the directories APW creates below
// it. Every APW-owned component must be a plain directory; links and junctions are refused.
type worktreeRoot struct {
	parent string
	owned  []string
	// external roots are created with user-only permissions and must be owned by the current user.
	external bool
}

func (r worktreeRoot) path() string {
	return filepath.Join(append([]string{r.parent}, r.owned...)...)
}

func plainDir(fi os.FileInfo) bool {
	return fi.IsDir() && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

// workspaceKey identifies a workspace by its resolved, case-folded (Windows) absolute path.
func workspaceKey(workspace string) string {
	p, err := filepath.Abs(workspace)
	if err != nil {
		p = workspace
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:4])
}

func externalRoot(workspace, base string) (worktreeRoot, error) {
	if !filepath.IsAbs(base) {
		return worktreeRoot{}, fmt.Errorf("worktree root %q must be an absolute path", base)
	}
	base = filepath.Clean(base)
	parent := filepath.Dir(base)
	if parent == base {
		return worktreeRoot{}, fmt.Errorf("worktree root %q must not be a volume or file-system root", base)
	}
	r := worktreeRoot{parent: parent, owned: []string{filepath.Base(base), workspaceKey(workspace)}, external: true}
	if within(workspace, r.path()) {
		return worktreeRoot{}, fmt.Errorf("worktree root %s is inside the workspace; choose a directory outside it", base)
	}
	return r, nil
}

func defaultRoot(workspace string) (worktreeRoot, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return worktreeRoot{}, fmt.Errorf("locate the user cache directory for task worktrees (set worktree_root in apw.json): %w", err)
	}
	r, err := externalRoot(workspace, filepath.Join(cache, "apw", "wt"))
	if err != nil {
		return r, err
	}
	// The cache directory itself is the trusted parent; apw and wt are APW-owned.
	r.parent, r.owned = cache, append([]string{"apw"}, r.owned...)
	return r, nil
}

// currentRoot is where new task worktrees for workspace are created.
func currentRoot(workspace, override string) (worktreeRoot, error) {
	if override != "" {
		return externalRoot(workspace, override)
	}
	if env := os.Getenv(WorktreeRootEnv); env != "" {
		return externalRoot(workspace, env)
	}
	return defaultRoot(workspace)
}

// knownRoots lists every root that may hold this workspace's task worktrees, newest layout first:
// the configured root, $APW_WORKTREE_ROOT, the default root and the legacy in-workspace directory.
// Roots that do not resolve (for example an invalid override) are skipped.
func knownRoots(workspace, override string) []worktreeRoot {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		abs = workspace
	}
	var roots []worktreeRoot
	add := func(r worktreeRoot, err error) {
		if err != nil {
			return
		}
		for _, have := range roots {
			if samePath(have.path(), r.path()) {
				return
			}
		}
		roots = append(roots, r)
	}
	if override != "" {
		add(externalRoot(abs, override))
	}
	if env := os.Getenv(WorktreeRootEnv); env != "" {
		add(externalRoot(abs, env))
	}
	add(defaultRoot(abs))
	add(worktreeRoot{parent: abs, owned: []string{LegacyWorktreeDir}}, nil)
	return roots
}

// verify checks that every APW-owned component exists and is a plain directory owned by this user.
func (r worktreeRoot) verify() error {
	cur := r.parent
	for _, part := range r.owned {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if !plainDir(fi) {
			return fmt.Errorf("worktree directory %s must be a plain directory, not a link or junction", cur)
		}
		if r.external {
			if err := checkDirOwner(cur); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensure creates the APW-owned components with user-only permissions and verifies them.
func (r worktreeRoot) ensure() error {
	if err := os.MkdirAll(r.parent, 0700); err != nil {
		return fmt.Errorf("create worktree base %s: %w", r.parent, err)
	}
	cur := r.parent
	for _, part := range r.owned {
		cur = filepath.Join(cur, part)
		err := os.Mkdir(cur, 0700)
		created := err == nil
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		fi, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if !plainDir(fi) {
			return fmt.Errorf("worktree directory %s must be a plain directory, not a link or junction", cur)
		}
		switch {
		case created && r.external:
			if err := restrictDir(cur); err != nil {
				return fmt.Errorf("restrict worktree directory %s: %w", cur, err)
			}
		case r.external:
			if err := checkDirOwner(cur); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureWorktreeDir creates (if needed) and returns the directory that receives new task worktrees.
func ensureWorktreeDir(workspace, override string) (string, error) {
	r, err := currentRoot(workspace, override)
	if err != nil {
		return "", err
	}
	if err := r.ensure(); err != nil {
		return "", err
	}
	return r.path(), nil
}

// WorktreeDir returns the directory that receives new task worktrees for workspace, without creating it.
func WorktreeDir(workspace, override string) (string, error) {
	r, err := currentRoot(workspace, override)
	if err != nil {
		return "", err
	}
	return r.path(), nil
}

// WorktreeDirs returns the existing, verified directories that may hold this workspace's task
// worktrees: the external roots and the legacy .worktrees directory.
func WorktreeDirs(workspace, override string) []string {
	var dirs []string
	for _, r := range knownRoots(workspace, override) {
		if r.verify() == nil {
			dirs = append(dirs, r.path())
		}
	}
	return dirs
}

// LocateWorktree checks that path is a direct child of one of the workspace's worktree directories,
// that no APW-owned component on the way is a link or junction, and that path itself (if present) is
// a plain directory. It returns the containing directory and the worktree's directory name.
func LocateWorktree(workspace, override, path string) (string, string, error) {
	target, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	roots := knownRoots(workspace, override)
	var tried []string
	for _, r := range roots {
		dir := r.path()
		tried = append(tried, dir)
		if !directChild(dir, target) {
			continue
		}
		if err := r.verify(); err != nil {
			return "", "", err
		}
		name := filepath.Base(target)
		fi, err := os.Lstat(filepath.Join(dir, name))
		if err == nil && !plainDir(fi) {
			return "", "", fmt.Errorf("worktree %s must be a plain directory, not a link or junction", path)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
		return dir, name, nil
	}
	return "", "", fmt.Errorf("worktree %s is not directly inside a task worktree directory (%s)", path, strings.Join(tried, ", "))
}

// directChild reports whether target is an immediate child of dir, lexically or after resolving links
// in both (a workspace opened through a different but equivalent path).
func directChild(dir, target string) bool {
	child := func(dir, target string) bool {
		rel, err := filepath.Rel(dir, target)
		return err == nil && rel != "." && filepath.IsLocal(rel) && filepath.Base(rel) == rel
	}
	if child(dir, target) {
		return true
	}
	realDir, e1 := filepath.EvalSymlinks(dir)
	realParent, e2 := filepath.EvalSymlinks(filepath.Dir(target))
	return e1 == nil && e2 == nil && samePath(realDir, realParent) && child(dir, filepath.Join(dir, filepath.Base(target)))
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// within reports whether path is workspace or inside it (lexically or after resolving links).
func within(workspace, path string) bool {
	inside := func(base, p string) bool {
		rel, err := filepath.Rel(base, p)
		return err == nil && (rel == "." || filepath.IsLocal(rel))
	}
	ws, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	if inside(ws, path) {
		return true
	}
	realWS, err := filepath.EvalSymlinks(ws)
	if err != nil {
		return false
	}
	// Resolve the deepest existing ancestor of path; the rest does not exist yet.
	existing, rest := path, ""
	for {
		if real, err := filepath.EvalSymlinks(existing); err == nil {
			return inside(realWS, filepath.Join(real, rest))
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return false
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
}
