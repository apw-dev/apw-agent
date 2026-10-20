<script lang="ts">
  import { FolderOpen, ShieldCheck, ShieldAlert, FileText, Check } from "lucide-svelte";
  import {
    workspaceStatus,
    toggleWorkspaceTrust,
    initInstructionsFile,
    loadInstructions,
    instructionsContent,
    inlineWorkspaceError
  } from "../lib/stores/workspace";
  import { recentWorkspaces, workspacePopoverOpen, rightCollapsed, rightPanelView } from "../lib/stores/settings";
  import { chooseWorkspace, openWorkspaceDir, inDesktopShell, revealInExplorer } from "../lib/shell";
  import { fetchWorkspaceFile } from "../lib/stores/workspace";

  let showInstructionsModal = $state(false);
  let busy = $state(false);

  function basename(p: string): string {
    if (!p) return "workspace";
    const norm = p.replace(/\\/g, "/").replace(/\/+$/, "");
    return norm.split("/").pop() || norm;
  }

  async function onToggleTrust() {
    if (!$workspaceStatus || busy) return;
    busy = true;
    await toggleWorkspaceTrust(!$workspaceStatus.trusted);
    busy = false;
  }

  async function onInitOrViewInstructions() {
    if (!$workspaceStatus) return;
    busy = true;
    if (!$workspaceStatus.instructions_loaded) {
      await initInstructionsFile();
    } else {
      await loadInstructions();
    }
    busy = false;
    showInstructionsModal = true;
  }

  async function onOpenInstructionsInRightPane() {
    const path = $workspaceStatus?.instructions_path || "APW.md";
    const res = await fetchWorkspaceFile(path);
    if (res.ok) {
      rightCollapsed.set(false);
      rightPanelView.set("file");
      workspacePopoverOpen.set(false);
    }
  }
</script>

{#if $workspacePopoverOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="popover-backdrop" onclick={() => workspacePopoverOpen.set(false)}></div>
  <div class="popover" role="dialog" aria-label="ตั้งค่า Workspace">
    <div class="sec">
      <div class="lbl">Workspace ปัจจุบัน</div>
      <div class="ws-path mono selectable" title={$workspaceStatus?.workspace || ""}>
        {$workspaceStatus?.workspace || "."}
      </div>
      <div class="meta">
        <span>Stack: <b>{$workspaceStatus?.stack || "Unknown"}</b></span>
        <span>Skills: <b>{$workspaceStatus?.skills_loaded ?? 0}</b></span>
      </div>
    </div>

    <div class="sec">
      <button type="button" class="row-btn" onclick={onToggleTrust} disabled={busy} data-testid="trust-toggle-btn">
        {#if $workspaceStatus?.trusted}
          <ShieldCheck size={15} />
          <span>Trusted (อนุญาตรัน build/test)</span>
          <span class="badge ok">ON</span>
        {:else}
          <ShieldAlert size={15} />
          <span>Untrusted (จำกัดการรันคำสั่ง)</span>
          <span class="badge warn">OFF</span>
        {/if}
      </button>

      <button type="button" class="row-btn" onclick={onInitOrViewInstructions} disabled={busy} data-testid="apw-md-btn">
        <FileText size={15} />
        {#if $workspaceStatus?.instructions_loaded}
          <span>เปิดอ่าน {$workspaceStatus.instructions_path || "APW.md"} ({$workspaceStatus.instructions_lines} บรรทัด)</span>
        {:else}
          <span>สร้างไฟล์คำแนะนำโปรเจกต์ APW.md</span>
        {/if}
      </button>

      {#if $workspaceStatus?.instructions_loaded}
        <button type="button" class="row-btn sub" onclick={onOpenInstructionsInRightPane}>
          <FileText size={14} />
          <span>เปิด {$workspaceStatus.instructions_path} ในพาเนลขวา</span>
        </button>
      {/if}

      {#if inDesktopShell()}
        <button
          type="button"
          class="row-btn"
          onclick={async () => {
            workspacePopoverOpen.set(false);
            await chooseWorkspace();
          }}
        >
          <FolderOpen size={15} />
          <span>เปิดโฟลเดอร์อื่น… (Ctrl+O)</span>
        </button>
        <button
          type="button"
          class="row-btn"
          onclick={() => {
            workspacePopoverOpen.set(false);
            void revealInExplorer(".");
          }}
        >
          <FolderOpen size={15} />
          <span>เปิดใน Windows Explorer</span>
        </button>
      {/if}
    </div>

    {#if $recentWorkspaces.length > 0}
      <div class="sec">
        <div class="lbl">โฟลเดอร์ล่าสุด</div>
        {#each $recentWorkspaces as dir}
          <button
            type="button"
            class="row-btn mono"
            onclick={() => {
              workspacePopoverOpen.set(false);
              void openWorkspaceDir(dir);
            }}
            title={dir}
          >
            <span class="truncate">{basename(dir)} ({dir})</span>
            {#if dir === $workspaceStatus?.workspace}
              <Check size={14} />
            {/if}
          </button>
        {/each}
      </div>
    {/if}

    {#if $inlineWorkspaceError}
      <div class="err-box">{$inlineWorkspaceError}</div>
    {/if}

    {#if showInstructionsModal && $instructionsContent}
      <div class="sec">
        <div class="lbl">{$instructionsContent.path}</div>
        <pre class="instr-pre selectable">{$instructionsContent.content}</pre>
      </div>
    {/if}
  </div>
{/if}

<style>
  .popover-backdrop {
    position: fixed;
    inset: 0;
    z-index: 40;
  }
  .popover {
    position: absolute;
    top: 44px;
    left: 8px;
    width: 310px;
    max-height: calc(100vh - 64px);
    overflow-y: auto;
    background: var(--panel);
    border: 1px solid var(--line-strong);
    border-radius: 10px;
    box-shadow: var(--shadow-md);
    padding: 8px;
    z-index: 50;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .sec {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding-bottom: 6px;
    border-bottom: 1px solid var(--line);
  }
  .sec:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }
  .lbl {
    font-size: 11px;
    color: var(--faint);
    font-weight: 600;
    padding: 2px 6px;
  }
  .ws-path {
    font-size: 11.5px;
    padding: 4px 6px;
    background: var(--panel-subtle);
    border-radius: 6px;
    word-break: break-all;
  }
  .meta {
    display: flex;
    gap: 12px;
    padding: 2px 6px;
    font-size: 11.5px;
    color: var(--muted);
  }
  .row-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    text-align: left;
    padding: 6px 8px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 12px;
  }
  .row-btn:hover {
    background: var(--hover);
  }
  .row-btn.sub {
    padding-left: 24px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .badge {
    margin-left: auto;
    font-size: 10px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 4px;
  }
  .badge.ok {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .badge.warn {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .truncate {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .err-box {
    padding: 6px 8px;
    border-radius: 6px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 11.5px;
  }
  .instr-pre {
    margin: 4px 0 0;
    padding: 8px;
    max-height: 180px;
    overflow: auto;
    font-size: 11px;
    background: var(--panel-subtle);
    border-radius: 6px;
    white-space: pre-wrap;
  }
</style>
