<script lang="ts">
  import {
    ArrowLeft,
    GitBranch,
    Lock,
    Code2,
    Database,
    FileText,
    AlertTriangle,
    Copy,
    Check,
    Send,
    ChevronDown,
    ChevronRight,
    Sliders,
    ShieldCheck,
    Share2,
    Cpu,
    Layers
  } from "lucide-svelte";
  import { agentBoardState, loadAgentBoard } from "../lib/stores/workspace";
  import { selectedTask, taskEvents } from "../lib/stores/tasks";
  import { centerView } from "../lib/stores/settings";
  import {
    deriveBoardForActiveTask,
    formatVisualFeedbackClipboard,
    jumpToBoardCode,
    sendFeedbackToComposer,
    type BoardPluginConfig,
    type HighlightedCanvasElement
  } from "../lib/board";
  import FlowCanvas from "./FlowCanvas.svelte";
  import { t } from "../lib/i18n";

  type BoardTab = "all" | "canvas" | "diff" | "decisions" | "workflow" | "architecture";
  type DiagramMode = "all" | "sequence" | "xyflow" | "er" | "trace";
  type DecisionFilter = "all" | "unprompted" | "requested";

  let activeTab = $state<BoardTab>("all");
  let diagramMode = $state<DiagramMode>("all");
  let decisionFilter = $state<DecisionFilter>("all");

  let pluginPanelOpen = $state(false);
  let pluginConfig = $state<BoardPluginConfig>({
    autoCollapseTests: true,
    autoCollapseDocs: true,
    defaultPseudocodeMode: true,
    highlightUnpromptedPins: true
  });

  // Tracks manual expand/collapse overrides per structural diff item ID or category
  let showRawCodeIds = $state<Record<string, boolean>>({});
  let categoryExpanded = $state<Record<string, boolean>>({
    core: true,
    test: false,
    docs: false
  });

  // Visual Feedback Loop state (Spec 3.1)
  let selectedCanvasItems = $state<HighlightedCanvasElement[]>([]);
  let feedbackComment = $state("");
  let copiedFeedback = $state(false);
  let copiedSnapshot = $state(false);
  let copiedTemplateId = $state("");
  let lastBoundJump = $state<string>("");

  // Privacy Telemetry Switch (Spec 5)
  let telemetryEnabled = $state(false);

  $effect(() => {
    const taskId = $selectedTask?.id;
    void loadAgentBoard(taskId);
  });

  $effect(() => {
    categoryExpanded.test = !pluginConfig.autoCollapseTests;
    categoryExpanded.docs = !pluginConfig.autoCollapseDocs;
  });

  const board = $derived(
    deriveBoardForActiveTask(
      $agentBoardState,
      $selectedTask,
      $selectedTask ? $taskEvents[$selectedTask.id] || [] : []
    )
  );

  const coreDiffItems = $derived(board.structural_diff.filter((d) => d.category === "core"));
  const testDiffItems = $derived(board.structural_diff.filter((d) => d.category === "test"));
  const docsDiffItems = $derived(board.structural_diff.filter((d) => d.category === "docs"));

  const filteredDecisions = $derived(
    board.decision_log.filter((d) => {
      if (decisionFilter === "unprompted") return d.kind === "unprompted" || d.pinned_alert;
      if (decisionFilter === "requested") return d.kind === "requested" && !d.pinned_alert;
      return true;
    })
  );

  const unpromptedCount = $derived(
    board.decision_log.filter((d) => d.kind === "unprompted" || d.pinned_alert).length
  );

  function isSelectedCanvasItem(id: string): boolean {
    return selectedCanvasItems.some((item) => item.id === id);
  }

  function toggleHighlightElement(el: HighlightedCanvasElement) {
    if (isSelectedCanvasItem(el.id)) {
      selectedCanvasItems = selectedCanvasItems.filter((item) => item.id !== el.id);
    } else {
      selectedCanvasItems = [...selectedCanvasItems, el];
    }
  }

  async function onJumpToCode(file: string, line: number, label: string, preferDiff = false) {
    lastBoundJump = `${label} (${file}:L${line})`;
    await jumpToBoardCode(file, line, {
      preferDiff,
      taskId: $selectedTask?.id
    });
  }

  async function copyVisualFeedback() {
    if (selectedCanvasItems.length === 0) return;
    const text = formatVisualFeedbackClipboard(selectedCanvasItems, feedbackComment);
    try {
      await navigator.clipboard?.writeText(text);
    } catch {
      /* fallback when clipboard API is restricted */
    }
    copiedFeedback = true;
    setTimeout(() => {
      copiedFeedback = false;
    }, 2200);
  }

  function sendVisualFeedbackToAgent() {
    if (selectedCanvasItems.length === 0) return;
    const text = formatVisualFeedbackClipboard(selectedCanvasItems, feedbackComment);
    sendFeedbackToComposer(text);
  }

  async function copySnapshotLink() {
    const snapId = Math.random().toString(36).slice(2, 10);
    const snapText = `apw-board://snapshot/${board.architecture.current_branch}-vs-${board.architecture.base_branch}#${snapId}`;
    try {
      await navigator.clipboard?.writeText(snapText);
    } catch {
      /* ignore */
    }
    copiedSnapshot = true;
    setTimeout(() => {
      copiedSnapshot = false;
    }, 2200);
  }

  async function copyPromptTemplate(id: string, promptText: string) {
    try {
      await navigator.clipboard?.writeText(promptText);
    } catch {
      /* ignore */
    }
    copiedTemplateId = id;
    setTimeout(() => {
      if (copiedTemplateId === id) copiedTemplateId = "";
    }, 2000);
  }

  function toggleRawCode(id: string) {
    const cur = showRawCodeIds[id] ?? !pluginConfig.defaultPseudocodeMode;
    showRawCodeIds = { ...showRawCodeIds, [id]: !cur };
  }

  function isRawCodeShown(id: string): boolean {
    return showRawCodeIds[id] ?? !pluginConfig.defaultPseudocodeMode;
  }
</script>

<main class="board-view" data-testid="agent-board-view">
  <header class="board-header">
    <div class="hdr-left">
      <button
        type="button"
        class="back-btn"
        onclick={() => centerView.set("thread")}
        data-testid="board-back-btn"
      >
        <ArrowLeft size={14} />
        <span>{$t("Back to Conversation", "กลับไปที่บทสนทนา")}</span>
      </button>

      <div class="title-block">
        <div class="title-row">
          <h2>Agent Board</h2>
          <span class="ro-badge mono" data-testid="board-readonly-badge">
            <Lock size={11} />
            <span>READ-ONLY REVIEW</span>
          </span>
          <span class="branch-badge mono" data-testid="board-branch-badge">
            <GitBranch size={12} />
            <span>{board.architecture.current_branch} → {board.architecture.base_branch}</span>
          </span>
        </div>
        <p class="sub">
          {$t(
            "Local-first Code & Diagram Review Board (Code OSS + Rust AST Diff + MCP Tooling) · Click any diagram element to jump to live code",
            "กระดานตรวจโค้ดและแผนภาพแบบ Local-first (Code OSS + Rust AST Diff + MCP Tooling) · คลิกส่วนใดของแผนภาพเพื่อเปิดโค้ดจริงด้านขวา"
          )}
        </p>
      </div>
    </div>

    <div class="hdr-actions">
      <button
        type="button"
        class="hdr-btn"
        class:active={pluginPanelOpen}
        onclick={() => (pluginPanelOpen = !pluginPanelOpen)}
        data-testid="board-plugin-btn"
      >
        <Sliders size={13} />
        <span>{$t("Display Plugins", "Plugin ตั้งค่าแสดงผล")}</span>
      </button>

      <button
        type="button"
        class="hdr-btn"
        onclick={copySnapshotLink}
        title={$t("Share a point-in-time review snapshot", "แชร์รีวิวแบบภาพนิ่ง ณ เวลาปัจจุบัน (Snapshot)")}
        data-testid="board-snapshot-btn"
      >
        {#if copiedSnapshot}
          <Check size={13} />
          <span>{$t("Snapshot Copied", "คัดลอก Snapshot แล้ว")}</span>
        {:else}
          <Share2 size={13} />
          <span>{$t("Share Snapshot", "แชร์ Snapshot")}</span>
        {/if}
      </button>
    </div>
  </header>

  {#if pluginPanelOpen}
    <section class="plugin-bar" data-testid="board-plugin-panel">
      <div class="plugin-title">
        <strong>{$t("Display Plugin System:", "ระบบปลั๊กอินปรับแต่งการแสดงผล (Display Plugin System):")}</strong>
        <span class="muted">{$t(
          "Customize default behavior of the board and structural code review",
          "ปรับแก้ค่าเริ่มต้นของกระดานและหน้าต่างตรวจโค้ดได้ทั้งหมด"
        )}</span>
      </div>
      <div class="plugin-toggles">
        <label class="chk">
          <input type="checkbox" bind:checked={pluginConfig.autoCollapseTests} />
          <span>{$t("Auto-collapse test files (Tests)", "พับเก็บไฟล์เทสต์ (Tests) อัตโนมัติ")}</span>
        </label>
        <label class="chk">
          <input type="checkbox" bind:checked={pluginConfig.autoCollapseDocs} />
          <span>{$t("Auto-collapse documentation files (Docs)", "พับเก็บไฟล์เอกสาร (Docs) อัตโนมัติ")}</span>
        </label>
        <label class="chk">
          <input type="checkbox" bind:checked={pluginConfig.defaultPseudocodeMode} />
          <span>{$t("Summarize new functions into 3-step Pseudocode by default", "ย่อฟังก์ชันใหม่เป็น Pseudocode 3 ส่วนเป็นค่าเริ่มต้น")}</span>
        </label>
        <label class="chk">
          <input type="checkbox" bind:checked={pluginConfig.highlightUnpromptedPins} />
          <span>{$t("Highlight Red-Pin alerts for unprompted Agent changes", "เน้นหมุดแดงแจ้งเตือนสิ่งที่ Agent ตัดสินใจทำเอง")}</span>
        </label>
      </div>
    </section>
  {/if}

  {#if lastBoundJump}
    <div class="binding-banner" data-testid="board-binding-banner">
      <Code2 size={13} />
      <span>{$t("Code-to-Diagram Binding opened in right panel:", "Code-to-Diagram Binding เปิดไฟล์ในพาเนลด้านขวาแล้ว:")} <strong class="mono">{lastBoundJump}</strong></span>
    </div>
  {/if}

  <nav class="board-tabs" aria-label={$t("Agent Board sections and pillars", "หมวดหมู่หลักการและฟีเจอร์ของ Agent Board")}>
    <button
      type="button"
      class="tab-btn"
      class:active={activeTab === "all"}
      onclick={() => (activeTab = "all")}
      data-testid="board-tab-all"
    >
      {$t("All 5 Pillars Overview", "ภาพรวมครบทั้ง 5 หลักการ")}
    </button>
    <button
      type="button"
      class="tab-btn"
      class:active={activeTab === "canvas"}
      onclick={() => (activeTab = "canvas")}
      data-testid="board-tab-canvas"
    >
      1. Interactive Diagram Canvas
    </button>
    <button
      type="button"
      class="tab-btn"
      class:active={activeTab === "diff"}
      onclick={() => (activeTab = "diff")}
      data-testid="board-tab-diff"
    >
      2. Structural Diff & Pseudocode ({board.structural_diff.length})
    </button>
    <button
      type="button"
      class="tab-btn"
      class:active={activeTab === "decisions"}
      onclick={() => (activeTab = "decisions")}
      data-testid="board-tab-decisions"
    >
      3. Decision Log
      {#if unpromptedCount > 0}
        <span class="pin-count">{unpromptedCount} {$t("Red Pins", "หมุดแดง")}</span>
      {/if}
    </button>
    <button
      type="button"
      class="tab-btn"
      class:active={activeTab === "workflow"}
      onclick={() => (activeTab = "workflow")}
      data-testid="board-tab-workflow"
    >
      {$t("4. Workflow & 3-Step Prompts", "4. เวิร์กโฟลว์ & Prompt 3 ขั้น")}
    </button>
    <button
      type="button"
      class="tab-btn"
      class:active={activeTab === "architecture"}
      onclick={() => (activeTab = "architecture")}
      data-testid="board-tab-architecture"
    >
      {$t("5. Architecture, MCP & Privacy", "5. สถาปัตยกรรม, MCP & Privacy")}
    </button>
  </nav>

  <div class="board-scroll">
    <div class="board-container">
      <!-- 1. INTERACTIVE DIAGRAM CANVAS (SPEC 3.1) -->
      {#if activeTab === "all" || activeTab === "canvas"}
        <section class="board-sec" data-testid="section-diagram-canvas">
          <div class="sec-hdr">
            <div>
              <span class="sec-eyebrow mono">SPEC 3.1 · INTERACTIVE DIAGRAM CANVAS</span>
              <h3>{$t(
                "Interactive Diagram Canvas (Click to jump to live code · Highlight to copy feedback)",
                "กระดานแผนภาพแบบโต้ตอบได้ (คลิกเพื่อเปิดโค้ดจริง · ไฮไลต์เพื่อคัดลอกสั่งแก้)"
              )}</h3>
            </div>
            <div class="pill-group" role="group" aria-label={$t("Switch diagram mode", "สลับชนิดแผนภาพ")}>
              <button
                type="button"
                class="pill-btn"
                class:active={diagramMode === "all"}
                onclick={() => (diagramMode = "all")}
              >
                {$t("All", "ทั้งหมด")}
              </button>
              <button
                type="button"
                class="pill-btn"
                class:active={diagramMode === "sequence"}
                onclick={() => (diagramMode = "sequence")}
                data-testid="pill-mode-sequence"
              >
                Sequence Lifeline
              </button>
              <button
                type="button"
                class="pill-btn"
                class:active={diagramMode === "xyflow"}
                onclick={() => (diagramMode = "xyflow")}
                data-testid="pill-mode-xyflow"
              >
                XYFlow Node Graph
              </button>
              <button
                type="button"
                class="pill-btn"
                class:active={diagramMode === "er"}
                onclick={() => (diagramMode = "er")}
              >
                ER Diagram
              </button>
              <button
                type="button"
                class="pill-btn"
                class:active={diagramMode === "trace"}
                onclick={() => (diagramMode = "trace")}
              >
                Trace Notes
              </button>
            </div>
          </div>

          <!-- Visual Sequence Lifeline Diagram + XYFlow / Svelte Flow Interactive Node Canvas -->
          {#if diagramMode === "all" || diagramMode === "sequence" || diagramMode === "xyflow"}
            <FlowCanvas
              diagram={board.diagram}
              decisions={board.decision_log}
              {diagramMode}
              selectedItems={selectedCanvasItems}
              onToggleHighlight={toggleHighlightElement}
              onJumpToCode={onJumpToCode}
            />
          {/if}

          <!-- ER Diagram -->
          {#if diagramMode === "all" || diagramMode === "er"}
            <div class="canvas-block">
              <div class="canvas-block-hdr">
                <strong>{$t(
                  "ER Diagram: Database Schema & Trace Storage Relationships",
                  "ER Diagram: ความสัมพันธ์ของตารางฐานข้อมูลและโครงสร้างบันทึก Trace"
                )}</strong>
                <span class="hint">{$t(
                  "Displays schema and 1:N relationships bound to source files",
                  "แสดง Schema และความสัมพันธ์ 1:N พร้อมผูกไฟล์ต้นทาง"
                )}</span>
              </div>

              <div class="er-grid">
                {#each board.diagram.er_tables as tbl (tbl.id)}
                  <div
                    class="canvas-card er-card"
                    class:highlighted={isSelectedCanvasItem(tbl.id)}
                    data-testid={`er-table-${tbl.id}`}
                  >
                    <div class="card-top">
                      <span class="er-name mono">
                        <Database size={12} />
                        <span>{tbl.name}</span>
                      </span>
                      <button
                        type="button"
                        class="hl-toggle"
                        class:active={isSelectedCanvasItem(tbl.id)}
                        onclick={() =>
                          toggleHighlightElement({
                            id: tbl.id,
                            kind: "er-table",
                            label: `Table ${tbl.name}`,
                            file: tbl.file,
                            line: tbl.line,
                            detail: tbl.description
                          })}
                      >
                        {isSelectedCanvasItem(tbl.id)
                          ? $t("✓ Highlighted", "✓ ไฮไลต์แล้ว")
                          : $t("+ Highlight", "+ ไฮไลต์")}
                      </button>
                    </div>
                    <p class="node-desc">{tbl.description}</p>
                    <div class="er-cols mono">
                      {#each tbl.columns as col (col.name)}
                        <div class="er-col-row">
                          <span class="col-name">{col.name}</span>
                          <span class="col-type">{col.type}</span>
                          {#if col.key}
                            <span class="col-key">{col.key}</span>
                          {/if}
                        </div>
                      {/each}
                    </div>
                    <button
                      type="button"
                      class="code-bind-btn mono"
                      onclick={() => onJumpToCode(tbl.file, tbl.line, tbl.name)}
                    >
                      <Code2 size={12} />
                      <span>{tbl.file}:L{tbl.line}</span>
                      <span class="jump-arrow">{$t("Open Code →", "เปิดโค้ด →")}</span>
                    </button>
                  </div>
                {/each}
              </div>

              <div class="er-relations">
                {#each board.diagram.er_relations as rel (rel.id)}
                  <div class="seq-edge-row">
                    <span class="edge-step mono">{rel.cardinality}</span>
                    <div class="edge-main">
                      <strong class="mono">{rel.from_table} ──► {rel.to_table}</strong>
                      <span class="muted"> · {rel.label}</span>
                    </div>
                    <button
                      type="button"
                      class="code-bind-btn compact mono"
                      onclick={() => onJumpToCode(rel.file, rel.line, `${rel.from_table}->${rel.to_table}`)}
                    >
                      <Code2 size={11} />
                      <span>{rel.file}:L{rel.line}</span>
                    </button>
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          <!-- Trace Notes -->
          {#if diagramMode === "all" || diagramMode === "trace"}
            <div class="canvas-block">
              <div class="canvas-block-hdr">
                <strong>{$t(
                  "Trace Notes: Summary Notes from Agent Execution & Decisions",
                  "Trace Notes: โน้ตสรุปจากบันทึกการทำงานและการตัดสินใจของ Agent"
                )}</strong>
                <span class="hint">{$t(
                  "Extracts Agent trace events into code-bound summary cards",
                  "ดึงข้อมูลจาก Trace ของ Agent มาแปะเป็นโน้ตสรุปพร้อมอ้างอิงโค้ด"
                )}</span>
              </div>

              <div class="trace-grid">
                {#each board.diagram.trace_notes as note (note.id)}
                  <div
                    class="canvas-card trace-card"
                    class:highlighted={isSelectedCanvasItem(note.id)}
                    data-testid={`trace-note-${note.id}`}
                  >
                    <div class="card-top">
                      <span class="role-tag mono">{note.timestamp} · {note.stage}</span>
                      <button
                        type="button"
                        class="hl-toggle"
                        class:active={isSelectedCanvasItem(note.id)}
                        onclick={() =>
                          toggleHighlightElement({
                            id: note.id,
                            kind: "trace-note",
                            label: note.title,
                            file: note.file,
                            line: note.line,
                            detail: note.summary
                          })}
                      >
                        {isSelectedCanvasItem(note.id)
                          ? $t("✓ Highlighted", "✓ ไฮไลต์แล้ว")
                          : $t("+ Highlight", "+ ไฮไลต์")}
                      </button>
                    </div>
                    <div class="node-title">{note.title}</div>
                    <p class="node-desc">{note.summary}</p>
                    <button
                      type="button"
                      class="code-bind-btn mono"
                      onclick={() => onJumpToCode(note.file, note.line, note.title)}
                    >
                      <Code2 size={12} />
                      <span>{note.file}:L{note.line}</span>
                      <span class="jump-arrow">{$t("Open Code →", "เปิดโค้ด →")}</span>
                    </button>
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          <!-- Visual Feedback Loop Bar (Highlight -> Copy to Clipboard / Paste to Agent) -->
          <div class="feedback-dock" data-testid="visual-feedback-dock">
            <div class="feedback-hdr">
              <div>
                <strong>{$t(
                  "Visual Feedback Loop (Highlight elements to request changes):",
                  "Visual Feedback Loop (ไฮไลต์ส่วนที่ไม่ชอบเพื่อสั่งแก้):"
                )}</strong>
                <span class="muted">
                  {#if selectedCanvasItems.length === 0}
                    {$t(
                      "Click '+ Highlight' on any diagram card above to collect code context and send instructions to the Agent",
                      "กดปุ่ม \"+ ไฮไลต์สั่งแก้\" บนกล่องแผนภาพด้านบน ระบบจะรวบรวมบริบทและตำแหน่งโค้ดเพื่อคัดลอกไปสั่ง Agent"
                    )}
                  {:else}
                    {$t(
                      `${selectedCanvasItems.length} item(s) selected on board · Ready to copy context to Clipboard`,
                      `เลือกไว้ ${selectedCanvasItems.length} รายการบนกระดาน พร้อมคัดลอกบริบทไปยัง Clipboard`
                    )}
                  {/if}
                </span>
              </div>
              {#if selectedCanvasItems.length > 0}
                <button
                  type="button"
                  class="clear-hl-btn"
                  onclick={() => (selectedCanvasItems = [])}
                >
                  {$t("Clear selection", "ล้างที่เลือก")}
                </button>
              {/if}
            </div>

            {#if selectedCanvasItems.length > 0}
              <div class="selected-chips mono">
                {#each selectedCanvasItems as item (item.id)}
                  <span class="sel-chip">
                    {item.label} ({item.file}:L{item.line})
                  </span>
                {/each}
              </div>

              <div class="feedback-controls">
                <input
                  type="text"
                  bind:value={feedbackComment}
                  placeholder={$t(
                    "Describe what the Agent should redraw or refactor (leave blank for default prompt)...",
                    "ระบุสิ่งที่ต้องการให้ Agent วาดใหม่หรือปรับดีไซน์ (เว้นว่างเพื่อใช้ข้อความมาตรฐาน)..."
                  )}
                  data-testid="visual-feedback-input"
                />
                <button
                  type="button"
                  class="btn"
                  onclick={copyVisualFeedback}
                  data-testid="copy-visual-feedback-btn"
                >
                  {#if copiedFeedback}
                    <Check size={13} />
                    <span>{$t("Copied to Clipboard!", "คัดลอกลง Clipboard แล้ว!")}</span>
                  {:else}
                    <Copy size={13} />
                    <span>{$t("Copy Context to Clipboard", "คัดลอกบริบทไปวางสั่งแก้")}</span>
                  {/if}
                </button>
                <button
                  type="button"
                  class="btn primary"
                  onclick={sendVisualFeedbackToAgent}
                  data-testid="send-visual-feedback-btn"
                >
                  <Send size={13} />
                  <span>{$t("Send to Agent Composer", "ส่งเข้าช่องแชต Agent")}</span>
                </button>
              </div>
            {/if}
          </div>
        </section>
      {/if}

      <!-- 2. STRUCTURAL DIFF & PSEUDOCODE (SPEC 3.2) -->
      {#if activeTab === "all" || activeTab === "diff"}
        <section class="board-sec" data-testid="section-structural-diff">
          <div class="sec-hdr">
            <div>
              <span class="sec-eyebrow mono">SPEC 3.2 · STRUCTURAL DIFF & PSEUDOCODE</span>
              <h3>{$t(
                "Smart Structural Diff (Separates Core / Tests / Docs · 3-Step Pseudocode Summary)",
                "หน้าต่างตรวจโค้ดอัจฉริยะ (แยกแยะ Core / Tests / Docs · ย่อเป็น Pseudocode 3 ส่วน)"
              )}</h3>
            </div>
            <span class="engine-pill mono">Rust / AST Structural Diff</span>
          </div>

          <!-- Category 1: Core Logic (Expanded by default) -->
          <div class="diff-cat-group" data-testid="diff-group-core">
            <div class="diff-cat-hdr">
              <button
                type="button"
                class="cat-toggle-btn"
                onclick={() => (categoryExpanded.core = !categoryExpanded.core)}
              >
                {#if categoryExpanded.core}
                  <ChevronDown size={14} />
                {:else}
                  <ChevronRight size={14} />
                {/if}
                <strong>{$t("1. Core Logic & New Functions", "1. โค้ดหลักและฟังก์ชันใหม่ (Core Logic)")}</strong>
                <span class="cat-count mono">{coreDiffItems.length} {$t(
                  "items · Expanded with Pseudocode by default",
                  "รายการ · กางแสดง Pseudocode เป็นค่าเริ่มต้น"
                )}</span>
              </button>
            </div>

            {#if categoryExpanded.core}
              <div class="sdiff-list">
                {#each coreDiffItems as item (item.id)}
                  <div class="sdiff-card" data-testid={`sdiff-item-${item.id}`}>
                    <div class="sdiff-top">
                      <div class="sdiff-title">
                        <span class="mono sym-name">{item.symbol}</span>
                        <span class="mono file-path">{item.file}:L{item.start_line}–L{item.end_line}</span>
                        <span class="diff-stat mono">
                          <span class="add">+{item.added}</span>
                          <span class="del">−{item.deleted}</span>
                        </span>
                      </div>
                      <div class="sdiff-actions">
                        <button
                          type="button"
                          class="mini-btn"
                          onclick={() => toggleRawCode(item.id)}
                        >
                          {isRawCodeShown(item.id)
                            ? $t("View 3-Step Pseudocode", "ดู Pseudocode 3 ส่วน")
                            : $t("View Raw Code Snippet", "ดูโค้ดจริงย่อ")}
                        </button>
                        <button
                          type="button"
                          class="code-bind-btn compact mono"
                          onclick={() => onJumpToCode(item.file, item.start_line, item.symbol)}
                        >
                          <Code2 size={11} />
                          <span>{$t("Open Code", "เปิดโค้ด")} L{item.start_line} →</span>
                        </button>
                      </div>
                    </div>

                    {#if !isRawCodeShown(item.id)}
                      <div class="pseudocode-grid" data-testid={`pseudocode-${item.id}`}>
                        <div class="pseudo-step">
                          <span class="pseudo-label mono">{$t("1. INPUT", "1. INPUT (รับอะไรเข้ามา)")}</span>
                          <p>{item.pseudocode.input}</p>
                        </div>
                        <div class="pseudo-step">
                          <span class="pseudo-label mono">{$t("2. PROCESS / VALIDATION", "2. PROCESS / VALIDATION (ตรวจและประมวลผลอะไร)")}</span>
                          <p>{item.pseudocode.process}</p>
                        </div>
                        <div class="pseudo-step">
                          <span class="pseudo-label mono">{$t("3. OUTPUT", "3. OUTPUT (ส่งอะไรกลับออกไป)")}</span>
                          <p>{item.pseudocode.output}</p>
                        </div>
                      </div>
                    {:else}
                      <pre class="snippet-box mono selectable">{item.code_snippet}</pre>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>

          <!-- Category 2: Tests (Auto-collapsed by default per Spec 3.2) -->
          <div class="diff-cat-group collapsed-group" data-testid="diff-group-test">
            <div class="diff-cat-hdr">
              <button
                type="button"
                class="cat-toggle-btn"
                onclick={() => (categoryExpanded.test = !categoryExpanded.test)}
                data-testid="toggle-test-group-btn"
              >
                {#if categoryExpanded.test}
                  <ChevronDown size={14} />
                {:else}
                  <ChevronRight size={14} />
                {/if}
                <strong>{$t(
                  `2. Test Suites (Tests) · ${testDiffItems.length} files`,
                  `2. ชุดทดสอบ (Tests) · ${testDiffItems.length} ไฟล์`
                )}</strong>
                <span class="auto-collapse-pill mono">
                  {categoryExpanded.test
                    ? $t("Expanded", "กางออกแล้ว")
                    : $t("+ Auto-collapsed (Click + to expand details)", "+ พับเก็บอัตโนมัติ (คลิก + เพื่อกางดูรายละเอียด)")}
                </span>
              </button>
            </div>

            {#if categoryExpanded.test}
              <div class="sdiff-list">
                {#each testDiffItems as item (item.id)}
                  <div class="sdiff-card">
                    <div class="sdiff-top">
                      <div class="sdiff-title">
                        <span class="mono sym-name">{item.symbol}</span>
                        <span class="mono file-path">{item.file}:L{item.start_line}</span>
                        <span class="diff-stat mono">
                          <span class="add">+{item.added}</span>
                          <span class="del">−{item.deleted}</span>
                        </span>
                      </div>
                      <button
                        type="button"
                        class="code-bind-btn compact mono"
                        onclick={() => onJumpToCode(item.file, item.start_line, item.symbol)}
                      >
                        <Code2 size={11} />
                        <span>{$t("Open Test File →", "เปิดไฟล์เทสต์ →")}</span>
                      </button>
                    </div>
                    <div class="pseudocode-grid">
                      <div class="pseudo-step">
                        <span class="pseudo-label mono">INPUT</span>
                        <p>{item.pseudocode.input}</p>
                      </div>
                      <div class="pseudo-step">
                        <span class="pseudo-label mono">VALIDATION</span>
                        <p>{item.pseudocode.process}</p>
                      </div>
                      <div class="pseudo-step">
                        <span class="pseudo-label mono">OUTPUT</span>
                        <p>{item.pseudocode.output}</p>
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>

          <!-- Category 3: Documentation (Auto-collapsed by default per Spec 3.2) -->
          <div class="diff-cat-group collapsed-group" data-testid="diff-group-docs">
            <div class="diff-cat-hdr">
              <button
                type="button"
                class="cat-toggle-btn"
                onclick={() => (categoryExpanded.docs = !categoryExpanded.docs)}
                data-testid="toggle-docs-group-btn"
              >
                {#if categoryExpanded.docs}
                  <ChevronDown size={14} />
                {:else}
                  <ChevronRight size={14} />
                {/if}
                <strong>{$t(
                  `3. Documentation · ${docsDiffItems.length} files`,
                  `3. เอกสาร (Documentation) · ${docsDiffItems.length} ไฟล์`
                )}</strong>
                <span class="auto-collapse-pill mono">
                  {categoryExpanded.docs
                    ? $t("Expanded", "กางออกแล้ว")
                    : $t("+ Auto-collapsed (Click + to expand details)", "+ พับเก็บอัตโนมัติ (คลิก + เพื่อกางดูรายละเอียด)")}
                </span>
              </button>
            </div>

            {#if categoryExpanded.docs}
              <div class="sdiff-list">
                {#each docsDiffItems as item (item.id)}
                  <div class="sdiff-card">
                    <div class="sdiff-top">
                      <div class="sdiff-title">
                        <FileText size={13} />
                        <span class="mono sym-name">{item.symbol}</span>
                        <span class="mono file-path">{item.file}:L{item.start_line}</span>
                      </div>
                      <button
                        type="button"
                        class="code-bind-btn compact mono"
                        onclick={() => onJumpToCode(item.file, item.start_line, item.symbol)}
                      >
                        <Code2 size={11} />
                        <span>{$t("Open Doc →", "เปิดเอกสาร →")}</span>
                      </button>
                    </div>
                    <div class="pseudocode-grid">
                      <div class="pseudo-step">
                        <span class="pseudo-label mono">INPUT</span>
                        <p>{item.pseudocode.input}</p>
                      </div>
                      <div class="pseudo-step">
                        <span class="pseudo-label mono">PROCESS</span>
                        <p>{item.pseudocode.process}</p>
                      </div>
                      <div class="pseudo-step">
                        <span class="pseudo-label mono">OUTPUT</span>
                        <p>{item.pseudocode.output}</p>
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </section>
      {/if}

      <!-- 3. DECISION LOG & RED PIN ALERTS (SPEC 3.3) -->
      {#if activeTab === "all" || activeTab === "decisions"}
        <section class="board-sec" data-testid="section-decision-log">
          <div class="sec-hdr">
            <div>
              <span class="sec-eyebrow mono">SPEC 3.3 · DECISION LOG & UNPROMPTED CHANGE DETECTOR</span>
              <h3>{$t(
                "Decision Log (Separates User-Requested Actions vs. Unprompted Agent Decisions)",
                "ระบบบันทึกการตัดสินใจ (แยก \"สิ่งที่ผู้ใช้สั่ง\" กับ \"สิ่งที่ Agent ตัดสินใจทำไปเองโดยไม่ได้ถาม\")"
              )}</h3>
            </div>
            <div class="pill-group" role="group" aria-label={$t("Decision Log filter", "ตัวกรอง Decision Log")}>
              <button
                type="button"
                class="pill-btn"
                class:active={decisionFilter === "all"}
                onclick={() => (decisionFilter = "all")}
              >
                {$t("All", "ทั้งหมด")} ({board.decision_log.length})
              </button>
              <button
                type="button"
                class="pill-btn"
                class:active={decisionFilter === "unprompted"}
                onclick={() => (decisionFilter = "unprompted")}
                data-testid="filter-unprompted-btn"
              >
                {$t("Red Pins: Unprompted", "หมุดแดง: ตัดสินใจเอง")} ({unpromptedCount})
              </button>
              <button
                type="button"
                class="pill-btn"
                class:active={decisionFilter === "requested"}
                onclick={() => (decisionFilter = "requested")}
              >
                {$t("User Requested", "ตามคำสั่งผู้ใช้")}
              </button>
            </div>
          </div>

          <div class="decision-list">
            {#each filteredDecisions as dec (dec.id)}
              <div
                class="decision-card"
                class:red-pin={dec.pinned_alert && pluginConfig.highlightUnpromptedPins}
                data-testid={`decision-card-${dec.id}`}
              >
                <div class="dec-hdr">
                  <div class="dec-badge-row">
                    {#if dec.pinned_alert}
                      <span class="red-pin-badge" data-testid="red-pin-indicator">
                        <AlertTriangle size={12} />
                        <span>{$t("Red Pin · Agent decided without asking", "ปักหมุดสีแดง · Agent ตัดสินใจทำเพิ่มเองโดยไม่ได้ถาม")}</span>
                      </span>
                    {:else}
                      <span class="req-pin-badge">{$t("✓ Matches user instruction", "✓ ตรงตามคำสั่งผู้ใช้")}</span>
                    {/if}
                    <strong class="dec-title">{dec.title}</strong>
                  </div>

                  <button
                    type="button"
                    class="code-bind-btn compact mono"
                    onclick={() => onJumpToCode(dec.file, dec.line, dec.symbol, true)}
                    data-testid={`decision-jump-${dec.id}`}
                  >
                    <Code2 size={11} />
                    <span>{$t("Open Code/Diff", "เปิดดูโค้ด/Diff")} ({dec.file}:L{dec.line}) →</span>
                  </button>
                </div>

                <div class="dec-compare-grid">
                  <div class="dec-col">
                    <span class="dec-label">{$t("User Instruction:", "สิ่งที่ผู้ใช้สั่ง (User Instruction):")}</span>
                    <p>{dec.user_requested}</p>
                  </div>
                  <div class="dec-col">
                    <span class="dec-label">{$t("Agent Decision in Trace:", "สิ่งที่ Agent ตัดสินใจทำใน Trace:")}</span>
                    <p>{dec.agent_decision}</p>
                  </div>
                  <div class="dec-col">
                    <span class="dec-label">{$t("Trace Rationale:", "เหตุผลเบื้องหลัง (Trace Rationale):")}</span>
                    <p>{dec.rationale}</p>
                  </div>
                </div>
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <!-- 4. WORKFLOW & 3-STEP PROMPT SPECS (SPEC 4) -->
      {#if activeTab === "all" || activeTab === "workflow"}
        <section class="board-sec" data-testid="section-workflow-prompts">
          <div class="sec-hdr">
            <div>
              <span class="sec-eyebrow mono">SPEC 4 · READ-ONLY WORKFLOW & STANDARDIZED PROMPT TEMPLATES</span>
              <h3>{$t(
                "Branch-Level Review Workflow & Standardized 3-Step Prompt Templates",
                "เวิร์กโฟลว์รีวิวระดับ Branch & โครงสร้าง Prompt มาตรฐาน 3 ลำดับขั้น"
              )}</h3>
            </div>
          </div>

          <div class="workflow-banner">
            <div class="wf-item">
              <strong>{$t("1. Read-Only Review Mode", "1. โหมด Read-Only (อ่านและตรวจโค้ดโดยเฉพาะ)")}</strong>
              <p>{$t(
                "Direct code editing on the board is disabled. Highlight elements to instruct the Agent or edit in your IDE.",
                "ไม่อนุญาตให้แก้ไขไฟล์โค้ดโดยตรงบนกระดาน หากต้องการแก้โค้ดให้ไฮไลต์สั่งผ่าน Agent หรือกลับไปแก้ใน IDE ตัวเดิม"
              )}</p>
            </div>
            <div class="wf-item">
              <strong>{$t("2. Branch Review vs Base", "2. การรีวิวระดับกิ่งงาน (Branch Review vs main)")}</strong>
              <p>{$t("Compare working branch", "เปรียบเทียบกิ่งงาน")} (<span class="mono">{board.architecture.current_branch}</span>) {$t(
                "against base branch",
                "ที่ Agent เพิ่งทำเสร็จ เทียบกับกิ่งหลัก"
              )} (<span class="mono">{board.architecture.base_branch}</span>) {$t("before applying or merging.", "ก่อนตัดสินใจกด Apply / Merge")}</p>
            </div>
          </div>

          <div class="tpl-grid">
            {#each board.prompt_templates as tpl (tpl.id)}
              <div class="tpl-card" data-testid={`prompt-tpl-${tpl.id}`}>
                <div class="tpl-hdr">
                  <div>
                    <span class="role-tag mono">{tpl.category}</span>
                    <h4>{tpl.name}</h4>
                    <p class="muted">{tpl.description}</p>
                  </div>
                </div>

                <div class="tpl-steps">
                  {#each tpl.steps as st (st.step)}
                    <div class="tpl-step">
                      <div class="tpl-step-title">{st.title}</div>
                      <p>{st.description}</p>
                      <code class="mono">{st.example}</code>
                    </div>
                  {/each}
                </div>

                <div class="tpl-footer">
                  <button
                    type="button"
                    class="btn"
                    onclick={() => copyPromptTemplate(tpl.id, tpl.prompt_text)}
                  >
                    {#if copiedTemplateId === tpl.id}
                      <Check size={13} />
                      <span>{$t("Prompt Copied", "คัดลอก Prompt แล้ว")}</span>
                    {:else}
                      <Copy size={13} />
                      <span>{$t("Copy Standard Prompt", "คัดลอก Prompt มาตรฐาน")}</span>
                    {/if}
                  </button>
                  <button
                    type="button"
                    class="btn primary"
                    onclick={() => sendFeedbackToComposer(tpl.prompt_text)}
                    data-testid={`use-tpl-${tpl.id}`}
                  >
                    <Send size={13} />
                    <span>{$t("Use in Agent Composer", "นำไฟใช้ในช่องแชต Agent")}</span>
                  </button>
                </div>
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <!-- 5. CORE ARCHITECTURE, MCP INTEGRATION & PRIVACY (SPEC 1, 2, 5) -->
      {#if activeTab === "all" || activeTab === "architecture"}
        <section class="board-sec" data-testid="section-architecture-privacy">
          <div class="sec-hdr">
            <div>
              <span class="sec-eyebrow mono">SPEC 1, 2 & 5 · ARCHITECTURE, MCP TOOLING & PRIVACY CONSTRAINTS</span>
              <h3>{$t(
                "Core Architecture, AI Agent Integration (MCP) & Privacy",
                "สถาปัตยกรรมหลัก, การเชื่อมต่อ AI Agent (MCP) และความเป็นส่วนตัว"
              )}</h3>
            </div>
          </div>

          <div class="arch-grid">
            <div class="arch-card">
              <div class="arch-card-hdr">
                <Layers size={15} />
                <strong>{$t("1. Core Architecture & Tech Stack", "1. สถาปัตยกรรมหลัก (Core Architecture & Tech Stack)")}</strong>
              </div>
              <ul class="arch-list">
                <li><strong>{$t("Application Type:", "ประเภทแอปพลิเคชัน:")}</strong> {board.architecture.app_type} ({board.architecture.license}) · {$t("100% Local Execution", "ประมวลผลในเครื่องผู้ใช้ 100%")}</li>
                <li><strong>{$t("Core Base:", "ฐานของระบบ (Core Base):")}</strong> {board.architecture.core_base} · {$t("Supports VS Code keybindings and Go to Definition", "รองรับคีย์ลัดชุดเดียวกับ VS Code และการกดกระโดดไปดูจุดประกาศฟังก์ชัน (Go to Definition)")}</li>
                <li><strong>{$t("Diff Engine:", "ระบบประมวลผล Diff:")}</strong> {board.architecture.diff_engine}</li>
                <li><strong>{$t("Plugin System:", "ระบบปลั๊กอิน:")}</strong> {board.architecture.plugin_system}</li>
                <li><strong>{$t("Supported OS:", "ระบบปฏิบัติการที่รองรับ:")}</strong> {board.architecture.supported_os.join(" · ")}</li>
              </ul>
            </div>

            <div class="arch-card">
              <div class="arch-card-hdr">
                <Cpu size={15} />
                <strong>{$t("2. AI Agent Connection (Tooling / MCP Integration)", "2. การเชื่อมต่อกับ AI Agent (Tooling / MCP Integration)")}</strong>
              </div>
              <ul class="arch-list">
                <li><strong>{$t("Integration Mode:", "รูปแบบการทำงาน:")}</strong> {board.architecture.integration_mode} · {$t("Acts as a visual diagramming & code-review tool without competing agents", "ทำหน้าที่เป็นเครื่องมือวาดแผนภาพและอธิบายงานบนกระดานโดยไม่ต้องสร้าง Agent ใหม่มาแข่ง")}</li>
                <li><strong>{$t("Recommended Models:", "โมเดลที่แนะนำ (สมดุลความฉลาด ราคา ความเร็ว):")}</strong> <span class="mono">{board.architecture.recommended_models.join(", ")}</span></li>
                <li><strong>{$t("Active Workspace Models:", "โมเดลที่เชื่อมต่อใน Workspace ปัจจุบัน:")}</strong> Architect (<span class="mono">{board.architecture.active_architect}</span>) · Reflex (<span class="mono">{board.architecture.active_reflex}</span>)</li>
                <li><strong>{$t("Trace Access:", "การเข้าถึง Trace:")}</strong> {$t("Allows the Agent to query its own execution trace and reference it directly on the board", "เปิดให้ Agent ค้นหาและดึงข้อมูลจากบันทึกการทำงาน (Trace) ของตัวเองมาอ้างอิงบนกระดานได้โดยตรง")}</li>
              </ul>
            </div>
          </div>

          <div class="privacy-box" data-testid="privacy-telemetry-box">
            <div class="privacy-hdr">
              <div class="privacy-title">
                <ShieldCheck size={16} />
                <div>
                  <strong>{$t(
                    "5. Privacy (Telemetry Redaction) & Beta Constraints",
                    "5. ระบบความเป็นส่วนตัว (Telemetry Redaction) และข้อจำกัดรุ่น Beta"
                  )}</strong>
                  <p class="muted">{$t(
                    "Redacts all sensitive data with an opt-out switch available at all times",
                    "กรองข้อมูลสำคัญออกทั้งหมด และมีสวิตช์ให้ผู้ใช้กดปิดการส่งข้อมูลได้ตลอดเวลา"
                  )}</p>
                </div>
              </div>

              <label class="telemetry-switch" data-testid="telemetry-toggle-label">
                <input
                  type="checkbox"
                  bind:checked={telemetryEnabled}
                  data-testid="telemetry-switch-input"
                />
                <span class="mono">
                  Telemetry: {telemetryEnabled
                    ? $t("ON (System status only, no code)", "ON (เฉพาะสถานะระบบ ไม่ส่งโค้ด)")
                    : $t("OFF (All telemetry disabled)", "OFF (ปิดการส่งข้อมูลทั้งหมด)")}
                </span>
              </label>
            </div>

            <div class="privacy-cols">
              <div class="priv-col">
                <span class="priv-sub">{$t("Strictly Redacted Fields:", "รายการข้อมูลที่ห้ามส่งออกนอกเครื่องโดยเด็ดขาด (Strictly Redacted):")}</span>
                <div class="redacted-chips">
                  {#each board.privacy.redacted_fields as field (field)}
                    <span class="redacted-chip mono">{$t("✕ Redacted:", "✕ ห้ามส่ง:")} {field}</span>
                  {/each}
                </div>
              </div>

              <div class="priv-col">
                <span class="priv-sub">{$t("Beta Technical Constraints:", "ข้อจำกัดทางเทคนิคในรุ่นปัจจุบัน (Beta Constraints):")}</span>
                <ul class="constraint-list">
                  {#each board.privacy.beta_constraints as c (c)}
                    <li>{c}</li>
                  {/each}
                </ul>
              </div>
            </div>
          </div>
        </section>
      {/if}
    </div>
  </div>
</main>

<style>
  .board-view {
    flex: 1;
    height: 100%;
    background: var(--bg);
    display: flex;
    flex-direction: column;
    min-width: 0;
    color: var(--text);
  }
  .board-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 18px;
    border-bottom: 1px solid var(--line);
    background: var(--panel);
    flex-wrap: wrap;
  }
  .hdr-left {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    min-width: 0;
    flex: 1;
  }
  .back-btn,
  .hdr-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 10px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
    font-weight: 500;
    flex: none;
  }
  .back-btn:hover,
  .hdr-btn:hover,
  .hdr-btn.active {
    background: var(--hover);
    border-color: var(--line-strong);
  }
  .title-block {
    min-width: 0;
    flex: 1;
  }
  .title-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .title-row h2 {
    margin: 0;
    font-size: 16px;
    font-weight: 700;
  }
  .ro-badge,
  .branch-badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 10.5px;
    padding: 2px 7px;
    border-radius: 5px;
    font-weight: 600;
  }
  .ro-badge {
    background: var(--accent-soft);
    color: var(--accent);
    border: 1px solid var(--line);
  }
  .branch-badge {
    background: var(--panel-subtle);
    color: var(--text);
    border: 1px solid var(--line);
  }
  .sub {
    margin: 3px 0 0;
    font-size: 12px;
    color: var(--muted);
  }
  .hdr-actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .plugin-bar {
    padding: 10px 18px;
    background: var(--panel-subtle);
    border-bottom: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 12px;
  }
  .plugin-title {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .plugin-toggles {
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
  }
  .chk {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
  }
  .binding-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 18px;
    background: var(--accent-soft);
    color: var(--accent);
    border-bottom: 1px solid var(--line);
    font-size: 12px;
  }
  .board-tabs {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 6px 18px;
    border-bottom: 1px solid var(--line);
    background: var(--panel-subtle);
    overflow-x: auto;
  }
  .tab-btn {
    border: 1px solid transparent;
    background: transparent;
    color: var(--muted);
    font-size: 12px;
    font-weight: 500;
    padding: 5px 10px;
    border-radius: 6px;
    white-space: nowrap;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .tab-btn:hover {
    color: var(--text);
    background: var(--hover);
  }
  .tab-btn.active {
    color: var(--accent);
    background: var(--panel);
    border-color: var(--line);
    font-weight: 600;
  }
  .pin-count {
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--err-soft);
    color: var(--err);
    font-weight: 700;
  }
  .board-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 18px;
  }
  .board-container {
    max-width: 980px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .board-sec {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .sec-hdr {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
  }
  .sec-eyebrow {
    font-size: 10.5px;
    color: var(--accent);
    font-weight: 700;
  }
  .sec-hdr h3 {
    margin: 2px 0 0;
    font-size: 14.5px;
  }
  .pill-group {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }
  .pill-btn {
    padding: 4px 9px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--muted);
    font-size: 11.5px;
  }
  .pill-btn.active {
    background: var(--accent);
    color: var(--accent-fg);
    border-color: var(--accent);
    font-weight: 600;
  }
  .canvas-block {
    border: 1px solid var(--line);
    border-radius: 8px;
    padding: 12px;
    background: var(--bg);
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .canvas-block-hdr {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    font-size: 12.5px;
  }
  .hint {
    font-size: 11px;
    color: var(--muted);
  }
  .er-grid,
  .trace-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 10px;
  }
  .canvas-card {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--panel);
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  .canvas-card.highlighted {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
  }
  .role-tag {
    font-size: 10.5px;
    color: var(--muted);
    font-weight: 600;
  }
  .node-title {
    font-weight: 600;
    font-size: 13px;
  }
  .node-desc {
    margin: 0;
    font-size: 11.5px;
    color: var(--muted);
    line-height: 1.5;
    flex: 1;
  }
  .hl-toggle {
    padding: 2px 7px;
    border-radius: 5px;
    border: 1px solid var(--line-strong);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 10.5px;
    font-weight: 500;
  }
  .hl-toggle.active {
    background: var(--accent);
    color: var(--accent-fg);
    border-color: var(--accent);
  }
  .code-bind-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 8px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11px;
    text-align: left;
    justify-content: space-between;
  }
  .code-bind-btn.compact {
    padding: 3px 8px;
  }
  .code-bind-btn:hover {
    border-color: var(--accent);
    color: var(--accent);
  }
  .jump-arrow {
    color: var(--accent);
    font-weight: 600;
    margin-left: auto;
  }
  .er-relations {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .seq-edge-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    font-size: 12px;
    flex-wrap: wrap;
  }
  .edge-step {
    font-size: 11px;
    font-weight: 700;
    color: var(--accent);
    padding: 1px 6px;
    border-radius: 4px;
    background: var(--accent-soft);
  }
  .edge-main {
    flex: 1;
    min-width: 180px;
  }
  .er-name {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-weight: 700;
    font-size: 12.5px;
    color: var(--accent);
  }
  .er-cols {
    border-top: 1px solid var(--line);
    padding-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 11px;
  }
  .er-col-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
  }
  .col-type {
    color: var(--muted);
  }
  .col-key {
    font-size: 9.5px;
    padding: 0 5px;
    border-radius: 4px;
    background: var(--panel-subtle);
    border: 1px solid var(--line);
    font-weight: 600;
  }
  .feedback-dock {
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    padding: 10px 12px;
    background: var(--panel-subtle);
    display: flex;
    flex-direction: column;
    gap: 8px;
    font-size: 12px;
  }
  .feedback-hdr {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
  }
  .clear-hl-btn {
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    font-size: 11px;
  }
  .selected-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .sel-chip {
    padding: 2px 8px;
    border-radius: 5px;
    background: var(--panel);
    border: 1px solid var(--accent);
    color: var(--accent);
    font-size: 11px;
  }
  .feedback-controls {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .feedback-controls input {
    flex: 1;
    min-width: 220px;
    padding: 6px 10px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
  }
  .engine-pill {
    font-size: 11px;
    padding: 3px 8px;
    border-radius: 5px;
    background: var(--panel-subtle);
    border: 1px solid var(--line);
    color: var(--muted);
  }
  .diff-cat-group {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg);
    overflow: hidden;
  }
  .diff-cat-hdr {
    padding: 8px 12px;
    background: var(--panel-subtle);
  }
  .cat-toggle-btn {
    width: 100%;
    border: none;
    background: transparent;
    color: var(--text);
    display: flex;
    align-items: center;
    gap: 8px;
    text-align: left;
    font-size: 12.5px;
    padding: 0;
    flex-wrap: wrap;
  }
  .cat-count,
  .auto-collapse-pill {
    margin-left: auto;
    font-size: 11px;
    color: var(--muted);
  }
  .sdiff-list {
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .sdiff-card {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--panel);
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .sdiff-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
  }
  .sdiff-title {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    font-size: 12px;
  }
  .sym-name {
    font-weight: 700;
  }
  .file-path {
    color: var(--muted);
    font-size: 11px;
  }
  .diff-stat .add {
    color: var(--ok);
    font-weight: 600;
  }
  .diff-stat .del {
    color: var(--err);
    font-weight: 600;
    margin-left: 4px;
  }
  .sdiff-actions {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .mini-btn {
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text);
    font-size: 11px;
  }
  .pseudocode-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
    gap: 8px;
  }
  .pseudo-step {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--bg);
    border: 1px solid var(--line);
  }
  .pseudo-label {
    display: block;
    font-size: 10px;
    font-weight: 700;
    color: var(--accent);
    margin-bottom: 4px;
  }
  .pseudo-step p {
    margin: 0;
    font-size: 12px;
    line-height: 1.5;
  }
  .snippet-box {
    margin: 0;
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--panel-subtle);
    border: 1px solid var(--line);
    font-size: 11.5px;
    overflow-x: auto;
  }
  .decision-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .decision-card {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .decision-card.red-pin {
    border-color: var(--err);
    background: var(--panel);
  }
  .dec-hdr {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
  }
  .dec-badge-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .red-pin-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 2px 8px;
    border-radius: 5px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 11px;
    font-weight: 700;
  }
  .req-pin-badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: 5px;
    background: var(--ok-soft);
    color: var(--ok);
    font-size: 11px;
    font-weight: 700;
  }
  .dec-title {
    font-size: 13px;
  }
  .dec-compare-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: 8px;
  }
  .dec-col {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--panel-subtle);
    border: 1px solid var(--line);
  }
  .dec-label {
    display: block;
    font-size: 10.5px;
    font-weight: 700;
    color: var(--muted);
    margin-bottom: 3px;
  }
  .dec-col p {
    margin: 0;
    font-size: 12px;
    line-height: 1.5;
  }
  .workflow-banner {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: 10px;
  }
  .wf-item {
    padding: 10px 12px;
    border-radius: 8px;
    background: var(--bg);
    border: 1px solid var(--line);
    font-size: 12px;
  }
  .wf-item p {
    margin: 4px 0 0;
    color: var(--muted);
  }
  .tpl-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: 12px;
  }
  .tpl-card {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .tpl-card h4 {
    margin: 3px 0;
    font-size: 13.5px;
  }
  .tpl-steps {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .tpl-step {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--panel);
    border: 1px solid var(--line);
    font-size: 12px;
  }
  .tpl-step-title {
    font-weight: 700;
    color: var(--accent);
  }
  .tpl-step p {
    margin: 3px 0 5px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .tpl-step code {
    display: block;
    padding: 3px 6px;
    border-radius: 4px;
    background: var(--panel-subtle);
    font-size: 11px;
  }
  .tpl-footer {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: auto;
  }
  .arch-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: 10px;
  }
  .arch-card {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg);
    padding: 12px;
  }
  .arch-card-hdr {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    margin-bottom: 8px;
  }
  .arch-list,
  .constraint-list {
    margin: 0;
    padding-left: 18px;
    display: flex;
    flex-direction: column;
    gap: 5px;
    font-size: 12px;
    line-height: 1.5;
  }
  .privacy-box {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .privacy-hdr {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
  }
  .privacy-title {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    font-size: 13px;
  }
  .telemetry-switch {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 5px 10px;
    border-radius: 6px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    font-size: 11.5px;
    cursor: pointer;
  }
  .privacy-cols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: 10px;
  }
  .priv-col {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--panel);
    border: 1px solid var(--line);
  }
  .priv-sub {
    display: block;
    font-size: 11px;
    font-weight: 700;
    margin-bottom: 6px;
  }
  .redacted-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .redacted-chip {
    font-size: 11px;
    padding: 2px 7px;
    border-radius: 4px;
    background: var(--err-soft);
    color: var(--err);
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 11px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
    font-weight: 500;
  }
  .btn:hover {
    background: var(--hover);
  }
  .btn.primary {
    background: var(--accent);
    color: var(--accent-fg);
    border-color: var(--accent);
  }
  .muted {
    color: var(--muted);
    font-size: 11.5px;
    margin: 0;
  }
</style>
