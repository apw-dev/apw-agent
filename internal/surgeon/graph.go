package surgeon

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SymbolDef describes a top-level symbol definition in the workspace.
type SymbolDef struct {
	File      string `json:"file"`
	Package   string `json:"package"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// CallEdge records a call or reference from a function/method to another symbol.
type CallEdge struct {
	FromFile   string `json:"from_file"`
	FromSymbol string `json:"from_symbol"`
	ToSymbol   string `json:"to_symbol"`
	Line       int    `json:"line"`
}

// WorkspaceGraph holds the cross-file AST symbol definitions and call edges of a workspace.
type WorkspaceGraph struct {
	Definitions []SymbolDef `json:"definitions"`
	Calls       []CallEdge  `json:"calls"`
}

var (
	// TypeScript / JavaScript top-level symbol patterns
	reTSFunc      = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*[(<]`)
	reTSClass     = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	reTSInterface = regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	reTSType      = regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*[=<]`)
	reTSArrow     = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`)
	// Python top-level / method symbol patterns
	rePyDef   = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	rePyClass = regexp.MustCompile(`^(\s*)class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	// Rust top-level symbol patterns
	reRsFn     = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+|const\s+|unsafe\s+)*fn\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	reRsStruct = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?struct\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	reRsEnum   = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	reRsTrait  = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?trait\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	// Generic call site identifier pattern
	reCallSite = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

var nonCallKeywords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true, "return": true,
	"function": true, "def": true, "class": true, "fn": true, "match": true, "sizeof": true,
	"typeof": true, "delete": true, "await": true, "yield": true, "super": true, "import": true,
	"require": true, "console": true, "print": true, "len": true, "range": true, "Some": true, "Ok": true, "Err": true,
}

func isPolyglotGraphExt(ext string) bool {
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".rs":
		return true
	}
	return false
}

func inspectPolyglotSymbols(ext string, data []byte) ([]SymbolSpan, bool) {
	if !isPolyglotGraphExt(ext) {
		return nil, false
	}
	lines := strings.Split(string(data), "\n")
	var out []SymbolSpan
	if ext == ".py" {
		for i, raw := range lines {
			line := strings.TrimRight(raw, "\r")
			var indent, name, kind string
			if m := rePyDef.FindStringSubmatch(line); len(m) == 3 {
				indent, name, kind = m[1], m[2], "func"
				if len(indent) > 0 {
					kind = "method"
				}
			} else if m := rePyClass.FindStringSubmatch(line); len(m) == 3 {
				indent, name, kind = m[1], m[2], "class"
			}
			if name == "" {
				continue
			}
			startLine := i + 1
			endLine := pyBlockEnd(lines, i, len(indent))
			out = append(out, SymbolSpan{Name: name, Kind: kind, StartLine: startLine, EndLine: endLine})
		}
		return out, true
	}

	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		var name, kind string
		switch ext {
		case ".rs":
			if m := reRsFn.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "func"
			} else if m := reRsStruct.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "struct"
			} else if m := reRsEnum.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "type"
			} else if m := reRsTrait.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "interface"
			}
		default: // TS / JS
			if m := reTSFunc.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "func"
			} else if m := reTSArrow.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "func"
			} else if m := reTSClass.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "class"
			} else if m := reTSInterface.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "interface"
			} else if m := reTSType.FindStringSubmatch(line); len(m) == 2 {
				name, kind = m[1], "type"
			}
		}
		if name == "" {
			continue
		}
		startLine := i + 1
		endLine := braceBlockEnd(lines, i)
		out = append(out, SymbolSpan{Name: name, Kind: kind, StartLine: startLine, EndLine: endLine})
	}
	return out, true
}

func pyBlockEnd(lines []string, startIdx, baseIndent int) int {
	endLine := startIdx + 1
	for j := startIdx + 1; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		ind := len(lines[j]) - len(strings.TrimLeft(lines[j], " \t"))
		if ind <= baseIndent {
			break
		}
		endLine = j + 1
	}
	return endLine
}

func braceBlockEnd(lines []string, startIdx int) int {
	depth := 0
	opened := false
	for j := startIdx; j < len(lines); j++ {
		line := lines[j]
		for k := 0; k < len(line); k++ {
			switch line[k] {
			case '{':
				depth++
				opened = true
			case '}':
				depth--
				if opened && depth <= 0 {
					return j + 1
				}
			}
		}
		if !opened && strings.HasSuffix(strings.TrimSpace(line), ";") {
			return j + 1
		}
	}
	return startIdx + 1
}

// InspectWorkspaceGraph builds a cross-file AST/lexical symbol and call graph for source files in root.
func InspectWorkspaceGraph(root string) (*WorkspaceGraph, error) {
	g := &WorkspaceGraph{}
	definedNames := make(map[string]bool)
	definedMethods := make(map[string]bool)
	pkgSymbols := make(map[string]map[string]bool)

	type parsedGoFile struct {
		rel     string
		pkgName string
		fset    *token.FileSet
		node    *ast.File
		imports map[string]string // local alias/pkg base -> import path base
	}
	type parsedPolyFile struct {
		rel   string
		lines []string
		spans []SymbolSpan
	}
	var parsedGo []parsedGoFile
	var parsedPoly []parsedPolyFile

	err := filepath.WalkDir(root, func(fpath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".apw", ".worktrees", "vendor", "node_modules", "dist", "build", ".next", "target", "__pycache__", ".venv", "venv":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, fpath)
		if err != nil || AllowedPath(rel) != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(fpath))

		if ext == ".go" {
			fset := token.NewFileSet()
			node, parseErr := parser.ParseFile(fset, fpath, nil, 0)
			if parseErr != nil {
				return nil
			}
			pkgName := ""
			if node.Name != nil {
				pkgName = node.Name.Name
			}
			if pkgSymbols[pkgName] == nil {
				pkgSymbols[pkgName] = make(map[string]bool)
			}
			imports := make(map[string]string)
			for _, imp := range node.Imports {
				if imp.Path == nil {
					continue
				}
				impPath := strings.Trim(imp.Path.Value, `"`)
				base := path.Base(impPath)
				alias := base
				if imp.Name != nil && imp.Name.Name != "_" && imp.Name.Name != "." {
					alias = imp.Name.Name
				}
				imports[alias] = base
			}
			for _, decl := range node.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					kind := "func"
					if d.Recv != nil {
						kind = "method"
						definedMethods[d.Name.Name] = true
					}
					g.Definitions = append(g.Definitions, SymbolDef{
						File:      relSlash,
						Package:   pkgName,
						Name:      d.Name.Name,
						Kind:      kind,
						StartLine: fset.Position(d.Pos()).Line,
						EndLine:   fset.Position(d.End()).Line,
					})
					definedNames[d.Name.Name] = true
					pkgSymbols[pkgName][d.Name.Name] = true
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
							g.Definitions = append(g.Definitions, SymbolDef{
								File:      relSlash,
								Package:   pkgName,
								Name:      ts.Name.Name,
								Kind:      kind,
								StartLine: fset.Position(ts.Pos()).Line,
								EndLine:   fset.Position(ts.End()).Line,
							})
							definedNames[ts.Name.Name] = true
							pkgSymbols[pkgName][ts.Name.Name] = true
						}
					}
				}
			}
			parsedGo = append(parsedGo, parsedGoFile{rel: relSlash, pkgName: pkgName, fset: fset, node: node, imports: imports})
			return nil
		}

		if isPolyglotGraphExt(ext) {
			data, _, readErr := ReadInRoot(root, rel)
			if readErr != nil {
				return nil
			}
			spans, ok := inspectPolyglotSymbols(ext, data)
			if !ok || len(spans) == 0 {
				return nil
			}
			pkgName := path.Dir(relSlash)
			if pkgName == "." {
				pkgName = strings.TrimPrefix(ext, ".")
			}
			for _, sp := range spans {
				g.Definitions = append(g.Definitions, SymbolDef{
					File:      relSlash,
					Package:   pkgName,
					Name:      sp.Name,
					Kind:      sp.Kind,
					StartLine: sp.StartLine,
					EndLine:   sp.EndLine,
				})
				definedNames[sp.Name] = true
			}
			parsedPoly = append(parsedPoly, parsedPolyFile{
				rel:   relSlash,
				lines: strings.Split(string(data), "\n"),
				spans: spans,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	seenEdge := make(map[string]bool)
	for _, pf := range parsedGo {
		for _, decl := range pf.node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			caller := fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var callee string
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					callee = fun.Name
				case *ast.SelectorExpr:
					if selX, isIdent := fun.X.(*ast.Ident); isIdent {
						if impPkg, isImported := pf.imports[selX.Name]; isImported {
							// Package-qualified call (e.g. auth.ValidateToken): only match if impPkg defines Sel.Name in workspace
							if syms := pkgSymbols[impPkg]; syms != nil && syms[fun.Sel.Name] {
								callee = fun.Sel.Name
							} else if syms := pkgSymbols[selX.Name]; syms != nil && syms[fun.Sel.Name] {
								callee = fun.Sel.Name
							} else {
								// Imported external/stdlib package not in workspace (e.g., os.Close, fmt.Sprintf) -> ignore
								return true
							}
						} else if definedMethods[fun.Sel.Name] || definedNames[fun.Sel.Name] {
							callee = fun.Sel.Name
						}
					} else if definedMethods[fun.Sel.Name] || definedNames[fun.Sel.Name] {
						callee = fun.Sel.Name
					}
				}
				if callee == "" || callee == caller || !definedNames[callee] {
					return true
				}
				key := pf.rel + ":" + caller + "->" + callee
				if seenEdge[key] {
					return true
				}
				seenEdge[key] = true
				g.Calls = append(g.Calls, CallEdge{
					FromFile:   pf.rel,
					FromSymbol: caller,
					ToSymbol:   callee,
					Line:       pf.fset.Position(call.Pos()).Line,
				})
				return true
			})
		}
	}

	for _, pf := range parsedPoly {
		for _, sp := range pf.spans {
			if sp.Kind != "func" && sp.Kind != "method" && sp.Kind != "class" {
				continue
			}
			caller := sp.Name
			startIdx := sp.StartLine - 1
			endIdx := sp.EndLine
			if startIdx < 0 {
				startIdx = 0
			}
			if endIdx > len(pf.lines) {
				endIdx = len(pf.lines)
			}
			for lineIdx := startIdx; lineIdx < endIdx; lineIdx++ {
				matches := reCallSite.FindAllStringSubmatch(pf.lines[lineIdx], -1)
				for _, m := range matches {
					callee := m[1]
					if callee == "" || callee == caller || nonCallKeywords[callee] || !definedNames[callee] {
						continue
					}
					key := pf.rel + ":" + caller + "->" + callee
					if seenEdge[key] {
						continue
					}
					seenEdge[key] = true
					g.Calls = append(g.Calls, CallEdge{
						FromFile:   pf.rel,
						FromSymbol: caller,
						ToSymbol:   callee,
						Line:       lineIdx + 1,
					})
				}
			}
		}
	}

	sort.Slice(g.Definitions, func(i, j int) bool {
		if g.Definitions[i].File != g.Definitions[j].File {
			return g.Definitions[i].File < g.Definitions[j].File
		}
		return g.Definitions[i].StartLine < g.Definitions[j].StartLine
	})
	sort.Slice(g.Calls, func(i, j int) bool {
		if g.Calls[i].FromFile != g.Calls[j].FromFile {
			return g.Calls[i].FromFile < g.Calls[j].FromFile
		}
		return g.Calls[i].Line < g.Calls[j].Line
	})
	return g, nil
}

// FindSymbols searches the workspace graph for definitions matching query (by exact name,
// package.Symbol, or case-insensitive substring). Exact matches are ordered first.
func (g *WorkspaceGraph) FindSymbols(query string) []SymbolDef {
	if g == nil {
		return nil
	}
	q := strings.TrimSpace(strings.TrimPrefix(query, "#"))
	if q == "" {
		return nil
	}
	qLower := strings.ToLower(q)
	var exact, partial []SymbolDef
	for _, d := range g.Definitions {
		qualified := d.Package + "." + d.Name
		if d.Name == q || qualified == q || strings.EqualFold(d.Name, q) || strings.EqualFold(qualified, q) {
			exact = append(exact, d)
		} else if strings.Contains(strings.ToLower(d.Name), qLower) || strings.Contains(strings.ToLower(qualified), qLower) {
			partial = append(partial, d)
		}
	}
	return append(exact, partial...)
}

// NeighborsOfFile returns workspace files connected to rel by 1-hop incoming or outgoing call edges.
func (g *WorkspaceGraph) NeighborsOfFile(rel string) []string {
	if g == nil || rel == "" {
		return nil
	}
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	fileSyms := make(map[string]bool)
	symFiles := make(map[string][]string)
	for _, d := range g.Definitions {
		symFiles[d.Name] = append(symFiles[d.Name], d.File)
		if d.File == rel {
			fileSyms[d.Name] = true
		}
	}
	neighbors := make(map[string]bool)
	for _, c := range g.Calls {
		if c.FromFile == rel {
			for _, f := range symFiles[c.ToSymbol] {
				if f != rel {
					neighbors[f] = true
				}
			}
		} else if fileSyms[c.ToSymbol] && c.FromFile != rel {
			neighbors[c.FromFile] = true
		}
	}
	var out []string
	for f := range neighbors {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// CallersOf returns all call edges targeting symbol.
func (g *WorkspaceGraph) CallersOf(symbol string) []CallEdge {
	if g == nil || symbol == "" {
		return nil
	}
	var out []CallEdge
	for _, c := range g.Calls {
		if c.ToSymbol == symbol {
			out = append(out, c)
		}
	}
	return out
}

// ImpactOfFiles summarizes cross-file callers of symbols defined in the given relative file paths.
func (g *WorkspaceGraph) ImpactOfFiles(files []string) string {
	if g == nil || len(files) == 0 {
		return ""
	}
	targetSet := make(map[string]bool, len(files))
	for _, f := range files {
		targetSet[filepath.ToSlash(strings.TrimSpace(f))] = true
	}
	var targetSymbols []string
	symbolFile := make(map[string]string)
	for _, def := range g.Definitions {
		if targetSet[def.File] {
			targetSymbols = append(targetSymbols, def.Name)
			symbolFile[def.Name] = def.File
		}
	}
	if len(targetSymbols) == 0 {
		return ""
	}
	var sb strings.Builder
	count := 0
	for _, sym := range targetSymbols {
		for _, edge := range g.CallersOf(sym) {
			if targetSet[edge.FromFile] {
				continue // skip intra-target calls to highlight external impact
			}
			if count == 0 {
				sb.WriteString("Cross-file callers referencing symbols in target files:\n")
			}
			fmt.Fprintf(&sb, "- %s:%d (%s) calls %s (defined in %s)\n", edge.FromFile, edge.Line, edge.FromSymbol, sym, symbolFile[sym])
			count++
			if count >= 25 {
				sb.WriteString("- [more callers omitted]\n")
				return sb.String()
			}
		}
	}
	return sb.String()
}
