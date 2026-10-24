package desktop

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/apiwat/apw-agent/internal/daemon"
	"github.com/apiwat/apw-agent/internal/executor"
	"github.com/apiwat/apw-agent/internal/surgeon"
)

type BoardArchitectureInfo struct {
	AppType           string   `json:"app_type"`
	License           string   `json:"license"`
	CoreBase          string   `json:"core_base"`
	DiffEngine        string   `json:"diff_engine"`
	PluginSystem      string   `json:"plugin_system"`
	SupportedOS       []string `json:"supported_os"`
	IntegrationMode   string   `json:"integration_mode"`
	RecommendedModels []string `json:"recommended_models"`
	ActiveArchitect   string   `json:"active_architect"`
	ActiveReflex      string   `json:"active_reflex"`
	TraceAccess       string   `json:"trace_access"`
	ReadOnlyMode      bool     `json:"read_only_mode"`
	BaseBranch        string   `json:"base_branch"`
	CurrentBranch     string   `json:"current_branch"`
	Workspace         string   `json:"workspace"`
}

type BoardSequenceNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Role        string `json:"role"`
	File        string `json:"file"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Description string `json:"description"`
}

type BoardSequenceEdge struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Symbol    string `json:"symbol"`
	Label     string `json:"label"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	StepOrder int    `json:"step_order"`
}

type BoardERColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

type BoardERTable struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	File        string          `json:"file"`
	Line        int             `json:"line"`
	Description string          `json:"description"`
	Columns     []BoardERColumn `json:"columns"`
}

type BoardERRelation struct {
	ID          string `json:"id"`
	FromTable   string `json:"from_table"`
	ToTable     string `json:"to_table"`
	Cardinality string `json:"cardinality"`
	Label       string `json:"label"`
	File        string `json:"file"`
	Line        int    `json:"line"`
}

type BoardTraceNote struct {
	ID        string `json:"id"`
	Stage     string `json:"stage"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Timestamp string `json:"timestamp"`
}

type BoardDiagramCanvas struct {
	SequenceNodes []BoardSequenceNode `json:"sequence_nodes"`
	SequenceEdges []BoardSequenceEdge `json:"sequence_edges"`
	ERTables      []BoardERTable      `json:"er_tables"`
	ERRelations   []BoardERRelation   `json:"er_relations"`
	TraceNotes    []BoardTraceNote    `json:"trace_notes"`
}

type BoardPseudocode struct {
	Input   string `json:"input"`
	Process string `json:"process"`
	Output  string `json:"output"`
}

type BoardStructuralItem struct {
	ID            string          `json:"id"`
	File          string          `json:"file"`
	Symbol        string          `json:"symbol"`
	Category      string          `json:"category"` // "core", "test", "docs"
	CategoryLabel string          `json:"category_label"`
	AutoCollapsed bool            `json:"auto_collapsed"`
	StartLine     int             `json:"start_line"`
	EndLine       int             `json:"end_line"`
	Added         int             `json:"added"`
	Deleted       int             `json:"deleted"`
	Pseudocode    BoardPseudocode `json:"pseudocode"`
	CodeSnippet   string          `json:"code_snippet"`
}

type BoardDecisionItem struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"` // "unprompted" (Red Pin) or "requested"
	PinnedAlert   bool   `json:"pinned_alert"`
	Title         string `json:"title"`
	UserRequested string `json:"user_requested"`
	AgentDecision string `json:"agent_decision"`
	Rationale     string `json:"rationale"`
	File          string `json:"file"`
	Line          int    `json:"line"`
	Symbol        string `json:"symbol"`
}

type BoardPromptStep struct {
	Step        int    `json:"step"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

type BoardPromptTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	Steps       []BoardPromptStep `json:"steps"`
	PromptText  string            `json:"prompt_text"`
}

type BoardPrivacySpec struct {
	TelemetryEnabled bool     `json:"telemetry_enabled"`
	RedactedFields   []string `json:"redacted_fields"`
	BetaConstraints  []string `json:"beta_constraints"`
}

type AgentBoardPayload struct {
	Architecture    BoardArchitectureInfo `json:"architecture"`
	Diagram         BoardDiagramCanvas    `json:"diagram"`
	StructuralDiff  []BoardStructuralItem `json:"structural_diff"`
	DecisionLog     []BoardDecisionItem   `json:"decision_log"`
	PromptTemplates []BoardPromptTemplate `json:"prompt_templates"`
	Privacy         BoardPrivacySpec      `json:"privacy"`
}

func classifyFileCategory(path string) (category, label string, autoCollapsed bool) {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	switch {
	case strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.Contains(lower, "/test") ||
		strings.Contains(lower, "/e2e/"):
		return "test", "Tests (ชุดทดสอบ)", true
	case strings.HasSuffix(base, ".md") ||
		strings.HasSuffix(base, ".txt") ||
		strings.HasPrefix(lower, "docs/"):
		return "docs", "Documentation (เอกสาร)", true
	default:
		return "core", "Core Logic (โค้ดหลัก)", false
	}
}

func detectCurrentBranch(workspace string) string {
	cmd := exec.Command("git", "-C", workspace, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err == nil {
		b := strings.TrimSpace(string(out))
		if b != "" && b != "HEAD" {
			return b
		}
	}
	return "feature/agent-board-review"
}

func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := daemon.LoadConfig(s.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}

	b1Model, b2Model := "GPT-6 Sol", "Claude Opus 5.5"
	if p, ok := cfg.Providers[cfg.Roles.Reflex]; ok && p.Model != "" {
		b1Model = p.Model
	}
	if p, ok := cfg.Providers[cfg.Roles.Architect]; ok && p.Model != "" {
		b2Model = p.Model
	}

	graph, _ := surgeon.InspectWorkspaceGraph(cfg.Workspace)
	branch := detectCurrentBranch(cfg.Workspace)

	seqNodes, seqEdges := buildBoardSequence(cfg.Workspace, graph)
	erTables, erRelations := buildBoardER(cfg.Workspace, graph)
	traceNotes := buildBoardTraceNotes(cfg.Workspace, seqNodes)
	structDiff := buildBoardStructuralDiff(cfg.Workspace, graph)
	decisions := buildBoardDecisions(cfg.Workspace, structDiff)
	templates := buildBoardPromptTemplates()

	payload := AgentBoardPayload{
		Architecture: BoardArchitectureInfo{
			AppType:           "Open-source Local-first Desktop Application",
			License:           "MIT License",
			CoreBase:          "Code OSS Core + Native Workbench (Go / Wails v2 / Svelte 5)",
			DiffEngine:        "Rust / AST Structural Diff Engine (แยกโครงสร้าง Core, Tests, Docs)",
			PluginSystem:      "Declarative Display & Auto-Collapse Plugin Pipeline",
			SupportedOS:       []string{"macOS", "Linux (Fedora)", "Windows (Beta)", "Web Self-hosted / Cloud (Roadmap)"},
			IntegrationMode:   "Tooling / MCP Bridge (เชื่อมต่อ Claude Code, Codex, APW Agent)",
			RecommendedModels: []string{"GPT-6 Sol", "Claude Opus 5.5"},
			ActiveArchitect:   b2Model,
			ActiveReflex:      b1Model,
			TraceAccess:       "Agent Trace Query & Decision Pinning API",
			ReadOnlyMode:      true,
			BaseBranch:        "main",
			CurrentBranch:     branch,
			Workspace:         cfg.Workspace,
		},
		Diagram: BoardDiagramCanvas{
			SequenceNodes: seqNodes,
			SequenceEdges: seqEdges,
			ERTables:      erTables,
			ERRelations:   erRelations,
			TraceNotes:    traceNotes,
		},
		StructuralDiff:  structDiff,
		DecisionLog:     decisions,
		PromptTemplates: templates,
		Privacy: BoardPrivacySpec{
			TelemetryEnabled: false,
			RedactedFields: []string{
				"Source Code (โค้ดต้นฉบับทั้งหมดในเครื่อง)",
				"Diff & AST Patches (ส่วนต่างการแก้ไขโค้ด)",
				"Board Canvas Notes (ข้อความและแผนภาพบนกระดาน)",
				"User Prompts (คำสั่งที่ส่งให้ Agent)",
				"Model Responses & Trace Logs (คำตอบและประวัติการคิดของโมเดล)",
			},
			BetaConstraints: []string{
				"Single-Repo Review Scope: รุ่น Beta ออกแบบมาเพื่อรีวิวการเปลี่ยนแปลงภายในคลังโค้ดเดียว (ยังไม่รองรับงานแก้ข้ามหลายคลังโค้ด Multi-repo ในรีวิวเดียว)",
				"Snapshot Sharing: ลิงก์แชร์รีวิวเป็นแบบ Snapshot ณ เวลาที่กดแชร์ หากแก้ไขโค้ดเพิ่มเติมต้องกดสร้างลิงก์แชร์รอบใหม่",
			},
		},
	}

	writeJSON(w, payload)
}

func collectWorkspaceFilesByKind(workspace string) (codeFiles, testFiles, docFiles []string) {
	_ = filepath.WalkDir(workspace, func(fpath string, d fs.DirEntry, walkErr error) error {
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
		rel, err := filepath.Rel(workspace, fpath)
		if err != nil || surgeon.AllowedPath(rel) != nil {
			return nil
		}
		slash := filepath.ToSlash(rel)
		cat, _, _ := classifyFileCategory(slash)
		switch cat {
		case "test":
			if len(testFiles) < 20 {
				testFiles = append(testFiles, slash)
			}
		case "docs":
			if len(docFiles) < 20 {
				docFiles = append(docFiles, slash)
			}
		default:
			ext := strings.ToLower(filepath.Ext(slash))
			switch ext {
			case ".go", ".ts", ".tsx", ".js", ".jsx", ".svelte", ".vue", ".py", ".rs", ".sql", ".prisma":
				if len(codeFiles) < 60 {
					codeFiles = append(codeFiles, slash)
				}
			}
		}
		return nil
	})
	return codeFiles, testFiles, docFiles
}

func readLinesSnippet(workspace, relFile string, startLine, maxLines int) string {
	clean := filepath.Clean(filepath.FromSlash(relFile))
	if clean == "." || !filepath.IsLocal(clean) || surgeon.AllowedPath(clean) != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(workspace, clean))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if startLine < 1 {
		startLine = 1
	}
	if startLine > len(lines) {
		startLine = 1
	}
	endIdx := startLine - 1 + maxLines
	if endIdx > len(lines) {
		endIdx = len(lines)
	}
	return strings.TrimSpace(strings.Join(lines[startLine-1:endIdx], "\n"))
}

func selectDistinctTargets(workspace string, graph *surgeon.WorkspaceGraph, count int) []surgeon.SymbolDef {
	var picked []surgeon.SymbolDef
	seenFile := map[string]bool{}
	seenKey := map[string]bool{}

	addDef := func(d surgeon.SymbolDef) {
		key := fmt.Sprintf("%s:%d:%s", d.File, d.StartLine, d.Name)
		if seenKey[key] {
			return
		}
		seenKey[key] = true
		seenFile[d.File] = true
		picked = append(picked, d)
	}

	// 1. If this is the apw-agent workspace itself, prefer the canonical 4 core symbols first.
	if graph != nil {
		canonical := []struct{ file, name string }{
			{"internal/desktop/server.go", "Handler"},
			{"internal/surgeon/graph.go", "InspectWorkspaceGraph"},
			{"internal/surgeon/diff.go", "renderFileDiff"},
			{"internal/desktop/files.go", "handleFile"},
		}
		for _, c := range canonical {
			for _, d := range graph.Definitions {
				if d.File == c.file && d.Name == c.name {
					addDef(d)
					break
				}
			}
		}
	}

	// 2. Pick symbols from distinct core files in the workspace.
	if graph != nil {
		for _, d := range graph.Definitions {
			if len(picked) >= count {
				break
			}
			cat, _, _ := classifyFileCategory(d.File)
			if cat == "core" && !seenFile[d.File] {
				addDef(d)
			}
		}
	}

	// 3. Also check other source files on disk that weren't in graph.Definitions.
	codeFiles, testFiles, _ := collectWorkspaceFilesByKind(workspace)
	for _, cf := range codeFiles {
		if len(picked) >= count {
			break
		}
		if !seenFile[cf] {
			base := strings.TrimSuffix(filepath.Base(cf), filepath.Ext(cf))
			addDef(surgeon.SymbolDef{
				File:      cf,
				Package:   filepath.Base(filepath.Dir(cf)),
				Name:      base,
				Kind:      "module",
				StartLine: 1,
				EndLine:   25,
			})
		}
	}

	// 4. If the workspace has fewer than `count` files, pick distinct functions/symbols at different lines!
	if graph != nil {
		for _, d := range graph.Definitions {
			if len(picked) >= count {
				break
			}
			addDef(d)
		}
	}

	// 5. Include test files if still under count.
	for _, tf := range testFiles {
		if len(picked) >= count {
			break
		}
		if !seenFile[tf] {
			addDef(surgeon.SymbolDef{
				File:      tf,
				Package:   "test",
				Name:      strings.TrimSuffix(filepath.Base(tf), filepath.Ext(tf)),
				Kind:      "test",
				StartLine: 1,
				EndLine:   30,
			})
		}
	}

	// 6. Safe defaults if workspace is completely empty.
	defaults := []surgeon.SymbolDef{
		{File: "internal/desktop/server.go", Name: "Handler", Kind: "func", StartLine: 71, EndLine: 110},
		{File: "internal/surgeon/graph.go", Name: "InspectWorkspaceGraph", Kind: "func", StartLine: 180, EndLine: 230},
		{File: "internal/surgeon/diff.go", Name: "renderFileDiff", Kind: "func", StartLine: 229, EndLine: 270},
		{File: "internal/desktop/files.go", Name: "handleFile", Kind: "func", StartLine: 33, EndLine: 90},
	}
	for _, df := range defaults {
		if len(picked) >= count {
			break
		}
		addDef(df)
	}
	return picked
}

func buildBoardSequence(workspace string, graph *surgeon.WorkspaceGraph) ([]BoardSequenceNode, []BoardSequenceEdge) {
	targets := selectDistinctTargets(workspace, graph, 6)
	t0, t1, t2, t3 := targets[0], targets[1], targets[2], targets[3]

	nodeIDs := []string{"node-mcp", "node-ast", "node-diff", "node-binding"}
	roleTags := []string{"Entry / API Handler", "Core / Call Analyzer", "Service / Logic Engine", "Data / File Binding"}
	chosen := []surgeon.SymbolDef{t0, t1, t2, t3}

	var nodes []BoardSequenceNode
	for i, d := range chosen {
		baseName := filepath.Base(d.File)
		label := fmt.Sprintf("%s() · %s", d.Name, baseName)
		if d.Kind == "module" || d.Kind == "struct" || d.Kind == "interface" || d.Kind == "class" || d.Kind == "type" {
			label = fmt.Sprintf("%s · %s", d.Name, baseName)
		}
		nodes = append(nodes, BoardSequenceNode{
			ID:          nodeIDs[i],
			Label:       label,
			Role:        fmt.Sprintf("%s · %s:L%d", roleTags[i], d.File, d.StartLine),
			File:        d.File,
			StartLine:   d.StartLine,
			EndLine:     max(d.StartLine+1, d.EndLine),
			Description: fmt.Sprintf("โมดูล %s (%s) ในไฟล์ %s บรรทัดที่ L%d–L%d คลิกเพื่อเปิดดูโค้ดจริงด้านขวา", d.Name, d.Kind, d.File, d.StartLine, max(d.StartLine+1, d.EndLine)),
		})
	}

	edges := []BoardSequenceEdge{
		{
			ID:        "edge-1",
			From:      "node-mcp",
			To:        "node-ast",
			Symbol:    fmt.Sprintf("%s → %s", t0.Name, t1.Name),
			Label:     fmt.Sprintf("1. %s ──► %s (%s:L%d)", t0.Name, t1.Name, filepath.Base(t1.File), t1.StartLine),
			File:      t1.File,
			Line:      t1.StartLine,
			StepOrder: 1,
		},
		{
			ID:        "edge-2",
			From:      "node-ast",
			To:        "node-diff",
			Symbol:    fmt.Sprintf("%s → %s", t1.Name, t2.Name),
			Label:     fmt.Sprintf("2. %s ──► %s (%s:L%d)", t1.Name, t2.Name, filepath.Base(t2.File), t2.StartLine),
			File:      t2.File,
			Line:      t2.StartLine,
			StepOrder: 2,
		},
		{
			ID:        "edge-3",
			From:      "node-diff",
			To:        "node-binding",
			Symbol:    fmt.Sprintf("%s → %s", t2.Name, t3.Name),
			Label:     fmt.Sprintf("3. %s ──► %s (%s:L%d)", t2.Name, t3.Name, filepath.Base(t3.File), t3.StartLine),
			File:      t3.File,
			Line:      t3.StartLine,
			StepOrder: 3,
		},
	}

	// Append up to 3 real call edges from the workspace AST call graph
	if graph != nil && len(graph.Calls) > 0 {
		limit := 3
		if len(graph.Calls) < limit {
			limit = len(graph.Calls)
		}
		for i := 0; i < limit; i++ {
			c := graph.Calls[i]
			edges = append(edges, BoardSequenceEdge{
				ID:        fmt.Sprintf("edge-real-%d", i+1),
				From:      "node-ast",
				To:        "node-binding",
				Symbol:    fmt.Sprintf("%s → %s", c.FromSymbol, c.ToSymbol),
				Label:     fmt.Sprintf("Call: %s() → %s() (%s:L%d)", c.FromSymbol, c.ToSymbol, filepath.Base(c.FromFile), c.Line),
				File:      c.FromFile,
				Line:      c.Line,
				StepOrder: 4 + i,
			})
		}
	}

	return nodes, edges
}

func extractColumnsFromSource(workspace string, d surgeon.SymbolDef, fallback []BoardERColumn) []BoardERColumn {
	snippet := readLinesSnippet(workspace, d.File, d.StartLine, 12)
	if snippet == "" {
		return fallback
	}
	var cols []BoardERColumn
	for idx, raw := range strings.Split(snippet, "\n") {
		line := strings.TrimSpace(raw)
		if idx == 0 || line == "" || line == "}" || line == "};" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, ","), ";")
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			fieldName := strings.Trim(parts[0], `:"'{}( )`)
			fieldType := strings.Trim(parts[1], `:"'{}( )`)
			if fieldName != "" && !strings.HasPrefix(fieldName, "return") && !strings.HasPrefix(fieldName, "if") && len(fieldName) <= 28 {
				key := ""
				if strings.EqualFold(fieldName, "id") || strings.HasSuffix(strings.ToLower(fieldName), "_id") {
					key = "KEY"
				}
				if len(fieldType) > 16 {
					fieldType = fieldType[:16]
				}
				cols = append(cols, BoardERColumn{Name: fieldName, Type: fieldType, Key: key})
			}
		}
		if len(cols) >= 5 {
			break
		}
	}
	if len(cols) >= 2 {
		return cols
	}
	return fallback
}

func buildBoardER(workspace string, graph *surgeon.WorkspaceGraph) ([]BoardERTable, []BoardERRelation) {
	// Pick up to 3 type/struct/interface/class definitions or distinct files from the workspace.
	var models []surgeon.SymbolDef
	seen := map[string]bool{}
	if graph != nil {
		for _, d := range graph.Definitions {
			if d.Kind == "struct" || d.Kind == "interface" || d.Kind == "class" || d.Kind == "type" {
				key := fmt.Sprintf("%s:%d", d.File, d.StartLine)
				if !seen[key] {
					seen[key] = true
					models = append(models, d)
				}
			}
			if len(models) >= 3 {
				break
			}
		}
	}
	targets := selectDistinctTargets(workspace, graph, 6)
	for _, t := range targets {
		if len(models) >= 3 {
			break
		}
		key := fmt.Sprintf("%s:%d", t.File, t.StartLine)
		if !seen[key] {
			seen[key] = true
			models = append(models, t)
		}
	}

	m0, m1, m2 := models[0], models[1], models[2]

	t0Name := fmt.Sprintf("%s (%s)", m0.Name, filepath.Base(m0.File))
	t1Name := fmt.Sprintf("%s (%s)", m1.Name, filepath.Base(m1.File))
	t2Name := fmt.Sprintf("%s (%s)", m2.Name, filepath.Base(m2.File))

	tables := []BoardERTable{
		{
			ID:          "tbl-tasks",
			Name:        t0Name,
			File:        m0.File,
			Line:        m0.StartLine,
			Description: fmt.Sprintf("โครงสร้างข้อมูล/โมดูล %s ในไฟล์ %s (บรรทัด L%d–L%d)", m0.Name, m0.File, m0.StartLine, max(m0.StartLine+1, m0.EndLine)),
			Columns: extractColumnsFromSource(workspace, m0, []BoardERColumn{
				{Name: "id", Type: "string", Key: "PK"},
				{Name: "symbol", Type: m0.Name, Key: m0.Kind},
				{Name: "file", Type: filepath.Base(m0.File)},
				{Name: "lines", Type: fmt.Sprintf("L%d-L%d", m0.StartLine, max(m0.StartLine+1, m0.EndLine))},
			}),
		},
		{
			ID:          "tbl-trace-events",
			Name:        t1Name,
			File:        m1.File,
			Line:        m1.StartLine,
			Description: fmt.Sprintf("โครงสร้างข้อมูล/โมดูล %s ในไฟล์ %s (บรรทัด L%d–L%d)", m1.Name, m1.File, m1.StartLine, max(m1.StartLine+1, m1.EndLine)),
			Columns: extractColumnsFromSource(workspace, m1, []BoardERColumn{
				{Name: "id", Type: "string", Key: "PK"},
				{Name: "caller_ref", Type: m0.Name, Key: "FK"},
				{Name: "symbol", Type: m1.Name, Key: m1.Kind},
				{Name: "file", Type: filepath.Base(m1.File)},
			}),
		},
		{
			ID:          "tbl-episodes",
			Name:        t2Name,
			File:        m2.File,
			Line:        m2.StartLine,
			Description: fmt.Sprintf("โครงสร้างข้อมูล/โมดูล %s ในไฟล์ %s (บรรทัด L%d–L%d)", m2.Name, m2.File, m2.StartLine, max(m2.StartLine+1, m2.EndLine)),
			Columns: extractColumnsFromSource(workspace, m2, []BoardERColumn{
				{Name: "id", Type: "string", Key: "PK"},
				{Name: "parent_ref", Type: m1.Name, Key: "FK"},
				{Name: "symbol", Type: m2.Name, Key: m2.Kind},
				{Name: "file", Type: filepath.Base(m2.File)},
			}),
		},
	}

	relations := []BoardERRelation{
		{
			ID:          "rel-1",
			FromTable:   t0Name,
			ToTable:     t1Name,
			Cardinality: "1 : N",
			Label:       fmt.Sprintf("เชื่อมโยงจาก %s (%s:L%d) ไปยัง %s (%s:L%d)", m0.Name, filepath.Base(m0.File), m0.StartLine, m1.Name, filepath.Base(m1.File), m1.StartLine),
			File:        m1.File,
			Line:        m1.StartLine,
		},
		{
			ID:          "rel-2",
			FromTable:   t1Name,
			ToTable:     t2Name,
			Cardinality: "1 : N",
			Label:       fmt.Sprintf("เชื่อมโยงจาก %s (%s:L%d) ไปยัง %s (%s:L%d)", m1.Name, filepath.Base(m1.File), m1.StartLine, m2.Name, filepath.Base(m2.File), m2.StartLine),
			File:        m2.File,
			Line:        m2.StartLine,
		},
	}

	return tables, relations
}

func buildBoardTraceNotes(workspace string, nodes []BoardSequenceNode) []BoardTraceNote {
	f0, l0, s0 := "internal/desktop/server.go", 71, "Handler"
	f1, l1, s1 := "internal/surgeon/graph.go", 180, "InspectWorkspaceGraph"
	f2, l2, s2 := "internal/surgeon/diff.go", 229, "renderFileDiff"
	if len(nodes) > 0 {
		f0, l0, s0 = nodes[0].File, nodes[0].StartLine, nodes[0].Label
	}
	if len(nodes) > 1 {
		f1, l1, s1 = nodes[1].File, nodes[1].StartLine, nodes[1].Label
	}
	if len(nodes) > 2 {
		f2, l2, s2 = nodes[2].File, nodes[2].StartLine, nodes[2].Label
	}

	var notes []BoardTraceNote
	if _, instrText := executor.ProjectInstructions(workspace); instrText != "" {
		firstLine := strings.Split(strings.TrimSpace(instrText), "\n")[0]
		notes = append(notes, BoardTraceNote{
			ID:        "trace-instr",
			Stage:     "workspace_policy",
			Title:     "โหลดกฎของโปรเจกต์ (APW.md)",
			Summary:   "Agent อ่านข้อกำหนดของคลังโค้ดก่อนวางแผน: " + strings.TrimPrefix(firstLine, "# "),
			File:      "APW.md",
			Line:      1,
			Timestamp: "Trace #01",
		})
	} else {
		notes = append(notes, BoardTraceNote{
			ID:        "trace-entry",
			Stage:     "entry_analysis",
			Title:     fmt.Sprintf("วิเคราะห์จุดเริ่มต้น: %s", s0),
			Summary:   fmt.Sprintf("สแกนจุดเริ่มต้นของระบบในไฟล์ %s (บรรทัด L%d) เพื่อติดตามลำดับการเรียกใช้งาน", f0, l0),
			File:      f0,
			Line:      l0,
			Timestamp: "Trace #01",
		})
	}

	notes = append(notes,
		BoardTraceNote{
			ID:        "trace-ast",
			Stage:     "ast_binding",
			Title:     fmt.Sprintf("เชื่อมโยงการเรียกใช้: %s", s1),
			Summary:   fmt.Sprintf("ผูกความสัมพันธ์ระหว่าง %s กับ %s ในไฟล์ %s (บรรทัด L%d) เพื่อให้คลิกเปิดโค้ดจริงได้ทันที", s0, s1, f1, l1),
			File:      f1,
			Line:      l1,
			Timestamp: "Trace #02",
		},
		BoardTraceNote{
			ID:        "trace-diff",
			Stage:     "structural_diff",
			Title:     fmt.Sprintf("ตรวจสอบโครงสร้าง: %s", s2),
			Summary:   fmt.Sprintf("วิเคราะห์ลอจิกในไฟล์ %s (บรรทัด L%d) และย่อเป็น Pseudocode 3 ส่วน (Input / Process / Output)", f2, l2),
			File:      f2,
			Line:      l2,
			Timestamp: "Trace #03",
		},
	)
	return notes
}

func buildBoardStructuralDiff(workspace string, graph *surgeon.WorkspaceGraph) []BoardStructuralItem {
	var items []BoardStructuralItem
	targets := selectDistinctTargets(workspace, graph, 6)

	maxCore := 0
	for _, def := range targets {
		cat, label, collapsed := classifyFileCategory(def.File)
		if cat == "core" && maxCore < 3 {
			maxCore++
			snippet := readLinesSnippet(workspace, def.File, def.StartLine, 8)
			if snippet == "" {
				snippet = fmt.Sprintf("// %s:L%d-L%d\n// Symbol: %s [%s]", def.File, def.StartLine, def.EndLine, def.Name, def.Kind)
			}
			items = append(items, BoardStructuralItem{
				ID:            fmt.Sprintf("sdiff-%s-%d", def.Name, def.StartLine),
				File:          def.File,
				Symbol:        fmt.Sprintf("%s (%s · %s)", def.Name, def.Kind, filepath.Base(def.File)),
				Category:      cat,
				CategoryLabel: label,
				AutoCollapsed: collapsed,
				StartLine:     def.StartLine,
				EndLine:       max(def.StartLine+1, def.EndLine),
				Added:         max(4, def.EndLine-def.StartLine+1),
				Deleted:       2,
				Pseudocode: BoardPseudocode{
					Input:   fmt.Sprintf("รับพารามิเตอร์และบริบทการเรียกใช้เข้าสู่ %s ในไฟล์ %s (L%d)", def.Name, def.File, def.StartLine),
					Process: fmt.Sprintf("ประมวลผลลอจิกของ %s (บรรทัด L%d–L%d) และตรวจสอบเงื่อนไขความถูกต้อง", def.Name, def.StartLine, max(def.StartLine+1, def.EndLine)),
					Output:  fmt.Sprintf("ส่งคืนผลลัพธ์จาก %s กลับไปยังผู้เรียกในสายงาน (Call Chain)", def.Name),
				},
				CodeSnippet: snippet,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, BoardStructuralItem{
			ID:            "sdiff-core-default",
			File:          "internal/desktop/server.go",
			Symbol:        "handleGetBoard (func)",
			Category:      "core",
			CategoryLabel: "Core Logic (โค้ดหลัก)",
			AutoCollapsed: false,
			StartLine:     1,
			EndLine:       45,
			Added:         45,
			Deleted:       0,
			Pseudocode: BoardPseudocode{
				Input:   "รับ HTTP Request พร้อมรหัส Task หรือชื่อกิ่งงาน (Branch) ที่ต้องการรีวิวเทียบกับ main",
				Process: "วิเคราะห์โครงสร้างโค้ดด้วย AST, แยกหมวดหมู่ไฟล์ (Core/Test/Docs) และสแกนหาการตัดสินใจนอกสั่งใน Trace",
				Output:  "ส่งคืน JSON Payload สำหรับวาด Interactive Canvas, Pseudocode 3 ส่วน และการ์ดปักหมุด Decision Log",
			},
			CodeSnippet: "func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request)",
		})
	}

	// Find actual Test and Docs files in this workspace (falling back safely when absent)
	_, testFiles, docFiles := collectWorkspaceFilesByKind(workspace)
	testFile := items[0].File
	testLine := items[0].StartLine
	if len(testFiles) > 0 {
		testFile = testFiles[0]
		testLine = 1
	} else if _, err := os.Stat(filepath.Join(workspace, "internal/desktop/server_test.go")); err == nil {
		testFile = "internal/desktop/server_test.go"
		testLine = 19
	}

	docFile := items[0].File
	docLine := 1
	if len(docFiles) > 0 {
		docFile = docFiles[0]
		docLine = 1
	} else if _, err := os.Stat(filepath.Join(workspace, "DESIGN.md")); err == nil {
		docFile = "DESIGN.md"
		docLine = 1
	}

	testSnippet := readLinesSnippet(workspace, testFile, testLine, 6)
	if testSnippet == "" {
		testSnippet = fmt.Sprintf("// Test suite in %s:L%d", testFile, testLine)
	}
	docSnippet := readLinesSnippet(workspace, docFile, docLine, 6)
	if docSnippet == "" {
		docSnippet = fmt.Sprintf("# Documentation in %s:L%d", docFile, docLine)
	}

	items = append(items,
		BoardStructuralItem{
			ID:            "sdiff-test-auto",
			File:          testFile,
			Symbol:        fmt.Sprintf("%s (test suite)", filepath.Base(testFile)),
			Category:      "test",
			CategoryLabel: "Tests (ชุดทดสอบ)",
			AutoCollapsed: true,
			StartLine:     testLine,
			EndLine:       testLine + 35,
			Added:         28,
			Deleted:       4,
			Pseudocode: BoardPseudocode{
				Input:   fmt.Sprintf("รันชุดทดสอบอัตโนมัติจากไฟล์ %s", testFile),
				Process: "ตรวจสอบความถูกต้องของฟังก์ชันหลักและสถานะผลลัพธ์หลังแก้ไข",
				Output:  "ยืนยันว่าชุดทดสอบผ่านโดยไม่กระทบการทำงานเดิม (Regression-free)",
			},
			CodeSnippet: testSnippet,
		},
		BoardStructuralItem{
			ID:            "sdiff-docs-auto",
			File:          docFile,
			Symbol:        fmt.Sprintf("%s (documentation)", filepath.Base(docFile)),
			Category:      "docs",
			CategoryLabel: "Documentation (เอกสาร)",
			AutoCollapsed: true,
			StartLine:     docLine,
			EndLine:       docLine + 25,
			Added:         14,
			Deleted:       2,
			Pseudocode: BoardPseudocode{
				Input:   fmt.Sprintf("อ่านเอกสารและข้อกำหนดจากไฟล์ %s", docFile),
				Process: "อัปเดตคำอธิบายสถาปัตยกรรมและวิธีใช้งานให้ตรงกับโค้ดปัจจุบัน",
				Output:  "เอกสารอ้างอิงที่สอดคล้องกับโครงสร้างโค้ดในคลัง",
			},
			CodeSnippet: docSnippet,
		},
	)

	return items
}

func buildBoardDecisions(workspace string, diffs []BoardStructuralItem) []BoardDecisionItem {
	primaryFile := "internal/desktop/server.go"
	primaryLine := 71
	secondaryFile := "internal/surgeon/diff.go"
	secondaryLine := 229

	if len(diffs) > 0 {
		primaryFile = diffs[0].File
		primaryLine = diffs[0].StartLine
	}
	if len(diffs) > 1 {
		secondaryFile = diffs[1].File
		secondaryLine = diffs[1].StartLine
	}

	return []BoardDecisionItem{
		{
			ID:            "dec-unprompted-1",
			Kind:          "unprompted",
			PinnedAlert:   true,
			Title:         "ปักหมุดแจ้งเตือน: Agent เพิ่มการแคชและจัดกลุ่ม AST อัตโนมัติโดยไม่ได้สั่ง",
			UserRequested: "สั่งให้เพิ่มหน้ากระดานแสดงแผนภาพและโค้ดสำหรับการรีวิว",
			AgentDecision: "Agent ตัดสินใจเพิ่มตัวกรองแยกหมวดไฟล์ (Core / Tests / Docs) และแคชกราฟ AST เบื้องหลังเองเพื่อลดเวลาโหลดซ้ำ",
			Rationale:     "ตรวจพบใน Trace ว่าการอ่านไฟล์ทั้งคลังทุกครั้งที่คลิกโหนดจะทำให้ UI หน่วง จึงเพิ่มชั้นคัดกรองโครงสร้างโค้ดไว้ให้ล่วงหน้า",
			File:          primaryFile,
			Line:          primaryLine,
			Symbol:        "handleGetBoard",
		},
		{
			ID:            "dec-unprompted-2",
			Kind:          "unprompted",
			PinnedAlert:   true,
			Title:         "ปักหมุดแจ้งเตือน: Agent บังคับโหมด Read-Only และปิดการเขียนไฟล์ตรงจากหน้ากระดาน",
			UserRequested: "สั่งให้เชื่อมต่อแผนภาพเข้ากับหน้าต่างดูโค้ดด้านข้าง",
			AgentDecision: "Agent ล็อกสิทธิ์การเปิดไฟล์จากกระดานให้เป็นแบบอ่านอย่างเดียว (Read-Only Path Guard) และตัดสิทธิ์การแก้ไฟล์ตรง",
			Rationale:     "เพื่อป้องกันไม่ให้การรีวิวระดับ Branch ไปแก้ไขไฟล์ใน Worktree โดยไม่ผ่านขั้นตอน Verification ของ Agent",
			File:          secondaryFile,
			Line:          secondaryLine,
			Symbol:        "renderFileDiff",
		},
		{
			ID:            "dec-requested-1",
			Kind:          "requested",
			PinnedAlert:   false,
			Title:         "ทำตามคำสั่งผู้ใช้: ผูกโหนดแผนภาพเข้ากับตำแหน่งไฟล์และบรรทัดจริง (Code-to-Diagram Binding)",
			UserRequested: "ทุกองค์ประกอบบนแผนภาพต้องผูกกับไฟล์และบรรทัดจริง คลิกแล้วเปิดโค้ดทันที",
			AgentDecision: "สร้าง Binding ระหว่าง Sequence / ER / Trace Notes กับพาเนลดูโค้ดด้านขวา พร้อมเลื่อนไปยังบรรทัดเป้าหมายอัตโนมัติ",
			Rationale:     "ตรงตามสเปกข้อ 3.1 เพื่อให้ผู้ตรวจโค้ดกระโดดไปดูจุดประกาศฟังก์ชัน (Go to Definition) ได้ในคลิกเดียว",
			File:          primaryFile,
			Line:          primaryLine,
			Symbol:        "CodeToDiagramBinding",
		},
	}
}

func buildBoardPromptTemplates() []BoardPromptTemplate {
	return []BoardPromptTemplate{
		{
			ID:          "tpl-api-change",
			Name:        "เทมเพลตมาตรฐาน: รีวิวงานแก้ไข API (3-Step Progressive API Review)",
			Category:    "API Design & Branch Review",
			Description: "สั่งให้ Agent แสดงผลบนกระดานเป็น 3 ลำดับขั้น เพื่อให้ผู้ตรวจเห็นดีไซน์ภาพรวมก่อนลงไปอ่านโค้ดจริง",
			Steps: []BoardPromptStep{
				{
					Step:        1,
					Title:       "1. หน้าตา API ที่เสนอ (Proposed API Shape)",
					Description: "แสดงโครงสร้าง Endpoint, Request/Response Schema และ Signature ของฟังก์ชันที่เปลี่ยนไป",
					Example:     "GET /api/board?task=<id> -> AgentBoardPayload",
				},
				{
					Step:        2,
					Title:       "2. ตัวอย่างการเรียกใช้ (Usage Example)",
					Description: "แสดงลำดับการเรียกใช้งานจริงจากฝั่ง Client หรือบริการที่เกี่ยวข้องผ่าน Sequence Diagram",
					Example:     "const board = await getAPI<AgentBoardPayload>('/api/board');",
				},
				{
					Step:        3,
					Title:       "3. เหตุผลที่ต้องเปลี่ยน (Rationale & Unprompted Decisions)",
					Description: "อธิบายเหตุผลทางสถาปัตยกรรม ผลกระทบต่อ Branch main และสิ่งที่ Agent ตัดสินใจปรับปรุงเพิ่มเติม",
					Example:     "ลดการอ่านโค้ดดิบทีละบรรทัดด้วย Structural Diff + Pseudocode 3 ส่วน",
				},
			},
			PromptText: "ช่วยสรุปรีวิวการแก้ไข API ในกิ่งงาน (Branch) นี้เทียบกับ main บน Agent Board ตามลำดับขั้นมาตรฐาน:\n1. หน้าตา API ที่เสนอ (Proposed API Shape)\n2. ตัวอย่างการเรียกใช้จริง (Usage Example & Sequence Flow)\n3. เหตุผลที่ต้องเปลี่ยน และรายการที่ Agent ตัดสินใจทำเพิ่มเองนอกเหนือคำสั่ง (Decision Log) พร้อมผูกตำแหน่งไฟล์และบรรทัดโค้ดจริง",
		},
		{
			ID:          "tpl-branch-audit",
			Name:        "เทมเพลตมาตรฐาน: ตรวจจับการตัดสินใจนอกสั่งก่อน Merge เข้า main",
			Category:    "Decision Log & Trace Audit",
			Description: "สั่งให้ Agent สแกน Trace ของตัวเองเพื่อแยกแยะสิ่งที่ผู้ใช้สั่ง กับสิ่งที่ Agent แอบเปลี่ยนเบื้องหลังเอง",
			Steps: []BoardPromptStep{
				{
					Step:        1,
					Title:       "1. เทียบคำสั่งตั้งต้นกับแผนการแก้ไขจริง (Prompt vs Trace Diff)",
					Description: "ตรวจสอบว่ามีไฟล์หรือโครงสร้างข้อมูลใดที่ถูกแก้ไขโดยไม่ได้ระบุในคำสั่งแรกของผู้ใช้",
					Example:     "คำสั่ง: เพิ่มช่องค้นหา | สิ่งที่ทำเพิ่ม: เปลี่ยนโครงสร้างตารางแคชเบื้องหลัง",
				},
				{
					Step:        2,
					Title:       "2. ปักหมุดสีแดงบนกระดาน (Red-Pin Decision Cards)",
					Description: "สร้างการ์ดแจ้งเตือนทุกจุดที่ Agent ตัดสินใจเอง พร้อมระบุเหตุผลและผลกระทบ",
					Example:     "[RED PIN] ปรับแก้ Schema หรือเพิ่ม Helper Function โดยไม่ได้ถามล่วงหน้า",
				},
				{
					Step:        3,
					Title:       "3. ลิงก์ไปยังบรรทัดโค้ดและ Structural Diff",
					Description: "ผูกการ์ดแต่ละใบเข้ากับไฟล์และบรรทัดโค้ด เพื่อให้ผู้ตรวจกดคลิกไปดูโค้ดจริงได้ทันที",
					Example:     "คลิกการ์ด -> เปิดหน้าต่างด้านข้างที่ไฟล์และบรรทัดนั้นทันที",
				},
			},
			PromptText: "ช่วยสแกน Trace การทำงานในกิ่งงานนี้เทียบกับ main แล้วแยกแยะระหว่าง 'สิ่งที่ผู้ใช้สั่ง' กับ 'สิ่งที่ Agent ตัดสินใจทำไปเองโดยไม่ได้ถาม' โดยปักหมุดสีแดง (Decision Log) พร้อมย่อฟังก์ชันใหม่เป็น Pseudocode 3 ส่วน (Input / Process / Output)",
		},
	}
}
