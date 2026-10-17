package surgeon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func InspectWorkspaceSymbols(base, rel string) ([]SymbolSpan, error) {
	data, _, err := ReadInRoot(base, rel)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, rel, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var result []SymbolSpan
	add := func(name, kind string, node ast.Node) {
		result = append(result, SymbolSpan{Name: name, Kind: kind, StartLine: fset.Position(node.Pos()).Line, EndLine: fset.Position(node.End()).Line})
	}
	for _, decl := range tree.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "func"
			if d.Recv != nil {
				kind = "method"
			}
			add(d.Name.Name, kind, d)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if t, ok := spec.(*ast.TypeSpec); ok {
					add(t.Name.Name, "type", t)
				}
			}
		}
	}
	return result, nil
}

// AllowedPath excludes agent state, Git metadata, and common credential files.
func AllowedPath(rel string) error {
	if !filepath.IsLocal(rel) || filepath.Clean(rel) == "." {
		return fmt.Errorf("%w: %q", ErrPathEscape, rel)
	}
	for _, p := range strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/") {
		p = strings.ToLower(p)
		if p == ".git" || p == ".apw" || p == ".worktrees" || p == ".env" || strings.HasPrefix(p, ".env.") || strings.HasSuffix(p, ".pem") || strings.HasSuffix(p, ".key") {
			return fmt.Errorf("protected workspace path %q", rel)
		}
	}
	return nil
}

// openSafeRoot rejects links at every component and uses os.Root for the actual I/O.
// No absolute-path check is relied upon as the filesystem access boundary.
func openSafeRoot(base, rel string) (*os.Root, error) {
	if err := AllowedPath(rel); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	cur := ""
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := r.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			r.Close()
			return nil, err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			r.Close()
			return nil, fmt.Errorf("%w: symbolic link %q", ErrPathEscape, cur)
		}
		if _, err := r.Stat(cur); err != nil {
			r.Close()
			return nil, fmt.Errorf("%w: %v", ErrPathEscape, err)
		}
	}
	return r, nil
}

func ReadInRoot(base, rel string) ([]byte, os.FileMode, error) {
	r, err := openSafeRoot(base, rel)
	if err != nil {
		return nil, 0, err
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !fi.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("not a regular file: %s", rel)
	}
	if fi.Size() > 4*1024*1024 {
		return nil, 0, fmt.Errorf("file exceeds 4 MiB limit: %s", rel)
	}
	b, err := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
	if len(b) > 4*1024*1024 {
		return nil, 0, fmt.Errorf("file exceeds limit: %s", rel)
	}
	mode := fi.Mode().Perm()
	if runtime.GOOS == "windows" {
		if mode&0200 != 0 {
			mode = 0644
		} else {
			mode = 0444
		}
	}
	return b, mode, err
}

func WriteInRoot(base, rel string, data []byte, perm os.FileMode, exclusive bool) error {
	r, err := openSafeRoot(base, rel)
	if err != nil {
		return err
	}
	defer r.Close()
	if err = r.MkdirAll(filepath.Dir(rel), 0755); err != nil {
		return err
	}
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(rel), ".apw-"+hex.EncodeToString(nonce[:])+".tmp")
	f, err := r.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer r.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if exclusive {
		return r.Link(tmp, rel)
	}
	return r.Rename(tmp, rel)
}

func RemoveInRoot(base, rel string) error {
	r, err := openSafeRoot(base, rel)
	if err != nil {
		return err
	}
	defer r.Close()
	fi, err := r.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove non-file %s", rel)
	}
	return r.Remove(rel)
}

func (s *Surgeon) ReadWorkspaceFile(base, rel string) (string, string, error) {
	b, _, err := ReadInRoot(base, rel)
	if err != nil {
		return "", "", err
	}
	key, err := snapshotKey(filepath.Join(base, rel))
	if err != nil {
		return "", "", err
	}
	content := string(b)
	hash := CalculateHash(content)
	s.mu.Lock()
	s.snapshots[key] = FileSnapshot{Path: filepath.Join(base, rel), Content: content, Hash: hash, LineCount: strings.Count(content, "\n") + 1}
	s.mu.Unlock()
	return content, hash, nil
}

func (s *Surgeon) PatchWorkspaceFile(base, rel string, op PatchOperation, hooks ...func(FileState, FileState) error) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, err := snapshotKey(filepath.Join(base, rel))
	if err != nil {
		return "", err
	}
	snap, ok := s.snapshots[key]
	if !ok {
		return "", ErrReadRequired
	}
	b, perm, err := ReadInRoot(base, rel)
	if err != nil {
		return "", err
	}
	current := string(b)
	hash := CalculateHash(current)
	if snap.Hash != hash || (op.BaseHash != "" && op.BaseHash != hash) {
		return "", ErrContentDrift
	}
	if op.OldContent == "" {
		return "", ErrEmptyTarget
	}
	n := strings.Count(current, op.OldContent)
	if n == 0 && strings.Contains(current, "\r\n") {
		op.OldContent = toCRLF(op.OldContent)
		op.NewContent = toCRLF(op.NewContent)
		n = strings.Count(current, op.OldContent)
	}
	if n == 0 {
		return "", ErrTargetNotFound
	}
	if n > 1 && !op.ReplaceAll {
		return "", ErrMultipleMatches
	}
	count := 1
	if op.ReplaceAll {
		count = -1
	}
	content := strings.Replace(current, op.OldContent, op.NewContent, count)
	for _, hook := range hooks {
		if err := hook(FileState{Exists: true, Hash: hash, Mode: perm}, FileState{Exists: true, Hash: CalculateHash(content), Mode: perm}); err != nil {
			return "", err
		}
	}
	if err = WriteInRoot(base, rel, []byte(content), perm, false); err != nil {
		return "", err
	}
	hash = CalculateHash(content)
	s.snapshots[key] = FileSnapshot{Path: filepath.Join(base, rel), Content: content, Hash: hash, LineCount: strings.Count(content, "\n") + 1}
	return hash, nil
}
