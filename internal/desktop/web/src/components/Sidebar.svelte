<script lang="ts">
  import { Plus, Settings, Clock, ChevronDown, FolderGit2 } from "lucide-svelte";
  import WorkspacePopover from "./WorkspacePopover.svelte";
  import { workspaceStatus, autonomyState } from "../lib/stores/workspace";
  import {
    groupedSessions,
    currentSessionId,
    selectSession,
    createNewConversation
  } from "../lib/stores/sessions";
  import { runningTasks, tasks, selectedTaskId, selectTask } from "../lib/stores/tasks";
  import {
    workspacePopoverOpen,
    settingsModalOpen,
    centerView
  } from "../lib/stores/settings";

  function workspaceName(p?: string): string {
    if (!p) return "apw-agent";
    const clean = p.replace(/\\/g, "/").replace(/\/+$/, "");
    return clean.split("/").pop() || clean;
  }
</script>

<aside class="sidebar" aria-label="แถบนำทางด้านซ้าย">
  <div class="header-wrap">
    <button
      type="button"
      class="ws-btn"
      onclick={() => workspacePopoverOpen.update((v) => !v)}
      data-testid="workspace-header-btn"
    >
      <FolderGit2 size={15} />
      <span class="ws-name">{workspaceName($workspaceStatus?.workspace)}</span>
      <ChevronDown size={13} />
      {#if $workspaceStatus?.trusted}
        <span class="trust-pill ok">trusted</span>
      {:else}
        <span class="trust-pill warn">untrusted</span>
      {/if}
    </button>
    <WorkspacePopover />
  </div>

  <button
    type="button"
    class="new-conv-btn"
    onclick={() => createNewConversation()}
    data-testid="new-conversation-btn"
  >
    <span class="new-left">
      <Plus size={14} />
      <span>Conversation ใหม่</span>
    </span>
    <kbd>Ctrl+N</kbd>
  </button>

  <div class="nav-scroll" id="session-list">
    {#if $runningTasks.length > 0}
      <div class="grp-title">กำลังทำงาน ({$runningTasks.length})</div>
      {#each $runningTasks as job (job.id)}
        <button
          type="button"
          class="nav-item"
          class:active={$selectedTaskId === job.id && $centerView === "thread"}
          onclick={() => {
            centerView.set("thread");
            void selectTask(job.id, true);
          }}
        >
          <span class="dot running"></span>
          <span class="label">{job.instruction || job.id}</span>
        </button>
      {/each}
    {/if}

    {#each $groupedSessions as group (group.label)}
      <div class="grp-title">{group.label}</div>
      {#each group.items as sess (sess.id)}
        <button
          type="button"
          class="nav-item"
          class:active={$currentSessionId === sess.id && $centerView === "thread"}
          onclick={() => selectSession(sess.id)}
          data-testid="session-item"
        >
          <span class="dot ok"></span>
          <span class="label">{sess.last_instruction || sess.id}</span>
        </button>
      {/each}
    {/each}

    {#if $tasks.length > 0}
      <div class="grp-title" id="task-list">งานล่าสุด</div>
      {#each $tasks.slice(0, 12) as job (job.id)}
        <button
          type="button"
          class="nav-item"
          class:active={$selectedTaskId === job.id}
          onclick={() => {
            centerView.set("thread");
            void selectTask(job.id, true);
          }}
          data-testid="task-list-item"
          title={`${job.id} [${job.status}]`}
        >
          <span
            class="dot"
            class:ok={job.status === "succeeded" || job.status === "ready_to_apply" || job.status === "preview_ready"}
            class:err={job.status === "failed" || job.status === "cancelled"}
          ></span>
          <span class="label">{job.instruction || job.id}</span>
        </button>
      {/each}
    {/if}
  </div>

  <div class="footer">
    <button
      type="button"
      class="foot-item"
      onclick={() => settingsModalOpen.set(true)}
      data-testid="open-settings-btn"
    >
      <Settings size={14} />
      <span>Settings</span>
      <kbd>Ctrl+,</kbd>
    </button>
    <button
      type="button"
      class="foot-item"
      class:active={$centerView === "autonomy"}
      onclick={() => centerView.set($centerView === "autonomy" ? "thread" : "autonomy")}
      data-testid="open-autonomy-btn"
    >
      <Clock size={14} />
      <span>Autonomy</span>
      {#if $autonomyState?.governor?.paused}
        <span class="auto-badge paused">paused</span>
      {:else}
        <span class="auto-badge on">24/7</span>
      {/if}
    </button>
  </div>
</aside>

<style>
  .sidebar {
    width: 100%;
    height: 100%;
    background: var(--side);
    backdrop-filter: blur(18px);
    display: flex;
    flex-direction: column;
    padding: 8px;
    gap: 2px;
    position: relative;
  }
  .header-wrap {
    position: relative;
  }
  .ws-btn {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--text);
    font-weight: 600;
    font-size: 13px;
  }
  .ws-btn:hover {
    background: var(--hover);
  }
  .ws-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .trust-pill {
    margin-left: auto;
    font-weight: 500;
    font-size: 10px;
    border-radius: 4px;
    padding: 0 5px;
    border: 1px solid currentColor;
  }
  .trust-pill.ok {
    color: var(--ok);
    background: var(--ok-soft);
  }
  .trust-pill.warn {
    color: var(--warn);
    background: var(--warn-soft);
  }
  .new-conv-btn {
    margin: 6px 0 8px;
    padding: 6px 8px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--panel);
    color: var(--text);
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 12.5px;
    font-weight: 500;
  }
  .new-conv-btn:hover {
    border-color: var(--line-strong);
    background: var(--hover);
  }
  .new-left {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  kbd {
    color: var(--faint);
    font-family: inherit;
    font-size: 10.5px;
  }
  .nav-scroll {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .grp-title {
    font-size: 10.5px;
    color: var(--faint);
    padding: 8px 8px 3px;
    font-weight: 600;
    text-transform: uppercase;
  }
  .nav-item {
    width: 100%;
    padding: 5px 8px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--text);
    display: flex;
    gap: 7px;
    align-items: center;
    text-align: left;
    font-size: 12.5px;
  }
  .nav-item:hover {
    background: var(--hover);
  }
  .nav-item.active {
    background: var(--active);
    font-weight: 500;
  }
  .nav-item .label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    flex: none;
  }
  .dot.ok {
    background: var(--ok);
  }
  .dot.err {
    background: var(--err);
  }
  .dot.running {
    background: var(--accent);
  }
  .footer {
    margin-top: auto;
    border-top: 1px solid var(--line);
    padding-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .foot-item {
    width: 100%;
    padding: 6px 8px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 12.5px;
  }
  .foot-item:hover,
  .foot-item.active {
    background: var(--hover);
    color: var(--text);
  }
  .foot-item kbd {
    margin-left: auto;
  }
  .auto-badge {
    margin-left: auto;
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 4px;
    font-weight: 600;
  }
  .auto-badge.paused {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .auto-badge.on {
    background: var(--ok-soft);
    color: var(--ok);
  }
</style>
