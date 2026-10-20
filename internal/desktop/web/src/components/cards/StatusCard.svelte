<script lang="ts">
  import { ChevronDown, ChevronRight } from "lucide-svelte";
  import type { TaskJob, TaskEvent } from "../../lib/api";
  import { mapTaskSteps } from "../../lib/stores/tasks";
  import { collapsedCards, toggleCardCollapsed } from "../../lib/stores/settings";

  let { job, events = [] }: { job: TaskJob; events?: TaskEvent[] } = $props();

  const collapsed = $derived(Boolean($collapsedCards["status"]));
  const stepBar = $derived(mapTaskSteps(job, events));

  function formatStatusBadge(st: string): string {
    if (st === "ready_to_apply") return "✓ Verified";
    if (st === "succeeded") return "✓ Applied";
    if (st === "preview_ready") return "✓ Preview";
    return st;
  }
</script>

<section class="card" data-testid="status-card">
  <button type="button" class="card-hdr" onclick={() => toggleCardCollapsed("status")}>
    <span class="title">
      {#if collapsed}
        <ChevronRight size={13} />
      {:else}
        <ChevronDown size={13} />
      {/if}
      <span>สถานะ</span>
    </span>
    <span
      class="badge"
      class:ok={job.status === "ready_to_apply" || job.status === "succeeded" || job.status === "preview_ready"}
      class:err={job.status === "failed" || job.status === "cancelled"}
    >
      {formatStatusBadge(job.status)}
    </span>
  </button>

  {#if !collapsed}
    <div class="body">
      <div class="row">
        <span>Mode</span>
        <b class="mono">{job.mode || "execute"}</b>
      </div>
      <div class="row">
        <span>Status</span>
        <b class="mono">{job.status}</b>
      </div>
      <div class="row">
        <span>Task ID</span>
        <b class="mono selectable">{job.id}</b>
      </div>
      {#if job.worktree_path}
        <div class="row">
          <span>Worktree</span>
          <b class="mono selectable truncate" title={job.worktree_path}>{job.worktree_path}</b>
        </div>
      {/if}

      <div class="steps">
        {#each stepBar.steps as s (s.label)}
          <i
            class="bar"
            class:done={s.state === "done"}
            class:active={s.state === "active"}
            class:failed={s.state === "failed"}
          ></i>
        {/each}
      </div>
      <div class="slab">
        {#each stepBar.steps as s (s.label)}
          <span>{s.label}</span>
        {/each}
      </div>
    </div>
  {/if}
</section>

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
  .badge.ok {
    color: var(--ok);
  }
  .badge.err {
    color: var(--err);
  }
  .body {
    margin-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
  }
  .row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    color: var(--muted);
  }
  .row b {
    font-weight: 500;
    color: var(--text);
  }
  .truncate {
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .steps {
    display: flex;
    gap: 4px;
    margin-top: 4px;
  }
  .bar {
    flex: 1;
    height: 3px;
    border-radius: 2px;
    background: var(--line);
  }
  .bar.done {
    background: var(--ok);
  }
  .bar.active {
    background: var(--accent);
  }
  .bar.failed {
    background: var(--err);
  }
  .slab {
    display: flex;
    justify-content: space-between;
    font-size: 10.5px;
    color: var(--faint);
  }
</style>
