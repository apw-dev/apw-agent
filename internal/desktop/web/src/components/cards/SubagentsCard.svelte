<script lang="ts">
  import { ChevronDown, ChevronRight, RefreshCw, Check, AlertCircle } from "lucide-svelte";
  import type { TaskJob, TaskEvent, SubagentReport } from "../../lib/api";
  import { extractTaskSubagents } from "../../lib/stores/tasks";
  import { fetchWorkspaceFile } from "../../lib/stores/workspace";
  import { collapsedCards, toggleCardCollapsed, rightPanelView } from "../../lib/stores/settings";

  let { job, events = [] }: { job: TaskJob; events?: TaskEvent[] } = $props();

  const collapsed = $derived(Boolean($collapsedCards["subagents"]));
  const subagents = $derived(extractTaskSubagents(job, events));
  const runningCount = $derived(subagents.filter((s) => s.status === "running").length);

  async function openSubagentTarget(s: SubagentReport) {
    if (!s.target_file) return;
    const res = await fetchWorkspaceFile(s.target_file, job.id);
    if (res.ok) {
      rightPanelView.set("file");
    }
  }
</script>

{#if subagents.length > 0}
  <section class="card" data-testid="subagents-card">
    <button type="button" class="card-hdr" onclick={() => toggleCardCollapsed("subagents")}>
      <span class="title">
        {#if collapsed}
          <ChevronRight size={13} />
        {:else}
          <ChevronDown size={13} />
        {/if}
        <span>Subagents ({subagents.length})</span>
      </span>
      <span class="badge" class:running={runningCount > 0} class:ok={runningCount === 0}>
        {#if runningCount > 0}
          {runningCount} running
        {:else}
          ✓ {subagents.length} done
        {/if}
      </span>
    </button>

    {#if !collapsed}
      <div class="body">
        {#each subagents as sub (sub.id)}
          <button
            type="button"
            class="sub-row"
            class:clickable={Boolean(sub.target_file)}
            onclick={() => openSubagentTarget(sub)}
            title={sub.detail || sub.target_file || sub.role}
            data-testid="subagent-item"
          >
            <span
              class="sub-icon"
              class:spin={sub.status === "running"}
              class:ok={sub.status === "done"}
              class:err={sub.status === "failed"}
            >
              {#if sub.status === "running"}
                <RefreshCw size={12} />
              {:else if sub.status === "failed"}
                <AlertCircle size={12} />
              {:else}
                <Check size={12} />
              {/if}
            </span>
            <div class="sub-info">
              <div class="sub-role">{sub.role}</div>
              {#if sub.target_file}
                <div class="sub-target mono truncate">{sub.target_file}</div>
              {/if}
            </div>
          </button>
        {/each}
      </div>
    {/if}
  </section>
{/if}

<style>
  .card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 8px 10px;
  }
  .card-hdr {
    width: 100%;
    border: none;
    background: transparent;
    padding: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
  }
  .title {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .badge {
    font-weight: 500;
    font-size: 11px;
    color: var(--faint);
  }
  .badge.running {
    color: var(--accent);
    font-weight: 600;
  }
  .badge.ok {
    color: var(--ok);
  }
  .body {
    margin-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .sub-row {
    width: 100%;
    border: none;
    background: transparent;
    padding: 5px 6px;
    border-radius: 6px;
    display: flex;
    align-items: center;
    gap: 8px;
    text-align: left;
    color: var(--text);
  }
  .sub-row.clickable:hover {
    background: var(--hover);
  }
  .sub-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--muted);
    flex: none;
  }
  .sub-icon.spin {
    color: var(--accent);
    animation: spin 1.2s linear infinite;
  }
  .sub-icon.ok {
    color: var(--ok);
  }
  .sub-icon.err {
    color: var(--err);
  }
  @keyframes spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }
  .sub-info {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .sub-role {
    font-size: 12px;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub-target {
    font-size: 10.5px;
    color: var(--faint);
  }
  .truncate {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
