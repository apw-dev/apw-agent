package surgeon

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	ErrReadRequired    = errors.New("surgeon: file must be read before editing (read-before-edit)")
	ErrContentDrift    = errors.New("surgeon: file content has drifted on disk; base hash mismatch")
	ErrTargetNotFound  = errors.New("surgeon: target string not found in file")
	ErrMultipleMatches = errors.New("surgeon: target string found multiple times; include more surrounding context for uniqueness")
	ErrEmptyTarget     = errors.New("surgeon: old_content must not be empty")
)

// FileSnapshot tracks file state for the read-before-write safety rule
type FileSnapshot struct {
	Path      string
	Content   string
	Hash      string
	LineCount int
}

// PatchOperation defines a surgical targeted edit
type PatchOperation struct {
	Path       string `json:"path"`
	BaseHash   string `json:"base_hash,omitempty"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// SymbolSpan represents a code symbol identified via AST
type SymbolSpan struct {
	Name      string
	Kind      string // func, struct, interface, type, var
	StartLine int
	EndLine   int
}

// Surgeon handles precision code surgery
type Surgeon struct {
	mu        sync.RWMutex
	snapshots map[string]FileSnapshot
}

// NewSurgeon creates a new Surgeon instance
func NewSurgeon() *Surgeon {
	return &Surgeon{
		snapshots: make(map[string]FileSnapshot),
	}
}

// CalculateHash computes SHA-256 hash of content
func CalculateHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func snapshotKey(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs, nil
}

func toCRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// ReadFile reads a file from disk, records snapshot in memory, and returns content.
// If the path targets a directory, it gracefully returns a directory listing instead of failing.
func (s *Surgeon) ReadFile(path string) (string, string, error) {
	key, err := snapshotKey(path)
	if err != nil {
		return "", "", err
	}

	fi, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to stat file: %w", err)
	}

	var content string
	var lineCount int

	if fi.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", "", fmt.Errorf("failed to read directory: %w", err)
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Directory listing for %s:\n", path))
		for _, e := range entries {
			typeStr := "file"
			if e.IsDir() {
				typeStr = "dir"
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", typeStr, e.Name()))
		}
		content = sb.String()
		lineCount = len(entries)
	} else {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("failed to read file: %w", err)
		}
		content = string(data)
		lineCount = len(strings.Split(content, "\n"))
	}

	hash := CalculateHash(content)

	s.mu.Lock()
	s.snapshots[key] = FileSnapshot{
		Path:      path,
		Content:   content,
		Hash:      hash,
		LineCount: lineCount,
	}
	s.mu.Unlock()

	return content, hash, nil
}

// ApplyPatch performs a surgical, targeted replacement: the file must have been read, the old text must match exactly once, and line endings are preserved
func (s *Surgeon) ApplyPatch(op PatchOperation) (string, error) {
	if op.OldContent == "" {
		return "", ErrEmptyTarget
	}

	key, err := snapshotKey(op.Path)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	snap, exists := s.snapshots[key]
	if !exists {
		return "", ErrReadRequired
	}

	// Verify target is not a directory
	fi, err := os.Stat(op.Path)
	if err != nil {
		return "", fmt.Errorf("failed to stat target file: %w", err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("cannot apply patch to a directory: %s", op.Path)
	}

	// Verify current file on disk hasn't drifted
	currentDiskData, err := os.ReadFile(op.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read target file on disk: %w", err)
	}

	currentDiskContent := string(currentDiskData)
	currentDiskHash := CalculateHash(currentDiskContent)

	if op.BaseHash != "" && op.BaseHash != currentDiskHash {
		return "", fmt.Errorf("%w (expected %s, got %s)", ErrContentDrift, op.BaseHash, currentDiskHash)
	}
	if snap.Hash != currentDiskHash {
		return "", fmt.Errorf("%w (snapshot hash %s != disk hash %s)", ErrContentDrift, snap.Hash, currentDiskHash)
	}

	// Match target content
	matchCount := strings.Count(currentDiskContent, op.OldContent)
	if matchCount == 0 && strings.Contains(currentDiskContent, "\r\n") {
		crlfOld := toCRLF(op.OldContent)
		if c := strings.Count(currentDiskContent, crlfOld); c > 0 {
			op.OldContent = crlfOld
			op.NewContent = toCRLF(op.NewContent)
			matchCount = c
		}
	}
	if matchCount == 0 {
		return "", ErrTargetNotFound
	}

	if matchCount > 1 && !op.ReplaceAll {
		return "", fmt.Errorf("%w: found %d matches", ErrMultipleMatches, matchCount)
	}

	var newContent string
	if op.ReplaceAll {
		newContent = strings.ReplaceAll(currentDiskContent, op.OldContent, op.NewContent)
	} else {
		newContent = strings.Replace(currentDiskContent, op.OldContent, op.NewContent, 1)
	}

	// Write back surgically to disk
	perm := fi.Mode().Perm()
	if perm == 0 {
		perm = 0644
	}
	err = WriteFileAtomic(op.Path, []byte(newContent), perm)
	if err != nil {
		return "", fmt.Errorf("failed to write patched file: %w", err)
	}

	// Update snapshot
	newHash := CalculateHash(newContent)
	s.snapshots[key] = FileSnapshot{
		Path:      op.Path,
		Content:   newContent,
		Hash:      newHash,
		LineCount: len(strings.Split(newContent, "\n")),
	}

	return newHash, nil
}

// InspectGoSymbols parses a file's AST or lexical structure to extract symbol boundaries
func (s *Surgeon) InspectGoSymbols(path string) ([]SymbolSpan, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" && ext != ".go" {
		if data, err := os.ReadFile(path); err == nil {
			if syms, ok := inspectPolyglotSymbols(ext, data); ok {
				return syms, nil
			}
		}
	}
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("ast parser failed: %w", err)
	}

	var symbols []SymbolSpan
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "func"
			if d.Recv != nil {
				kind = "method"
			}
			symbols = append(symbols, SymbolSpan{
				Name:      d.Name.Name,
				Kind:      kind,
				StartLine: fset.Position(d.Pos()).Line,
				EndLine:   fset.Position(d.End()).Line,
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					kind := "type"
					switch ts.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}
					symbols = append(symbols, SymbolSpan{
						Name:      ts.Name.Name,
						Kind:      kind,
						StartLine: fset.Position(ts.Pos()).Line,
						EndLine:   fset.Position(ts.End()).Line,
					})
				}
			}
		}
	}

	return symbols, nil
}
