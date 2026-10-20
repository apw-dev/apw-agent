package surgeon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
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
	ext := strings.ToLower(filepath.Ext(rel))
	if ext != "" && ext != ".go" {
		if syms, ok := inspectPolyglotSymbols(ext, data); ok {
			return syms, nil
		}
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
				switch s := spec.(type) {
				case *ast.TypeSpec:
					add(s.Name.Name, "type", s)
				case *ast.ValueSpec:
					// Package-level variables and constants (for example exported sentinel errors).
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if name.Name != "_" {
							add(name.Name, kind, s)
						}
					}
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
		if p == ".git" || p == ".apw" || p == ".apw-sandbox" || p == ".worktrees" || p == ".env" || strings.HasPrefix(p, ".env.") || strings.HasSuffix(p, ".pem") || strings.HasSuffix(p, ".key") {
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

// allowedCustomizationPath permits reading only safe customization files (.apw/skills/*/SKILL.md,
// .apw/rules/*.md, .apw/hooks.json) while keeping .apw/auth.json, .apw/tasks.db, etc. inaccessible.
func allowedCustomizationPath(rel string) error {
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("%w: %q", ErrPathEscape, rel)
	}
	slash := filepath.ToSlash(filepath.Clean(rel))
	parts := strings.Split(slash, "/")
	switch {
	case len(parts) == 2 && parts[0] == ".apw" && parts[1] == "hooks.json":
		return nil
	case len(parts) == 3 && parts[0] == ".apw" && parts[1] == "rules" && strings.HasSuffix(strings.ToLower(parts[2]), ".md"):
		return nil
	case len(parts) == 4 && parts[0] == ".apw" && parts[1] == "skills" && parts[2] != "" && strings.EqualFold(parts[3], "SKILL.md"):
		return nil
	default:
		return fmt.Errorf("disallowed customization path %q", rel)
	}
}

// ReadAPWCustomization safely reads a customization file (.apw/skills/<slug>/SKILL.md,
// .apw/rules/<name>.md, or .apw/hooks.json) inside base using os.Root without following symlinks.
func ReadAPWCustomization(base, rel string) ([]byte, error) {
	if err := allowedCustomizationPath(rel); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cur := ""
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := r.Lstat(cur)
		if err != nil {
			return nil, err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: symbolic link %q", ErrPathEscape, cur)
		}
	}
	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > 256*1024 {
		return nil, fmt.Errorf("invalid customization file %s", rel)
	}
	return io.ReadAll(io.LimitReader(f, 256*1024))
}

// CreateAPWCustomization writes a new customization file (.apw/skills/<slug>/SKILL.md,
// .apw/rules/<name>.md, or .apw/hooks.json) inside base through os.Root. Missing parent
// directories are created; a link or non-directory at any component is rejected, and an
// existing file is never overwritten (the error wraps os.ErrExist).
func CreateAPWCustomization(base, rel string, data []byte) error {
	if err := allowedCustomizationPath(rel); err != nil {
		return err
	}
	r, err := os.OpenRoot(base)
	if err != nil {
		return err
	}
	defer r.Close()
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	cur := ""
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		last := i == len(parts)-1
		fi, err := r.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			if last {
				break
			}
			if err = r.Mkdir(cur, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return fmt.Errorf("%w: link %q", ErrPathEscape, cur)
		}
		if last {
			return fmt.Errorf("customization %s: %w", rel, os.ErrExist)
		}
		if !fi.IsDir() {
			return fmt.Errorf("customization parent %q is not a directory", cur)
		}
	}
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(rel), ".apw-"+hex.EncodeToString(nonce[:])+".tmp")
	f, err := r.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer r.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	if err = r.Link(tmp, rel); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("customization %s: %w", rel, os.ErrExist)
		}
		return err
	}
	return nil
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
	content = FormatGoIfValid(rel, content)
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

// FormatGoIfValid formats .go content with go/format.Source when it is syntactically valid,
// preserving CRLF line endings when present. If formatting fails (syntax error), content is
// returned unchanged so verification diagnostics can report the exact compiler error.
func FormatGoIfValid(rel, content string) string {
	if !strings.HasSuffix(strings.ToLower(rel), ".go") {
		return content
	}
	hasCRLF := strings.Contains(content, "\r\n")
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	formatted, err := format.Source([]byte(normalized))
	if err != nil {
		return content
	}
	out := string(formatted)
	if hasCRLF {
		out = toCRLF(out)
	}
	return out
}
