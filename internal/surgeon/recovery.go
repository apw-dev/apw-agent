package surgeon

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

// ExportDirtyWorktree exports tracked changes and untracked regular files, never applying them.
func ExportDirtyWorktree(ctx context.Context, path, destination string) (string, map[string]FileState, error) {
	headBefore, err := GetHeadCommit(ctx, path)
	if err != nil {
		return "", nil, err
	}
	files, err := GetDirtyFiles(ctx, path)
	if err != nil {
		return "", nil, err
	}
	states := map[string]FileState{}
	var args = []string{"diff", "--binary", "--no-ext-diff", "--no-textconv", "HEAD", "--"}
	argBytes, totalBytes := 0, 0
	if len(files) > 10000 {
		return "", nil, errors.New("too many recovery files")
	}
	for _, file := range files {
		if err := AllowedPath(file); err != nil {
			return "", nil, fmt.Errorf("cannot export protected dirty file %s: %w", file, err)
		}
		argBytes += len(file)
		if argBytes > 16000 {
			return "", nil, errors.New("recovery path list exceeds safe argument limit")
		}
		state, data, err := readState(path, file)
		if err != nil {
			return "", nil, err
		}
		totalBytes += len(data)
		if totalBytes > 128*1024*1024 {
			return "", nil, errors.New("recovery exceeds 128 MiB limit")
		}
		states[file] = state
		if state.Exists {
			if err := WriteInRoot(destination, file, data, state.Mode, true); err != nil {
				return "", nil, err
			}
		}
		args = append(args, file)
	}
	var diff []byte
	if len(files) > 0 {
		diff, err = gitOutput(ctx, path, args...)
		if err != nil {
			return "", nil, err
		}
	}
	for file, expected := range states {
		actual, err := StateInRoot(path, file)
		if err != nil || actual != expected {
			return "", nil, fmt.Errorf("worktree changed during export: %s", file)
		}
	}
	head, err := GetHeadCommit(ctx, path)
	if err != nil {
		return "", nil, err
	}
	if head != headBefore {
		return "", nil, errors.New("worktree HEAD changed during export")
	}
	if err := WriteFileAtomic(filepath.Join(filepath.Dir(destination), "changes.patch"), diff, 0600); err != nil {
		return "", nil, err
	}
	return head, states, nil
}
