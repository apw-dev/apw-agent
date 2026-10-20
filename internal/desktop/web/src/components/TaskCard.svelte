<script lang="ts">
  import { ChevronDown, ChevronRight, Sparkles } from "lucide-svelte";
  import type { TaskJob, TaskEvent } from "../lib/api";
  import {
    ACTIVE_TASK_STATUSES,
    mapTaskSteps,
    selectTask,
    applyTaskChanges,
    cancelTaskById,
    resumeTaskById,
    taskActionError
  } from "../lib/stores/tasks";
  import { createProceduralSkill } from "../lib/stores/workspace";
  import { rightCollapsed, rightPanelView, selectedDiffFile } from "../lib/stores/settings";

  let { job, events = [] }: { job: TaskJob; events?: TaskEvent[] } = $props();

  let logExpanded = $state(false);
  let busy = $state(false);
  let skillNotice = $state("");

  const stepBar = $derived(mapTaskSteps(job, events));
  const isRunning = $derived(ACTIVE_TASK_STATUSES.has(job.status));
  const canApply = $derived(job.status === "ready_to_apply");
  const canResume = $derived(
    job.status === "failed" || job.status === "cancelled" || job.status === "preview_ready"
  );
  const changedFiles = $derived.by(() => {
    const mod = job.report?.modified_files || [];
    if (mod.length > 0) return mod;
    return job.report?.plan?.affected_files || [];
  });
  const actionErr = $derived($taskActionError[job.id] || "");

  function statusLabel(st: string): string {
    switch (st) {
      case "succeeded":
        return "✓ Applied";
      case "ready_to_apply":
        return "✓ Verified";
      case "preview_ready":
        return "✓ Preview ready";
      case "failed":
        return "✗ Failed";
      case "cancelled":
        return "⊘ Cancelled";
      default:
        return st;
    }
  }

  async function onApply() {
    if (busy) return;
    busy = true;
    await applyTaskChanges(job.id, "ask");
    busy = false;
  }

  async function onCancel() {
    if (busy) return;
    busy = true;
    await cancelTaskById(job.id);
    busy = false;
  }

  async function onResume() {
    if (busy) return;
    busy = true;
    await resumeTaskById(job.id);
    busy = false;
  }

  async function onSaveSkill() {
    if (busy) return;
    busy = true;
    skillNotice = "";
    const res = await createProceduralSkill({
      task_id: job.id,
      slug: "skill-" + job.id,
      title: job.instruction,
      instruction: job.instruction
    });
    skillNotice = res.ok ? `บันทึก Skill แล้ว (${res.path})` : res.message;
    busy = false;
  }

  function openDiffFor(file?: string) {
    void selectTask(job.id, true);
    rightCollapsed.set(false);
    if (file) {
      selectedDiffFile.set(file);
      rightPanelView.set("diff");
    } else {
      rightPanelView.set("cards");
    }
  }
</script>

<div class="task-card" data-testid="task-card" data-task-id={job.id} data-task-status={job.status}>
  <div class="hdr">
    <div class="hdr-left">
      <span class="mode-label">{(job.mode || "execute").toUpperCase()}</span>
      <span class="sep">·</span>
      <span>{changedFiles.length} ไฟล์</span>
      <span class="sep">·</span>
      <span class="instr">{job.instruction || job.id}</span>
    </div>
    <span
      class="status-pill"
      class:ok={job.status === "succeeded" || job.status === "ready_to_apply" || job.status === "preview_ready"}
      class:err={job.status === "failed" || job.status === "cancelled"}
    >
      {statusLabel(job.status)}
    </span>
  </div>

  <div class="steps" role="list" aria-label="ขั้นตอนของงาน">
    {#each stepBar.steps as step (step.label)}
      <i
        class="step-bar"
        class:done={step.state === "done"}
        class:active={step.state === "active"}
        class:failed={step.state === "failed"}
        data-step={step.label}
        data-state={step.state}
      ></i>
    {/each}
  </div>
  <div class="step-labels">
    {#each stepBar.steps as step (step.label)}
      <span class:active={step.state === "active" || step.state === "done"}>{step.label}</span>
    {/each}
  </div>

  {#if changedFiles.length > 0}
    <div class="file-chips">
      {#each changedFiles as f (f)}
        <button type="button" class="file-chip mono" onclick={() => openDiffFor(f)}>
          {f}
        </button>
      {/each}
    </div>
  {/if}

  {#if job.report?.summary}
    <div class="summary selectable">{job.report.summary}</div>
  {/if}

  {#if job.error || actionErr}
    <div class="error-box selectable" data-testid="task-error">
      {actionErr || job.error}
    </div>
  {/if}

  {#if skillNotice}
    <div class="skill-notice">{skillNotice}</div>
  {/if}

  {#if events.length > 0 || job.report?.steps?.length}
    <button type="button" class="log-toggle" onclick={() => (logExpanded = !logExpanded)}>
      {#if logExpanded}
        <ChevronDown size={13} />
      {:else}
        <ChevronRight size={13} />
      {/if}
      <span>บันทึกการทำงาน ({events.length || job.report?.steps?.length || 0} รายการ)</span>
    </button>
    {#if logExpanded}
      <div class="log-box mono selectable">
        {#each events as ev (ev.seq)}
          <div>[{ev.stage}] {ev.message}</div>
        {/each}
        {#if events.length === 0 && job.report?.steps}
          {#each job.report.steps as st (st.subtask_id)}
            <div>{st.success ? "✓" : "✗"} [{st.action}] {st.target_file || st.subtask_id} — {st.message}</div>
          {/each}
        {/if}
      </div>
    {/if}
  {/if}

  <div class="actions">
    <button type="button" class="btn" onclick={() => openDiffFor()} data-testid="review-task-btn">
      ดู diff / รีวิว
    </button>

    {#if canApply}
      <button
        type="button"
        class="btn primary"
        onclick={onApply}
        disabled={busy}
        data-testid="apply-task-btn"
      >
        Apply
      </button>
    {/if}

    {#if isRunning}
      <button
        type="button"
        class="btn"
        onclick={onCancel}
        disabled={busy}
        data-testid="cancel-task-btn"
      >
        Cancel
      </button>
    {/if}

    {#if canResume}
      <button
        type="button"
        class="btn"
        onclick={onResume}
        disabled={busy}
        data-testid="resume-task-btn"
      >
        Resume
      </button>
    {/if}

    {#if (job.status === "succeeded" || job.status === "ready_to_apply") && job.report?.passed}
      <button type="button" class="btn ghost" onclick={onSaveSkill} disabled={busy}>
        <Sparkles size={12} />
        <span>Save as Skill</span>
      </button>
    {/if}
  </div>
</div>

<style>
  .task-card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    box-shadow: var(--shadow-sm);
  }
  .hdr {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }
  .hdr-left {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    flex: 1;
  }
  .mode-label {
    font-size: 11px;
    color: var(--muted);
  }
  .sep {
    color: var(--faint);
  }
  .instr {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
  }
  .status-pill {
    font-weight: 500;
    font-size: 11.5px;
    color: var(--muted);
    flex: none;
  }
  .status-pill.ok {
    color: var(--ok);
  }
  .status-pill.err {
    color: var(--err);
  }
  .steps {
    display: flex;
    gap: 4px;
  }
  .step-bar {
    flex: 1;
    height: 3.5px;
    border-radius: 2px;
    background: var(--line);
  }
  .step-bar.done {
    background: var(--ok);
  }
  .step-bar.active {
    background: var(--accent);
  }
  .step-bar.failed {
    background: var(--err);
  }
  .step-labels {
    display: flex;
    justify-content: space-between;
    color: var(--faint);
    font-size: 11px;
  }
  .step-labels span.active {
    color: var(--muted);
    font-weight: 500;
  }
  .file-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .file-chip {
    padding: 2px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11px;
  }
  .file-chip:hover {
    background: var(--hover);
  }
  .summary {
    font-size: 12.5px;
    color: var(--text);
    white-space: pre-wrap;
    line-height: 1.6;
  }
  .error-box {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 12px;
    white-space: pre-wrap;
  }
  .skill-notice {
    font-size: 11.5px;
    color: var(--ok);
  }
  .log-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 0;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 11.5px;
    align-self: flex-start;
  }
  .log-box {
    padding: 8px;
    border-radius: 6px;
    background: var(--panel-subtle);
    font-size: 11px;
    max-height: 160px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding-top: 2px;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
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
  .btn.ghost {
    margin-left: auto;
    color: var(--muted);
  }
</style>
