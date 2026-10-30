import type {
  AgentBoardPayload,
  BoardDecisionItem,
  BoardStructuralItem,
  BoardTraceNote,
  TaskEvent,
  TaskJob
} from "./api";
import { parseUnifiedDiff } from "./diff";
import { tNow } from "./i18n";
import {
  centerView,
  composerDraftText,
  highlightedFileLine,
  rightCollapsed,
  rightPanelView,
  selectedDiffFile
} from "./stores/settings";
import { fetchWorkspaceFile } from "./stores/workspace";

export interface HighlightedCanvasElement {
  id: string;
  kind: "sequence-node" | "sequence-edge" | "er-table" | "er-relation" | "trace-note";
  label: string;
  file: string;
  line: number;
  detail?: string;
}

export interface BoardPluginConfig {
  autoCollapseTests: boolean;
  autoCollapseDocs: boolean;
  defaultPseudocodeMode: boolean;
  highlightUnpromptedPins: boolean;
}

export function classifyDiffCategory(filePath: string): {
  category: "core" | "test" | "docs";
  label: string;
  autoCollapsed: boolean;
} {
  const lower = (filePath || "").replace(/\\/g, "/").toLowerCase();
  const base = lower.split("/").pop() || lower;
  if (
    base.endsWith("_test.go") ||
    base.endsWith(".test.ts") ||
    base.endsWith(".spec.ts") ||
    base.endsWith("_test.py") ||
    lower.includes("/test") ||
    lower.includes("/e2e/")
  ) {
    return { category: "test", label: tNow("Tests", "Tests (ชุดทดสอบ)"), autoCollapsed: true };
  }
  if (base.endsWith(".md") || base.endsWith(".txt") || lower.startsWith("docs/")) {
    return { category: "docs", label: tNow("Documentation", "Documentation (เอกสาร)"), autoCollapsed: true };
  }
  return { category: "core", label: tNow("Core Logic", "Core Logic (โค้ดหลัก)"), autoCollapsed: false };
}

export async function jumpToBoardCode(
  file: string,
  line = 1,
  options: { preferDiff?: boolean; taskId?: string } = {}
): Promise<boolean> {
  if (!file) return false;
  rightCollapsed.set(false);
  highlightedFileLine.set(line > 0 ? line : 1);

  if (options.preferDiff && options.taskId) {
    selectedDiffFile.set(file);
    rightPanelView.set("diff");
    return true;
  }

  if (options.taskId) {
    const res = await fetchWorkspaceFile(file, options.taskId);
    if (res.ok) {
      rightPanelView.set("file");
      return true;
    }
  }

  const wsRes = await fetchWorkspaceFile(file);
  if (wsRes.ok) {
    rightPanelView.set("file");
    return true;
  }

  selectedDiffFile.set(file);
  rightPanelView.set("diff");
  return false;
}

export function formatVisualFeedbackClipboard(
  selected: HighlightedCanvasElement[],
  userFeedback: string
): string {
  const header = "[Agent Board · Visual Feedback Loop]";
  const targets = selected
    .map(
      (item, idx) =>
        `${idx + 1}. [${item.kind}] ${item.label} -> ${item.file}:L${item.line}${
          item.detail ? ` (${item.detail})` : ""
        }`
    )
    .join("\n");
  const note = userFeedback.trim()
    ? `\n${tNow("Design / Code Instruction:", "คำสั่งปรับแก้ดีไซน์/โค้ด:")} ${userFeedback.trim()}`
    : `\n${tNow(
        "Design / Code Instruction: Please refine the highlighted elements on the board and update their bound code accordingly.",
        "คำสั่งปรับแก้ดีไซน์/โค้ด: ช่วยปรับโครงสร้างส่วนที่ไฮไลต์บนกระดานและอัปเดตโค้ดที่ผูกอยู่ให้เหมาะสม"
      )}`;
  return `${header}\n${tNow("Selected elements on board:", "องค์ประกอบที่เลือกบนกระดาน:")}\n${targets}${note}`;
}

export function sendFeedbackToComposer(promptText: string) {
  if (!promptText.trim()) return;
  composerDraftText.set(promptText.trim());
  centerView.set("thread");
}

export const DEFAULT_AGENT_BOARD_PAYLOAD: AgentBoardPayload = {
  architecture: {
    app_type: "Open-source Local-first Desktop Application",
    license: "MIT License",
    core_base: "Code OSS Core + Native Workbench (Go / Wails v2 / Svelte 5)",
    diff_engine: "Rust / AST Structural Diff Engine (แยกโครงสร้าง Core, Tests, Docs)",
    plugin_system: "Declarative Display & Auto-Collapse Plugin Pipeline",
    supported_os: ["macOS", "Linux (Fedora)", "Windows (Beta)", "Web Self-hosted / Cloud (Roadmap)"],
    integration_mode: "Tooling / MCP Bridge (เชื่อมต่อ Claude Code, Codex, APW Agent)",
    recommended_models: ["GPT-6 Sol", "Claude Opus 5.5"],
    active_architect: "Claude Opus 5.5",
    active_reflex: "GPT-6 Sol",
    trace_access: "Agent Trace Query & Decision Pinning API",
    read_only_mode: true,
    base_branch: "main",
    current_branch: "feature/agent-board",
    workspace: "apw-agent"
  },
  diagram: {
    sequence_nodes: [
      {
        id: "node-mcp",
        label: "AI Agent (MCP / Tooling)",
        role: "Claude Code · Codex · APW",
        file: "internal/desktop/server.go",
        start_line: 71,
        end_line: 105,
        description: "ส่ง Trace การทำงานและแผนภาพเข้าสู่กระดานผ่าน MCP Tooling โดยไม่ต้องสร้าง Agent ใหม่"
      },
      {
        id: "node-ast",
        label: "AST & Call Graph Analyzer",
        role: "Code OSS / Surgeon AST",
        file: "internal/surgeon/graph.go",
        start_line: 180,
        end_line: 245,
        description: "สแกนโครงสร้างโค้ดและจุดประกาศฟังก์ชัน (Go to Definition) เพื่อผูกเข้ากับโหนดบนกระดาน"
      },
      {
        id: "node-diff",
        label: "Structural Diff Engine",
        role: "Rust / AST Diff Pipeline",
        file: "internal/surgeon/diff.go",
        start_line: 229,
        end_line: 274,
        description: "แยกแยะโค้ดหลัก (Core), ชุดทดสอบ (Tests) และเอกสาร (Docs) พร้อมสรุป Pseudocode 3 ส่วน"
      },
      {
        id: "node-binding",
        label: "Code-to-Diagram Reader",
        role: "Read-Only Branch Viewer",
        file: "internal/desktop/files.go",
        start_line: 33,
        end_line: 112,
        description: "เปิดไฟล์และบรรทัดโค้ดจริงในหน้าต่างด้านข้างทันทีเมื่อผู้ใช้คลิกโหนดหรือลูกศรบนกระดาน"
      }
    ],
    sequence_edges: [
      {
        id: "edge-1",
        from: "node-mcp",
        to: "node-ast",
        symbol: "InspectWorkspaceGraph",
        label: "1. ดึง Trace + วิเคราะห์โครงสร้าง AST",
        file: "internal/surgeon/graph.go",
        line: 180,
        step_order: 1
      },
      {
        id: "edge-2",
        from: "node-ast",
        to: "node-diff",
        symbol: "renderFileDiff",
        label: "2. แยกหมวดโค้ด & ย่อ Pseudocode (Input/Process/Output)",
        file: "internal/surgeon/diff.go",
        line: 229,
        step_order: 2
      },
      {
        id: "edge-3",
        from: "node-diff",
        to: "node-binding",
        symbol: "handleFile",
        label: "3. ผูกตำแหน่งไฟล์:บรรทัด (Code-to-Diagram Binding)",
        file: "internal/desktop/files.go",
        line: 33,
        step_order: 3
      }
    ],
    er_tables: [
      {
        id: "tbl-tasks",
        name: "tasks",
        file: "internal/store/jobs.go",
        line: 1,
        description: "ตารางบันทึกงานของ Agent, สถานะ Worktree และผลลัพธ์ Structural Diff",
        columns: [
          { name: "id", type: "TEXT", key: "PK" },
          { name: "session_id", type: "TEXT", key: "FK" },
          { name: "instruction", type: "TEXT" },
          { name: "status", type: "TEXT" },
          { name: "worktree_path", type: "TEXT" },
          { name: "report_json", type: "JSON" }
        ]
      },
      {
        id: "tbl-trace-events",
        name: "task_trace_events",
        file: "internal/store/sqlite.go",
        line: 1,
        description: "ตารางบันทึก Trace ลำดับการตัดสินใจของ Agent สำหรับตรวจสอบ Decision Log",
        columns: [
          { name: "seq", type: "INTEGER", key: "PK" },
          { name: "task_id", type: "TEXT", key: "FK" },
          { name: "stage", type: "TEXT" },
          { name: "message", type: "TEXT" },
          { name: "unprompted_flag", type: "BOOLEAN" }
        ]
      },
      {
        id: "tbl-episodes",
        name: "autonomy_episodes",
        file: "internal/store/autonomy.go",
        line: 1,
        description: "ตารางบทเรียนและประวัติการรันอัตโนมัติ (Episodic Memory)",
        columns: [
          { name: "id", type: "TEXT", key: "PK" },
          { name: "task_id", type: "TEXT", key: "FK" },
          { name: "outcome", type: "TEXT" },
          { name: "lesson", type: "TEXT" }
        ]
      }
    ],
    er_relations: [
      {
        id: "rel-1",
        from_table: "tasks",
        to_table: "task_trace_events",
        cardinality: "1 : N",
        label: "บันทึก Trace และตรวจจับการตัดสินใจนอกสั่ง (Decision Log)",
        file: "internal/store/sqlite.go",
        line: 1
      },
      {
        id: "rel-2",
        from_table: "tasks",
        to_table: "autonomy_episodes",
        cardinality: "1 : N",
        label: "สรุปบทเรียนจากผลลัพธ์การรีวิว Branch",
        file: "internal/store/autonomy.go",
        line: 1
      }
    ],
    trace_notes: [
      {
        id: "trace-1",
        stage: "workspace_policy",
        title: "โหลดกฎของโปรเจกต์ (APW.md)",
        summary: "Agent อ่านข้อกำหนดของคลังโค้ดและแยก Worktree ก่อนวางแผนแก้ไข",
        file: "APW.md",
        line: 1,
        timestamp: "Trace #01"
      },
      {
        id: "trace-2",
        stage: "ast_binding",
        title: "ผูกแผนภาพเข้ากับโค้ดจริง (Code-to-Diagram Binding)",
        summary: "สแกน AST Symbols และสร้างลิงก์ตำแหน่งไฟล์+บรรทัด เพื่อให้คลิกบนกระดานแล้วกระโดดไปดูโค้ดจริงได้ทันที",
        file: "internal/desktop/server.go",
        line: 71,
        timestamp: "Trace #02"
      },
      {
        id: "trace-3",
        stage: "structural_diff",
        title: "แยกหมวดหมู่โค้ดและพับเก็บไฟล์รองอัตโนมัติ",
        summary: "ย่อฟังก์ชันใหม่เป็น Pseudocode 3 ส่วน (Input / Process / Output) และพับเก็บไฟล์ Tests + Docs ไว้เป็นค่าเริ่มต้น",
        file: "internal/surgeon/diff.go",
        line: 229,
        timestamp: "Trace #03"
      }
    ]
  },
  structural_diff: [
    {
      id: "sdiff-core-1",
      file: "internal/desktop/board.go",
      symbol: "handleGetBoard (func)",
      category: "core",
      category_label: "Core Logic (โค้ดหลัก)",
      auto_collapsed: false,
      start_line: 193,
      end_line: 258,
      added: 66,
      deleted: 0,
      pseudocode: {
        input: "รับ HTTP Request พร้อมรหัส Task หรือชื่อกิ่งงาน (Branch) ที่ต้องการรีวิวเทียบกับ main",
        process: "วิเคราะห์โครงสร้างโค้ดด้วย AST, แยกหมวดหมู่ไฟล์ (Core/Test/Docs) และสแกนหาการตัดสินใจนอกสั่งใน Trace",
        output: "ส่งคืน JSON Payload สำหรับวาด Interactive Canvas, Pseudocode 3 ส่วน และการ์ดปักหมุด Decision Log"
      },
      code_snippet: "func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {\n  cfg, _, err := daemon.LoadConfig(s.ConfigPath)\n  graph, _ := surgeon.InspectWorkspaceGraph(cfg.Workspace)\n  writeJSON(w, payload)\n}"
    },
    {
      id: "sdiff-core-2",
      file: "internal/surgeon/diff.go",
      symbol: "renderFileDiff (func)",
      category: "core",
      category_label: "Core Logic (โค้ดหลัก)",
      auto_collapsed: false,
      start_line: 229,
      end_line: 273,
      added: 44,
      deleted: 6,
      pseudocode: {
        input: "รับพาธไฟล์, สถานะไฟล์ก่อน/หลัง (FileState) และเนื้อหาไบต์ของโค้ดต้นฉบับเทียบกับที่แก้ไข",
        process: "เปรียบเทียบโครงสร้างบรรทัดด้วยอัลกอริทึม Myers Diff, ตรวจจับการเปลี่ยนโหมดไฟล์หรือบรรทัดจบ (LF/CRLF)",
        output: "สร้าง Unified Diff Hunks พร้อมหมายเลขบรรทัดสำหรับแสดงผลในหน้าต่างตรวจโค้ด"
      },
      code_snippet: "func renderFileDiff(path string, before, after FileState, original, data []byte) string"
    },
    {
      id: "sdiff-test-1",
      file: "internal/desktop/server_test.go",
      symbol: "TestServer_Endpoints (test suite)",
      category: "test",
      category_label: "Tests (ชุดทดสอบ)",
      auto_collapsed: true,
      start_line: 19,
      end_line: 95,
      added: 28,
      deleted: 4,
      pseudocode: {
        input: "จำลองเซิร์ฟเวอร์ทดสอบ (httptest.NewServer) และส่งคำขอไปยัง /api/board และ /api/status",
        process: "ตรวจเช็กสถานะ HTTP 200 OK และความครบถ้วนของโครงสร้างข้อมูลบนกระดาน",
        output: "ยืนยันว่าทุก Endpoint ทำงานถูกต้องโดยไม่กระทบโค้ดส่วนอื่น"
      },
      code_snippet: "func TestServer_Endpoints(t *testing.T) { ... }"
    },
    {
      id: "sdiff-docs-1",
      file: "DESIGN.md",
      symbol: "Architecture & Review Guidelines (docs)",
      category: "docs",
      category_label: "Documentation (เอกสาร)",
      auto_collapsed: true,
      start_line: 1,
      end_line: 30,
      added: 14,
      deleted: 2,
      pseudocode: {
        input: "ข้อกำหนดการออกแบบระบบกระดานตรวจโค้ด (Read-Only Workbench)",
        process: "อัปเดตคำอธิบายโครงสร้าง 3-Pane Workbench และการผูกแผนภาพกับโค้ดจริง",
        output: "เอกสารคู่มือสถาปัตยกรรมที่ตรงกับพฤติกรรมของระบบปัจจุบัน"
      },
      code_snippet: "# APW Desktop Design System & Agent Board Spec"
    }
  ],
  decision_log: [
    {
      id: "dec-unprompted-1",
      kind: "unprompted",
      pinned_alert: true,
      title: "ปักหมุดแจ้งเตือน: Agent เพิ่มการแคชและจัดกลุ่ม AST อัตโนมัติโดยไม่ได้สั่ง",
      user_requested: "สั่งให้เพิ่มหน้ากระดานแสดงแผนภาพและโค้ดสำหรับการรีวิว",
      agent_decision: "Agent ตัดสินใจเพิ่มตัวกรองแยกหมวดไฟล์ (Core / Tests / Docs) และแคชกราฟ AST เบื้องหลังเองเพื่อลดเวลาโหลดซ้ำ",
      rationale: "ตรวจพบใน Trace ว่าการอ่านไฟล์ทั้งคลังทุกครั้งที่คลิกโหนดจะทำให้ UI หน่วง จึงเพิ่มชั้นคัดกรองโครงสร้างโค้ดไว้ให้ล่วงหน้า",
      file: "internal/desktop/board.go",
      line: 193,
      symbol: "handleGetBoard"
    },
    {
      id: "dec-unprompted-2",
      kind: "unprompted",
      pinned_alert: true,
      title: "ปักหมุดแจ้งเตือน: Agent บังคับโหมด Read-Only และปิดการเขียนไฟล์ตรงจากหน้ากระดาน",
      user_requested: "สั่งให้เชื่อมต่อแผนภาพเข้ากับหน้าต่างดูโค้ดด้านข้าง",
      agent_decision: "Agent ล็อกสิทธิ์การเปิดไฟล์จากกระดานให้เป็นแบบอ่านอย่างเดียว (Read-Only Path Guard) และตัดสิทธิ์การแก้ไฟล์ตรง",
      rationale: "เพื่อป้องกันไม่ให้การรีวิวระดับ Branch ไปแก้ไขไฟล์ใน Worktree โดยไม่ผ่านขั้นตอน Verification ของ Agent",
      file: "internal/desktop/files.go",
      line: 33,
      symbol: "handleFile"
    },
    {
      id: "dec-requested-1",
      kind: "requested",
      pinned_alert: false,
      title: "ทำตามคำสั่งผู้ใช้: ผูกโหนดแผนภาพเข้ากับตำแหน่งไฟล์และบรรทัดจริง (Code-to-Diagram Binding)",
      user_requested: "ทุกองค์ประกอบบนแผนภาพต้องผูกกับไฟล์และบรรทัดจริง คลิกแล้วเปิดโค้ดทันที",
      agent_decision: "สร้าง Binding ระหว่าง Sequence / ER / Trace Notes กับพาเนลดูโค้ดด้านขวา พร้อมเลื่อนไปยังบรรทัดเป้าหมายอัตโนมัติ",
      rationale: "ตรงตามสเปกข้อ 3.1 เพื่อให้ผู้ตรวจโค้ดกระโดดไปดูจุดประกาศฟังก์ชัน (Go to Definition) ได้ในคลิกเดียว",
      file: "internal/desktop/server.go",
      line: 71,
      symbol: "Handler"
    }
  ],
  prompt_templates: [
    {
      id: "tpl-api-change",
      name: "เทมเพลตมาตรฐาน: รีวิวงานแก้ไข API (3-Step Progressive API Review)",
      category: "API Design & Branch Review",
      description: "สั่งให้ Agent แสดงผลบนกระดานเป็น 3 ลำดับขั้น เพื่อให้ผู้ตรวจเห็นดีไซน์ภาพรวมก่อนลงไปอ่านโค้ดจริง",
      steps: [
        {
          step: 1,
          title: "1. หน้าตา API ที่เสนอ (Proposed API Shape)",
          description: "แสดงโครงสร้าง Endpoint, Request/Response Schema และ Signature ของฟังก์ชันที่เปลี่ยนไป",
          example: "GET /api/board?task=<id> -> AgentBoardPayload"
        },
        {
          step: 2,
          title: "2. ตัวอย่างการเรียกใช้ (Usage Example)",
          description: "แสดงลำดับการเรียกใช้งานจริงจากฝั่ง Client หรือบริการที่เกี่ยวข้องผ่าน Sequence Diagram",
          example: "const board = await getAPI<AgentBoardPayload>('/api/board');"
        },
        {
          step: 3,
          title: "3. เหตุผลที่ต้องเปลี่ยน (Rationale & Unprompted Decisions)",
          description: "อธิบายเหตุผลทางสถาปัตยกรรม ผลกระทบต่อ Branch main และสิ่งที่ Agent ตัดสินใจปรับปรุงเพิ่มเติม",
          example: "ลดการอ่านโค้ดดิบทีละบรรทัดด้วย Structural Diff + Pseudocode 3 ส่วน"
        }
      ],
      prompt_text:
        "ช่วยสรุปรีวิวการแก้ไข API ในกิ่งงาน (Branch) นี้เทียบกับ main บน Agent Board ตามลำดับขั้นมาตรฐาน:\n1. หน้าตา API ที่เสนอ (Proposed API Shape)\n2. ตัวอย่างการเรียกใช้จริง (Usage Example & Sequence Flow)\n3. เหตุผลที่ต้องเปลี่ยน และรายการที่ Agent ตัดสินใจทำเพิ่มเองนอกเหนือคำสั่ง (Decision Log) พร้อมผูกตำแหน่งไฟล์และบรรทัดโค้ดจริง"
    },
    {
      id: "tpl-branch-audit",
      name: "เทมเพลตมาตรฐาน: ตรวจจับการตัดสินใจนอกสั่งก่อน Merge เข้า main",
      category: "Decision Log & Trace Audit",
      description: "สั่งให้ Agent สแกน Trace ของตัวเองเพื่อแยกแยะสิ่งที่ผู้ใช้สั่ง กับสิ่งที่ Agent แอบเปลี่ยนเบื้องหลังเอง",
      steps: [
        {
          step: 1,
          title: "1. เทียบคำสั่งตั้งต้นกับแผนการแก้ไขจริง (Prompt vs Trace Diff)",
          description: "ตรวจสอบว่ามีไฟล์หรือโครงสร้างข้อมูลใดที่ถูกแก้ไขโดยไม่ได้ระบุในคำสั่งแรกของผู้ใช้",
          example: "คำสั่ง: เพิ่มช่องค้นหา | สิ่งที่ทำเพิ่ม: เปลี่ยนโครงสร้างตารางแคชเบื้องหลัง"
        },
        {
          step: 2,
          title: "2. ปักหมุดสีแดงบนกระดาน (Red-Pin Decision Cards)",
          description: "สร้างการ์ดแจ้งเตือนทุกจุดที่ Agent ตัดสินใจเอง พร้อมระบุเหตุผลและผลกระทบ",
          example: "[RED PIN] ปรับแก้ Schema หรือเพิ่ม Helper Function โดยไม่ได้ถามล่วงหน้า"
        },
        {
          step: 3,
          title: "3. ลิงก์ไปยังบรรทัดโค้ดและ Structural Diff",
          description: "ผูกการ์ดแต่ละใบเข้ากับไฟล์และบรรทัดโค้ด เพื่อให้ผู้ตรวจกดคลิกไปดูโค้ดจริงได้ทันที",
          example: "คลิกการ์ด -> เปิดหน้าต่างด้านข้างที่ไฟล์และบรรทัดนั้นทันที"
        }
      ],
      prompt_text:
        "ช่วยสแกน Trace การทำงานในกิ่งงานนี้เทียบกับ main แล้วแยกแยะระหว่าง 'สิ่งที่ผู้ใช้สั่ง' กับ 'สิ่งที่ Agent ตัดสินใจทำไปเองโดยไม่ได้ถาม' โดยปักหมุดสีแดง (Decision Log) พร้อมย่อฟังก์ชันใหม่เป็น Pseudocode 3 ส่วน (Input / Process / Output)"
    }
  ],
  privacy: {
    telemetry_enabled: false,
    redacted_fields: [
      "Source Code (โค้ดต้นฉบับทั้งหมดในเครื่อง)",
      "Diff & AST Patches (ส่วนต่างการแก้ไขโค้ด)",
      "Board Canvas Notes (ข้อความและแผนภาพบนกระดาน)",
      "User Prompts (คำสั่งที่ส่งให้ Agent)",
      "Model Responses & Trace Logs (คำตอบและประวัติการคิดของโมเดล)"
    ],
    beta_constraints: [
      "Single-Repo Review Scope: รุ่น Beta ออกแบบมาเพื่อรีวิวการเปลี่ยนแปลงภายในคลังโค้ดเดียว (ยังไม่รองรับงานแก้ข้ามหลายคลังโค้ด Multi-repo ในรีวิวเดียว)",
      "Snapshot Sharing: ลิงก์แชร์รีวิวเป็นแบบ Snapshot ณ เวลาที่กดแชร์ หากแก้ไขโค้ดเพิ่มเติมต้องกดสร้างลิงก์แชร์รอบใหม่"
    ]
  }
};

export function deriveBoardForActiveTask(
  base: AgentBoardPayload | null,
  job: TaskJob | null,
  events: TaskEvent[] = []
): AgentBoardPayload {
  const board: AgentBoardPayload = base
    ? JSON.parse(JSON.stringify(base))
    : JSON.parse(JSON.stringify(DEFAULT_AGENT_BOARD_PAYLOAD));

  if (!job) return board;

  const fallbackFiles = job.report?.modified_files?.length
    ? job.report.modified_files
    : job.report?.plan?.affected_files || [];
  const parsedDiffs = parseUnifiedDiff(job.report?.diff || "", fallbackFiles);

  if (parsedDiffs.length > 0) {
    const taskDiffItems: BoardStructuralItem[] = parsedDiffs.map((fd, idx) => {
      const cat = classifyDiffCategory(fd.file);
      const firstChanged = fd.lines.find((l) => l.kind === "add" || l.kind === "del");
      const startLine = firstChanged?.newLine || firstChanged?.oldLine || 1;
      const addedLines = fd.lines.filter((l) => l.kind === "add").map((l) => l.text);
      const snippet =
        addedLines.slice(0, 6).join("\n") ||
        fd.lines
          .slice(0, 6)
          .map((l) => l.text)
          .join("\n") ||
        `// Modified ${fd.file}`;

      return {
        id: `task-sdiff-${idx}-${fd.file}`,
        file: fd.file,
        symbol: `${fd.file.split("/").pop() || fd.file} (+${fd.added} −${fd.deleted})`,
        category: cat.category,
        category_label: cat.label,
        auto_collapsed: cat.autoCollapsed,
        start_line: startLine,
        end_line: startLine + Math.max(1, fd.added),
        added: fd.added,
        deleted: fd.deleted,
        pseudocode: {
          input: `รับคำสั่งงาน "${job.instruction}" สำหรับปรับปรุงไฟล์ ${fd.file}`,
          process: `แก้ไขโครงสร้างโค้ดจำนวน +${fd.added} / −${fd.deleted} บรรทัด พร้อมตรวจสอบความถูกต้องผ่าน Reflex Pipeline`,
          output: `ส่งคืนผลลัพธ์การแก้ไขใน ${fd.file} (คลิกเพื่อดูจุดแก้ไขบรรทัดที่ L${startLine})`
        },
        code_snippet: snippet
      };
    });
    board.structural_diff = taskDiffItems;

    // Detect unprompted file changes vs explicitly requested files
    const instrLower = (job.instruction || "").toLowerCase();
    const taskDecisions: BoardDecisionItem[] = [];
    for (const item of taskDiffItems) {
      const baseName = (item.file.split("/").pop() || item.file).toLowerCase();
      const mentioned = instrLower.includes(baseName) || instrLower.includes(item.file.toLowerCase());
      if (!mentioned) {
        taskDecisions.push({
          id: `task-dec-unprompted-${item.file}`,
          kind: "unprompted",
          pinned_alert: true,
          title: `ปักหมุดแจ้งเตือน: Agent แก้ไขไฟล์ ${item.file} เพิ่มเติมโดยไม่ได้ระบุชื่อในคำสั่ง`,
          user_requested: job.instruction,
          agent_decision: `Agent ตัดสินใจแก้ไข ${item.file} (+${item.added} −${item.deleted} บรรทัด) เพื่อให้สอดคล้องกับโครงสร้างระบบ`,
          rationale:
            job.report?.summary ||
            "ตรวจพบใน Trace ว่าต้องปรับไฟล์ที่เชื่อมโยงกันเพื่อให้การคอมไพล์และชุดทดสอบผ่านสมบูรณ์",
          file: item.file,
          line: item.start_line,
          symbol: item.symbol
        });
      } else {
        taskDecisions.push({
          id: `task-dec-req-${item.file}`,
          kind: "requested",
          pinned_alert: false,
          title: `ทำตามคำสั่งผู้ใช้: แก้ไข ${item.file}`,
          user_requested: job.instruction,
          agent_decision: `ดำเนินการแก้ไข ${item.file} ตามที่ระบุในคำสั่งโดยตรง`,
          rationale: job.report?.summary || "ตรงตามขอบเขตงานที่ผู้ใช้สั่ง",
          file: item.file,
          line: item.start_line,
          symbol: item.symbol
        });
      }
    }
    if (taskDecisions.length > 0) {
      board.decision_log = [...taskDecisions, ...board.decision_log.slice(0, 2)];
    }

    // Bind Sequence Diagram nodes to the distinct files modified by this task
    if (taskDiffItems.length >= 2) {
      const nodeIds = ["node-mcp", "node-ast", "node-diff", "node-binding"];
      const roles = ["Task Entry", "Core Module", "Linked Module", "Verification / Output"];
      board.diagram.sequence_nodes = nodeIds.map((nid, idx) => {
        const item = taskDiffItems[idx % taskDiffItems.length];
        const baseFile = item.file.split("/").pop() || item.file;
        return {
          id: nid,
          label: `${baseFile} (+${item.added} −${item.deleted})`,
          role: `${roles[idx]} · ${item.file}:L${item.start_line}`,
          file: item.file,
          start_line: item.start_line,
          end_line: item.end_line,
          description: `แก้ไขในงาน "${job.instruction}" (${item.category_label}) คลิกเพื่อเปิดไฟล์ ${item.file} บรรทัดที่ L${item.start_line}`
        };
      });
    }
  }

  if (events.length > 0) {
    const diffPool =
      board.structural_diff.length > 0
        ? board.structural_diff
        : [{ file: "internal/desktop/server.go", start_line: 1 }];
    const eventNotes: BoardTraceNote[] = events.slice(-4).map((ev, i) => {
      const target = diffPool[i % diffPool.length];
      return {
        id: `task-trace-${ev.seq || i}`,
        stage: ev.stage || "trace",
        title: `Trace [${ev.stage}]: งาน ${job.id}`,
        summary: ev.message,
        file: target.file,
        line: target.start_line,
        timestamp: `Seq #${ev.seq}`
      };
    });
    board.diagram.trace_notes = [...eventNotes, ...board.diagram.trace_notes.slice(0, 2)];
  }

  return board;
}
