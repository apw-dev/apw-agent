//go:build unix

package surgeon

import (
	"fmt"
	"os"
	"syscall"
)

func restrictDir(path string) error {
	return os.Chmod(path, 0700)
}

// checkDirOwner refuses an existing APW-owned directory owned by another user or writable by others.
func checkDirOwner(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("worktree directory %s is owned by another user (uid %d); remove it or choose another worktree_root", path, st.Uid)
	}
	if fi.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("worktree directory %s is writable by other users; run chmod go-w on it", path)
	}
	return nil
}
