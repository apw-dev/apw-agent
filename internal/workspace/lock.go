package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Lock is an OS-held lock: a crashed process releases it without stale PID guessing.
type Lock struct{ file *os.File }

func Acquire(root, name string) (*Lock, error) {
	if name == "" || filepath.Base(name) != name || !filepath.IsLocal(name) || name == "." {
		return nil, errors.New("invalid lock name")
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if err := r.Mkdir(".apw", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := r.Lstat(".apw")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("workspace state directory must not be a link")
	}
	rel := filepath.Join(".apw", name+".lock")
	if info, err := r.Lstat(rel); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("workspace lock must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := r.OpenFile(rel, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("workspace %s is busy: %w", name, err)
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
