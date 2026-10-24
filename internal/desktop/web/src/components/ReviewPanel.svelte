<script lang="ts">
  import { X, ArrowLeft } from "lucide-svelte";
  import ConflictCard from "./cards/ConflictCard.svelte";
  import StatusCard from "./cards/StatusCard.svelte";
  import SubagentsCard from "./cards/SubagentsCard.svelte";
  import ChangesCard from "./cards/ChangesCard.svelte";
  import VerificationCard from "./cards/VerificationCard.svelte";
  import WorkspaceCard from "./cards/WorkspaceCard.svelte";
  import DiffView from "./DiffView.svelte";
  import FileTreeView from "./FileTreeView.svelte";
  import MarkdownView from "./MarkdownView.svelte";
  import { parseUnifiedDiff } from "../lib/diff";
  import {
    selectedTask,
    selectedTaskId,
    taskEvents,
    applyTaskChanges,
    cancelTaskById,
    ACTIVE_TASK_STATUSES
  } from "../lib/stores/tasks";
  import {
    rightCollapsed,
    rightPanelView,
    selectedDiffFile,
    highlightedFileLine,
    toggleRightPanel
  } from "../lib/stores/settings";
  import {
    activeFileView,
    workspaceFiles,
    workspaceSymbols,
    workspaceGraph,
    fetchWorkspaceFiles,
    fetchWorkspaceSymbols,
    fetchWorkspaceGraph
  } from "../lib/stores/workspace";

  let busy = $state(false);
  let fullFileContainer = $state<HTMLDivElement | null>(null);
  let changeCursor = $state(0);
  let mdPreview = $state(false);
  const isMarkdownFile = $derived(Boolean($activeFileView?.path && /\.mdx?$/i.test($activeFileView.path)));

  interface FullFileAnnotatedLine {
    kind: "ctx" | "add" | "del";
    lineNo: number | string;
    text: string;
  }

  const activeFileDiff = $derived.by(() => {
    const fv = $activeFileView;
    const job = $selectedTask;
    if (!fv || !job?.report?.diff) return null;
    const diffs = parseUnifiedDiff(job.report.diff, job.report.modified_files || []);
    return diffs.find((d) => d.file === fv.path) || null;
  });

  const annotatedFileLines = $derived.by((): FullFileAnnotatedLine[] => {
    const fv = $activeFileView;
    if (!fv || fv.binary) return [];
    const rawLines = (fv.content || "").replace(/\r\n/g, "\n").split("\n");
    const fd = activeFileDiff;
    if (!fd || fd.lines.length === 0) {
      return rawLines.map((text, i) => ({ kind: "ctx", lineNo: i + 1, text }));
    }

    const addedSet = new Set<number>();
    const delsBeforeNew = new Map<number, Array<{ oldLine?: number; text: string }>>();
    const trailingDels: Array<{ oldLine?: number; text: string }> = [];
    let pendingDels: Array<{ oldLine?: number; text: string }> = [];
    let lastSeenNewLine = 1;

    for (const ln of fd.lines) {
      if (ln.kind === "hunk") continue;
      if (ln.kind === "del") {
        pendingDels.push({ oldLine: ln.oldLine, text: ln.text });
      } else {
        const nLine = ln.newLine ?? lastSeenNewLine;
        lastSeenNewLine = nLine;
        if (pendingDels.length > 0) {
          const prev = delsBeforeNew.get(nLine) || [];
          delsBeforeNew.set(nLine, [...prev, ...pendingDels]);
          pendingDels = [];
        }
        if (ln.kind === "add" && ln.newLine !== undefined) {
          addedSet.add(ln.newLine);
        }
      }
    }
    if (pendingDels.length > 0) {
      trailingDels.push(...pendingDels);
    }

    const out: FullFileAnnotatedLine[] = [];
    for (let i = 0; i < rawLines.length; i++) {
      const nLine = i + 1;
      const dels = delsBeforeNew.get(nLine);
      if (dels) {
        for (const d of dels) {
          out.push({ kind: "del", lineNo: d.oldLine ?? "", text: d.text });
        }
      }
      out.push({
        kind: addedSet.has(nLine) ? "add" : "ctx",
        lineNo: nLine,
        text: rawLines[i]
      });
    }
    for (const d of trailingDels) {
      out.push({ kind: "del", lineNo: d.oldLine ?? "", text: d.text });
    }
    return out;
  });

  function scrollToNextChange() {
    if (!fullFileContainer) return;
    const changedEls = fullFileContainer.querySelectorAll<HTMLElement>(".ln.add, .ln.del");
    if (changedEls.length === 0) return;
    const idx = changeCursor % changedEls.length;
    changeCursor = idx + 1;
    changedEls[idx]?.scrollIntoView({ block: "center", behavior: "smooth" });
  }

  $effect(() => {
    const targetLn = $highlightedFileLine;
    if ($rightPanelView === "file" && $activeFileView && fullFileContainer && targetLn) {
      const lineEl = fullFileContainer.querySelector<HTMLElement>(`[data-line-no="${targetLn}"]`);
      if (lineEl) {
        lineEl.scrollIntoView({ block: "center", behavior: "smooth" });
        return;
      }
    }
    if ($rightPanelView === "file" && $activeFileView && fullFileContainer && activeFileDiff) {
      changeCursor = 0;
      const first = fullFileContainer.querySelector<HTMLElement>(".ln.add, .ln.del");
      first?.scrollIntoView({ block: "center" });
    }
  });

  async function onApply() {
    if (!$selectedTask || busy) return;
    busy = true;
    await applyTaskChanges($selectedTask.id, "ask");
    busy = false;
  }

  async function onDiscardOrCancel() {
    if (!$selectedTask || busy) return;
    if (ACTIVE_TASK_STATUSES.has($selectedTask.status)) {
      busy = true;
      await cancelTaskById($selectedTask.id);
      busy = false;
    } else {
      selectedTaskId.set("");
    }
  }
</script>

{#if !$rightCollapsed}
  <aside class="review-panel" aria-label="พาเนลตรวจสอบและรีวิว" data-testid="review-panel">
    <div class="panel-hdr">
      <span class="title" id="artifact-title">
        {#if $rightPanelView === "files"}
          Workspace Files ({$workspaceFiles.length})
        {:else if $rightPanelView === "symbols"}
          AST Symbols ({$workspaceSymbols.length} files)
        {:else if $rightPanelView === "graph"}
          Call Impact Graph ({$workspaceGraph?.calls?.length ?? 0} edges)
        {:else if $rightPanelView === "file" && $activeFileView}
          {$activeFileView.path} ({$activeFileView.source})
        {:else if $selectedTask}
          Task · {$selectedTask.instruction || $selectedTask.id}
        {:else}
          Workspace Overview
        {/if}
      </span>
      <button
        type="button"
        class="close-btn"
        onclick={toggleRightPanel}
        aria-label="ปิดพาเนลขวา"
        title="ปิดพาเนล (Ctrl+J)"
      >
        <X size={14} />
      </button>
    </div>

    <div class="nav-tabs" role="tablist" aria-label="มุมมองพาเนลขวา">
      <button
        type="button"
        class="nav-tab"
        class:active={$rightPanelView === "cards"}
        onclick={() => rightPanelView.set("cards")}
        data-testid="tab-overview"
      >
        Overview
      </button>
      <button
        type="button"
        class="nav-tab"
        class:active={$rightPanelView === "diff"}
        disabled={!$selectedTask}
        onclick={() => rightPanelView.set("diff")}
        data-testid="tab-review"
      >
        Review
      </button>
      <button
        type="button"
        class="nav-tab"
        class:active={$rightPanelView === "files" || $rightPanelView === "file"}
        onclick={async () => {
          await fetchWorkspaceFiles($selectedTask?.id);
          rightPanelView.set("files");
        }}
        data-testid="tab-files"
      >
        Files
      </button>
      <button
        type="button"
        class="nav-tab"
        class:active={$rightPanelView === "symbols"}
        onclick={async () => {
          await fetchWorkspaceSymbols();
          rightPanelView.set("symbols");
        }}
        data-testid="tab-symbols"
      >
        Symbols
      </button>
      <button
        type="button"
        class="nav-tab"
        class:active={$rightPanelView === "graph"}
        onclick={async () => {
          await fetchWorkspaceGraph();
          rightPanelView.set("graph");
        }}
        data-testid="tab-graph"
      >
        Graph
      </button>
    </div>

    {#if $rightPanelView === "diff" && $selectedTask}
      <DiffView job={$selectedTask} />
    {:else if $rightPanelView === "files"}
      <FileTreeView />
    {:else if $rightPanelView === "file" && $activeFileView}
      <div class="full-view">
        <div class="sub-bar">
          <button
            type="button"
            class="back-btn"
            onclick={() => {
              if (activeFileDiff) {
                selectedDiffFile.set($activeFileView.path);
                rightPanelView.set("diff");
              } else {
                rightPanelView.set("files");
              }
            }}
          >
            <ArrowLeft size={13} />
            <span>{activeFileDiff ? "กลับหน้า Diff" : "Files"}</span>
          </button>
          <span class="mono truncate file-title">
            {$activeFileView.path}{#if $highlightedFileLine}:L{$highlightedFileLine}{/if}
          </span>
          <span class="ro-pill mono" title="โหมดอ่านและตรวจโค้ดอย่างเดียว (Read-Only)">READ-ONLY</span>
          {#if isMarkdownFile}
            <button type="button" class="jump-btn" onclick={() => (mdPreview = !mdPreview)}>
              {mdPreview ? "ดูโค้ด & Diff" : "Preview Markdown"}
            </button>
          {/if}
          {#if activeFileDiff && !mdPreview}
            <button type="button" class="jump-btn mono" onclick={scrollToNextChange} title="เลื่อนไปยังบรรทัดที่แก้ไข">
              <span class="add">+{activeFileDiff.added}</span>
              <span class="del">−{activeFileDiff.deleted}</span>
              <span>จุดที่แก้ ↓</span>
            </button>
          {/if}
        </div>
        {#if $activeFileView.binary}
          <div class="empty">ไฟล์ไบนารีไม่สามารถแสดงเป็นข้อความได้</div>
        {:else if isMarkdownFile && mdPreview}
          <div class="md-file-preview selectable">
            <MarkdownView text={$activeFileView.content} taskId={$selectedTask?.id} />
          </div>
        {:else}
          <div class="full-file-scroll mono selectable" bind:this={fullFileContainer}>
            {#each annotatedFileLines as ln, idx (idx)}
              <div
                class="ln"
                class:add={ln.kind === "add"}
                class:del={ln.kind === "del"}
                class:target-line={$highlightedFileLine !== null && Number(ln.lineNo) === $highlightedFileLine}
                data-line-no={ln.lineNo}
              >
                <span class="g">{ln.lineNo}</span>
                <span class="m">{ln.kind === "add" ? "+" : ln.kind === "del" ? "−" : " "}</span>
                <span class="code">{ln.text}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {:else if $rightPanelView === "symbols"}
      <div class="full-view" data-testid="symbols-view">
        <div class="sub-bar">
          <button type="button" class="back-btn" onclick={() => rightPanelView.set("cards")}>
            <ArrowLeft size={13} />
            <span>กลับ</span>
          </button>
          <span>AST Symbols</span>
        </div>
        <div class="code-box mono selectable">
          {#if $workspaceSymbols.length === 0}
            <div>ไม่พบ AST Symbols</div>
          {:else}
            {#each $workspaceSymbols as f (f.file)}
              <div class="sym-file">=== {f.file} ===</div>
              {#each f.symbols || [] as s (s.Name + s.StartLine)}
                <div>  [{s.Kind}] {s.Name} (L{s.StartLine}-L{s.EndLine})</div>
              {/each}
            {/each}
          {/if}
        </div>
      </div>
    {:else if $rightPanelView === "graph"}
      <div class="full-view" data-testid="graph-view">
        <div class="sub-bar">
          <button type="button" class="back-btn" onclick={() => rightPanelView.set("cards")}>
            <ArrowLeft size={13} />
            <span>กลับ</span>
          </button>
          <span>Cross-File Call Impact Graph</span>
        </div>
        <div class="code-box mono selectable">
          <div>Symbols Indexed : {$workspaceGraph?.definitions?.length ?? 0}</div>
          <div>Call Edges      : {$workspaceGraph?.calls?.length ?? 0}</div>
          <hr />
          {#each $workspaceGraph?.calls || [] as edge, idx (idx)}
            <div>{edge.from_file}:L{edge.line} {edge.from_symbol} ──► {edge.to_symbol}</div>
          {/each}
        </div>
      </div>
    {:else}
      <div class="cards-scroll">
        <ConflictCard />
        {#if $selectedTask}
          <StatusCard job={$selectedTask} events={$taskEvents[$selectedTask.id] || []} />
          <SubagentsCard job={$selectedTask} events={$taskEvents[$selectedTask.id] || []} />
          <ChangesCard job={$selectedTask} />
          <VerificationCard job={$selectedTask} />
        {/if}
        <WorkspaceCard />
      </div>
    {/if}

    {#if $selectedTask}
      <div class="panel-footer">
        <span class="ver-state" class:ok={$selectedTask.status === "ready_to_apply" || $selectedTask.status === "succeeded"}>
          {#if $selectedTask.status === "ready_to_apply"}
            ✓ พร้อม apply
          {:else if $selectedTask.status === "succeeded"}
            ✓ Applied
          {:else if $selectedTask.status === "preview_ready"}
            ✓ Preview พร้อมตรวจทาน
          {:else}
            {$selectedTask.status}
          {/if}
        </span>

        <button
          type="button"
          class="btn"
          onclick={onDiscardOrCancel}
          disabled={busy}
          data-testid="panel-discard-btn"
        >
          {ACTIVE_TASK_STATUSES.has($selectedTask.status) ? "Cancel" : "Discard"}
        </button>

        <button
          type="button"
          class="btn primary"
          onclick={onApply}
          disabled={busy || $selectedTask.status !== "ready_to_apply"}
          data-testid="panel-apply-btn"
        >
          Apply
        </button>
      </div>
    {/if}
  </aside>
{/if}

<style>
  .review-panel {
    width: 100%;
    height: 100%;
    background: var(--panel);
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .panel-hdr {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 10px;
    border-bottom: 1px solid var(--line);
  }
  .panel-hdr .title {
    font-weight: 600;
    font-size: 12.5px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .close-btn {
    border: none;
    background: transparent;
    color: var(--faint);
    padding: 6px;
    min-width: 32px;
    min-height: 32px;
    border-radius: 5px;
    display: grid;
    place-items: center;
  }
  .close-btn:hover {
    color: var(--text);
    background: var(--hover);
  }
  .nav-tabs {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 4px 8px;
    border-bottom: 1px solid var(--line);
    background: var(--panel-subtle);
    overflow-x: auto;
  }
  .nav-tab {
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 11.5px;
    font-weight: 500;
    padding: 4px 8px;
    border-radius: 5px;
    white-space: nowrap;
  }
  .nav-tab:hover:not(:disabled) {
    color: var(--text);
    background: var(--hover);
  }
  .nav-tab.active {
    color: var(--accent);
    background: var(--accent-soft);
    font-weight: 600;
  }
  .nav-tab:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .cards-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    background: var(--bg);
  }
  .full-view {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .sub-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 8px;
    border-bottom: 1px solid var(--line);
    background: var(--bg);
    font-size: 11.5px;
  }
  .file-title {
    flex: 1;
    min-width: 0;
  }
  .ro-pill {
    font-size: 9.5px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 4px;
    background: var(--accent-soft);
    color: var(--accent);
    border: 1px solid var(--line);
    flex: none;
  }
  .ln.target-line {
    background: var(--accent-soft);
    outline: 1px solid var(--accent);
  }
  .ln.target-line .g {
    color: var(--accent);
    font-weight: 700;
  }
  .back-btn,
  .jump-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 11px;
    flex: none;
  }
  .back-btn:hover,
  .jump-btn:hover {
    background: var(--hover);
  }
  .jump-btn .add {
    color: var(--addt);
    font-weight: 600;
  }
  .jump-btn .del {
    color: var(--delt);
    font-weight: 600;
  }
  .full-file-scroll {
    flex: 1;
    overflow: auto;
    font-size: 11px;
    line-height: 1.65;
    background: var(--panel);
  }
  .md-file-preview {
    flex: 1;
    overflow: auto;
    padding: 14px 16px;
    background: var(--panel);
  }
  .ln {
    display: flex;
    white-space: pre;
  }
  .ln .g {
    width: 38px;
    text-align: right;
    padding-right: 6px;
    color: var(--faint);
    flex: none;
    user-select: none;
  }
  .ln .m {
    width: 14px;
    flex: none;
    color: var(--faint);
    user-select: none;
  }
  .ln .code {
    padding-right: 12px;
  }
  .ln.add {
    background: var(--add);
  }
  .ln.add .g {
    background: var(--addg);
    color: var(--addt);
    font-weight: 600;
  }
  .ln.add .m {
    color: var(--addt);
    font-weight: 600;
  }
  .ln.del {
    background: var(--del);
  }
  .ln.del .g {
    background: var(--delg);
    color: var(--delt);
    font-weight: 600;
  }
  .ln.del .m {
    color: var(--delt);
    font-weight: 600;
  }
  .code-box {
    flex: 1;
    margin: 0;
    padding: 10px 12px;
    overflow: auto;
    font-size: 11.5px;
    line-height: 1.6;
    background: var(--panel);
    white-space: pre;
  }
  .sym-file {
    margin-top: 8px;
    font-weight: 600;
    color: var(--accent);
  }
  .empty {
    padding: 16px;
    color: var(--faint);
  }
  .panel-footer {
    border-top: 1px solid var(--line);
    padding: 8px 10px;
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--panel);
  }
  .ver-state {
    flex: 1;
    font-size: 11.5px;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ver-state.ok {
    color: var(--ok);
    font-weight: 500;
  }
  .btn {
    padding: 5px 11px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
    font-weight: 500;
  }
  .btn:hover:not(:disabled) {
    background: var(--hover);
  }
  .btn.primary {
    background: var(--accent);
    color: var(--accent-fg);
    border-color: var(--accent);
  }
  .btn:disabled {
    opacity: 0.45;
    cursor: default;
  }
</style>
