<script lang="ts">
  import { X, ArrowLeft } from "lucide-svelte";
  import ConflictCard from "./cards/ConflictCard.svelte";
  import StatusCard from "./cards/StatusCard.svelte";
  import ChangesCard from "./cards/ChangesCard.svelte";
  import VerificationCard from "./cards/VerificationCard.svelte";
  import WorkspaceCard from "./cards/WorkspaceCard.svelte";
  import DiffView from "./DiffView.svelte";
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
    toggleRightPanel
  } from "../lib/stores/settings";
  import {
    activeFileView,
    workspaceSymbols,
    workspaceGraph
  } from "../lib/stores/workspace";

  let busy = $state(false);

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
        {#if $rightPanelView === "symbols"}
          AST Symbols ({$workspaceSymbols.length} files)
        {:else if $rightPanelView === "graph"}
          Call Impact Graph ({$workspaceGraph?.calls?.length ?? 0} edges)
        {:else if $rightPanelView === "file" && $activeFileView}
          {$activeFileView.path} ({$activeFileView.source})
        {:else if $selectedTask}
          Task · {$selectedTask.instruction || $selectedTask.id}
        {:else}
          Workspace Review
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

    {#if $rightPanelView === "diff" && $selectedTask}
      <DiffView job={$selectedTask} />
    {:else if $rightPanelView === "file" && $activeFileView}
      <div class="full-view">
        <div class="sub-bar">
          <button type="button" class="back-btn" onclick={() => rightPanelView.set("cards")}>
            <ArrowLeft size={13} />
            <span>กลับ</span>
          </button>
          <span class="mono truncate">{$activeFileView.path}</span>
        </div>
        {#if $activeFileView.binary}
          <div class="empty">ไฟล์ไบนารีไม่สามารถแสดงเป็นข้อความได้</div>
        {:else}
          <pre class="code-box mono selectable">{$activeFileView.content}</pre>
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
    padding: 3px;
    border-radius: 5px;
    display: grid;
    place-items: center;
  }
  .close-btn:hover {
    color: var(--text);
    background: var(--hover);
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
  .back-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 11.5px;
  }
  .code-box {
    flex: 1;
    margin: 0;
    padding: 10px;
    overflow: auto;
    font-size: 11px;
    line-height: 1.6;
    white-space: pre;
  }
  .sym-file {
    font-weight: 600;
    color: var(--accent);
    margin-top: 6px;
  }
  .truncate {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    gap: 6px;
    background: var(--panel);
  }
  .ver-state {
    margin-right: auto;
    font-size: 11.5px;
    color: var(--muted);
  }
  .ver-state.ok {
    color: var(--ok);
    font-weight: 500;
  }
  .btn {
    padding: 4px 11px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 11.5px;
    font-weight: 500;
  }
  .btn:hover:not(:disabled) {
    background: var(--hover);
  }
  .btn.primary {
    background: var(--text);
    color: var(--panel);
    border-color: var(--text);
  }
  .btn:disabled {
    opacity: 0.45;
    cursor: default;
  }
</style>
