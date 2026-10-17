package surgeon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrPathEscape is returned when a target path escapes the workspace root (implementation-plan M1.3).
var ErrPathEscape = errors.New("surgeon: path escapes workspace root")

func isPathEscapeErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrPathEscape) || strings.Contains(err.Error(), "path escapes")
}

// ResolveInRoot resolves rel inside root, verifying that it does not escape (implementation-plan M1.3).
func ResolveInRoot(root, rel string) (string, error) {
	if rel == "" || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: %q", ErrPathEscape, rel)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer r.Close()

	parentDir := filepath.Clean(filepath.Dir(rel))
	parentExists := true
	if parentDir != "." {
		var cur string
		for _, part := range strings.Split(parentDir, string(filepath.Separator)) {
			if part == "" || part == "." {
				continue
			}
			if cur == "" {
				cur = part
			} else {
				cur = filepath.Join(cur, part)
			}
			if _, err := r.Lstat(cur); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					parentExists = false
					break
				}
				if isPathEscapeErr(err) {
					return "", fmt.Errorf("%w: %w", ErrPathEscape, err)
				}
				return "", err
			}
		}
	}

	if parentExists {
		if _, err := r.Stat(rel); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				if isPathEscapeErr(err) {
					return "", fmt.Errorf("%w: %w", ErrPathEscape, err)
				}
				return "", err
			}
		}
	}
	return filepath.Join(rootAbs, rel), nil
}

// WriteFileAtomic safely writes data to path using a temporary file and atomic rename (implementation-plan M1.4).
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".apw-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}

	var renameErr error
	for attempt := 0; attempt < 4; attempt++ {
		renameErr = os.Rename(tmpName, path)
		if renameErr == nil {
			success = true
			return nil
		}
		if attempt < 3 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return renameErr
}
