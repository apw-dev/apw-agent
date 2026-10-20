<script lang="ts">
  import { ChevronDown, ChevronRight, BookmarkPlus } from "lucide-svelte";
  import MarkdownView from "./MarkdownView.svelte";
  import type { TaskJob, TaskEvent } from "../lib/api";
  import { parseUnifiedDiff } from "../lib/diff";
  import {
    ACTIVE_TASK_STATUSES,
    RESUMABLE_TASK_STATUSES,
    selectedTaskId,
    mapTaskSteps,
    selectTask,
    applyTaskChanges,
    cancelTaskById,
    resumeTaskById,
    taskActionError
  } from "../lib/stores/tasks";
  import { createProceduralSkill, fetchWorkspaceFile } from "../lib/stores/workspace";
  import { rightCollapsed, rightPanelView, selectedDiffFile } from "../lib/stores/settings";

  let { job, events = [] }: { job: TaskJob; events?: TaskEvent[] } = $props();

  let logExpanded = $state(false);
  let busy = $state(false);
  let skillNotice = $state("");

  const stepBar = $derived(mapTaskSteps(job, events));
  const isRunning = $derived(ACTIVE_TASK_STATUSES.has(job.status));
  const canApply = $derived(job.status === "ready_to_apply");
  const canResume = $derived(RESUMABLE_TASK_STATUSES.has(job.status));
  const changedFiles = $derived.by(() => {
    const mod = job.report?.modified_files || [];
    if (mod.length > 0) return mod;
    return job.report?.plan?.affected_files || [];
  });
  const fileDiffs = $derived(parseUnifiedDiff(job.report?.diff || "", changedFiles));
  const diffStatsByFile = $derived.by(() => {
    const m = new Map<string, { added: number; deleted: number }>();
    for (const fd of fileDiffs) {
      m.set(fd.file, { added: fd.added, deleted: fd.deleted });
    }
    return m;
  });
  const totalAdded = $derived(fileDiffs.reduce((acc, f) => acc + f.added, 0));
  const totalDeleted = $derived(fileDiffs.reduce((acc, f) => acc + f.deleted, 0));
  const actionErr = $derived($taskActionError[job.id] || "");

  interface ActivityItem {
    key: string;
    verb: string;
    file?: string;
    added?: number;
    deleted?: number;
    detail?: string;
    status: "done" | "active" | "failed";
  }

  const activityItems = $derived.by((): ActivityItem[] => {
    const items: ActivityItem[] = [];
    const subtasksById = new Map(
      (job.report?.plan?.subtasks || []).map((st) => [st.id, st.description])
    );
    const steps = job.report?.steps || [];
    const seenFiles = new Set<string>();

    if (steps.length > 0) {
      for (const st of steps) {
        const act = (st.action || "").toLowerCase();
        const file = st.target_file || "";
        const stats = file ? diffStatsByFile.get(file) : undefined;
        let verb = "Executed step";
        if (act === "read" || act === "inspect") {
          verb = "Read";
        } else if (act === "patch" || act === "modify" || act === "edit" || act === "write" || act === "create") {
          verb = "Edited";
          if (file) seenFiles.add(file);
        } else if (act === "verify" || act === "test") {
          verb = "Ran verification";
        } else if (act) {
          verb = act.charAt(0).toUpperCase() + act.slice(1);
        }
        const planDesc = subtasksById.get(st.subtask_id) || "";
        const detail =
          planDesc && st.message && planDesc !== st.message
            ? `${planDesc} (${st.message})`
            : planDesc || st.message || "";
        items.push({
          key: st.subtask_id + "-" + file,
          verb,
          file: file || undefined,
          added: verb === "Edited" ? stats?.added : undefined,
          deleted: verb === "Edited" ? stats?.deleted : undefined,
          detail,
          status: st.success ? "done" : "failed"
        });
      }
    } else if (events.length > 0) {
      for (const ev of events) {
        const stg = (ev.stage || "").toLowerCase();
        if (stg === "planning" || stg === "step_start" || stg === "step_done" || stg === "verifying") {
          items.push({
            key: "ev-" + ev.seq,
            verb: stg === "planning" ? "Planned changes" : stg === "verifying" ? "Running verification" : "Step",
            detail: ev.message,
            status: isRunning && ev.seq === events[events.length - 1]?.seq ? "active" : "done"
          });
        }
      }
    }

    for (const fd of fileDiffs) {
      if (!seenFiles.has(fd.file)) {
        items.push({
          key: "diff-" + fd.file,
          verb: "Edited",
          file: fd.file,
          added: fd.added,
          deleted: fd.deleted,
          status: "done"
        });
      }
    }
    return items;
  });

  const latestLiveMessage = $derived.by(() => {
    if (!isRunning) return "";
    if (events.length > 0) {
      const last = events[events.length - 1];
      return `[${last.stage}] ${last.message}`;
    }
    switch (job.status) {
      case "queued":
      case "analyzing":
        return "กำลังวิเคราะห์โปรเจกต์และเตรียมสภาพแวดล้อม…";
      case "planning":
        return "กำลังวางแผนขั้นตอนการแก้ไขโค้ด (Architect)…";
      case "patching":
        return "กำลังอ่านและแก้ไขไฟล์ใน Isolated Worktree…";
      case "verifying":
        return "กำลังรันตรวจสอบโค้ดและเทสต์อัตโนมัติ (Reflex)…";
      case "applying":
        return "กำลังนำการเปลี่ยนแปลงไปใช้ในโปรเจกต์…";
      default:
        return "กำลังทำงาน…";
    }
  });

  function statusLabel(st: string): string {
    switch (st) {
      case "succeeded":
        return "✓ เสร็จสมบูรณ์ (Applied)";
      case "ready_to_apply":
        return "✓ ตรวจสอบผ่านแล้ว · พร้อม Apply";
      case "preview_ready":
        return "✓ สร้าง Preview เสร็จแล้ว";
      case "failed":
        return "✗ ไม่สำเร็จ (Failed)";
      case "cancelled":
        return "⊘ ยกเลิกแล้ว";
      case "queued":
      case "analyzing":
        return "กำลังวิเคราะห์…";
      case "planning":
        return "กำลังวางแผน…";
      case "patching":
        return "กำลังแก้ไขโค้ด…";
      case "verifying":
        return "กำลังรันทดสอบ…";
      case "applying":
        return "กำลัง Apply…";
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
    const alreadySelected = $selectedTaskId === job.id;
    void selectTask(job.id, false);
    rightCollapsed.set(false);
    const targetFile = file || changedFiles[0] || "";
    if (file) {
      selectedDiffFile.set(file);
      rightPanelView.set("diff");
    } else if (alreadySelected && !$rightCollapsed && $rightPanelView === "diff") {
      rightPanelView.set("cards");
    } else {
      if (targetFile) selectedDiffFile.set(targetFile);
      rightPanelView.set("diff");
    }
  }

  async function openActivityItem(item: ActivityItem) {
    if (!item.file) return;
    if (item.verb === "Read") {
      void selectTask(job.id, false);
      rightCollapsed.set(false);
      const res = await fetchWorkspaceFile(item.file, job.id);
      if (res.ok) {
        rightPanelView.set("file");
        return;
      }
    }
    openDiffFor(item.file);
  }
</script>

<div class="task-card" data-testid="task-card" data-task-id={job.id} data-task-status={job.status}>
  <div class="hdr">
    <div class="hdr-left">
      {#if isRunning}
        <span class="pulse-dot" aria-hidden="true"></span>
      {/if}
      <span class="mode-label">{(job.mode || "execute").toUpperCase()}</span>
      <span class="sep">·</span>
      <span>{changedFiles.length} ไฟล์</span>
      {#if totalAdded > 0 || totalDeleted > 0}
        <span class="diff-stat add mono">+{totalAdded}</span>
        <span class="diff-stat del mono">−{totalDeleted}</span>
      {/if}
      <span class="sep">·</span>
      <span class="instr">{job.instruction || job.id}</span>
    </div>
    <span
      class="status-pill"
      class:running={isRunning}
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

  {#if isRunning}
    <div class="live-banner">
      <span class="pulse-dot" aria-hidden="true"></span>
      <div class="live-text">
        <b>Agent กำลังทำงานอยู่ในขณะนี้</b>
        <div class="mono live-sub">{latestLiveMessage}</div>
      </div>
    </div>
  {/if}

  {#if job.report?.plan?.objective}
    <div class="plan-objective selectable">{job.report.plan.objective}</div>
  {/if}

  {#if activityItems.length > 0}
    <div class="activity-stream">
      {#each activityItems as item (item.key)}
        <div class="activity-block">
          {#if item.file}
            <button type="button" class="activity-row clickable" data-file-path={item.file} onclick={() => openActivityItem(item)}>
              <span class="act-verb">{item.verb}</span>
              <span class="act-file mono">{item.file}</span>
              {#if item.added !== undefined || item.deleted !== undefined}
                <span class="diff-stat add mono">+{item.added ?? 0}</span>
                <span class="diff-stat del mono">−{item.deleted ?? 0}</span>
              {/if}
              <span class="act-chevron">›</span>
            </button>
          {:else}
            <div class="activity-row">
              <span class="act-verb">{item.verb}</span>
            </div>
          {/if}
          {#if item.detail}
            <div class="activity-detail selectable">{item.detail}</div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if changedFiles.length > 0}
    <div class="file-chips">
      {#each changedFiles as f (f)}
        {@const st = diffStatsByFile.get(f)}
        <button type="button" class="file-chip mono" data-file-path={f} onclick={() => openDiffFor(f)}>
          <span>{f}</span>
          {#if st}
            <span class="diff-stat add">+{st.added}</span>
            <span class="diff-stat del">−{st.deleted}</span>
          {/if}
        </button>
      {/each}
    </div>
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
      <span>บันทึกเหตุการณ์ละเอียด ({events.length || job.report?.steps?.length || 0} รายการ)</span>
    </button>
    {#if logExpanded}
      <div class="log-box mono selectable">
        {#each events as ev (ev.seq)}
          <div>[{ev.stage}] {ev.message}</div>
        {/each}
        {#if events.length === 0 && job.report?.steps}
          {#each job.report.steps as st (st.subtask_id)}
            <div>{st.success ? "✓" : "✗"} [{st.action}] {st.target_file || st.subtask_id}: {st.message}</div>
          {/each}
        {/if}
      </div>
    {/if}
  {/if}

  {#if !isRunning}
    <div
      class="completion-box"
      class:ok={job.status === "succeeded" || job.status === "ready_to_apply" || job.status === "preview_ready"}
      class:err={job.status === "failed" || job.status === "cancelled"}
    >
      {#if job.report?.summary}
        <div class="summary selectable">
          <MarkdownView text={job.report.summary} taskId={job.id} />
        </div>
      {/if}

      <div class="completion-prompt">
        {#if job.status === "preview_ready"}
          <b>Agent สร้าง Preview Diff เสร็จแล้ว ({changedFiles.length} ไฟล์):</b> ยังไม่ได้รันทดสอบและยังไม่ได้แก้ไขไฟล์จริงในโปรเจกต์ ต้องการรันตรวจสอบเพื่อเตรียม Apply เลยไหม?
        {:else if job.status === "ready_to_apply"}
          <b>Agent แก้ไขและรันตรวจสอบผ่านครบแล้ว ({changedFiles.length} ไฟล์):</b> ต้องการนำการเปลี่ยนแปลงนี้ไปใช้ (Apply) ลงโปรเจกต์เลยไหม?
        {:else if job.status === "succeeded"}
          <b>✓ Agent ทำงานเสร็จสมบูรณ์แล้ว</b>
          {job.report?.applied ? " (นำการเปลี่ยนแปลงไปใช้ใน Workspace เรียบร้อยแล้ว)" : ""}
        {:else if job.status === "failed" || job.status === "cancelled"}
          <b>งานนี้หยุดลงก่อนเสร็จสมบูรณ์:</b> ต้องการสั่งให้ Agent ทำงานต่อจากจุดเดิม (Resume) หรือเปิดดู Diff ก่อนไหม?
        {/if}
      </div>

      <div class="actions">
        {#if canApply}
          <button
            type="button"
            class="btn primary"
            onclick={onApply}
            disabled={busy}
            data-testid="apply-task-btn"
          >
            Apply ลงโปรเจกต์เลย
          </button>
        {/if}

        {#if canResume}
          <button
            type="button"
            class="btn primary"
            onclick={onResume}
            disabled={busy}
            data-testid="resume-task-btn"
          >
            {job.status === "preview_ready" ? "รันตรวจสอบและเตรียม Apply (Resume)" : "ทำต่อ (Resume)"}
          </button>
        {/if}

        <button type="button" class="btn" onclick={() => openDiffFor()} data-testid="review-task-btn">
          {changedFiles.length > 0 ? `ดู diff / รีวิว (${changedFiles.length} ไฟล์)` : "ดู diff / รีวิว"}
        </button>

        {#if (job.status === "succeeded" || job.status === "ready_to_apply") && job.report?.passed}
          <button type="button" class="btn ghost" onclick={onSaveSkill} disabled={busy}>
            <BookmarkPlus size={12} />
            <span>Save as Skill</span>
          </button>
        {/if}
      </div>
    </div>
  {:else}
    <div class="actions">
      <button type="button" class="btn" onclick={() => openDiffFor()} data-testid="review-task-btn">
        ดู diff / รีวิว
      </button>
      <button
        type="button"
        class="btn"
        onclick={onCancel}
        disabled={busy}
        data-testid="cancel-task-btn"
      >
        Cancel
      </button>
    </div>
  {/if}
</div>

<style>
  .task-card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
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
    font-weight: 600;
    font-size: 11.5px;
    color: var(--muted);
    flex: none;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--panel-subtle);
  }
  .status-pill.running {
    color: var(--accent);
    background: var(--accent-soft);
  }
  .status-pill.ok {
    color: var(--ok);
    background: var(--ok-soft);
  }
  .status-pill.err {
    color: var(--err);
    background: var(--err-soft);
  }
  .pulse-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
    flex: none;
    animation: pulse 1.2s ease-in-out infinite;
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.35;
      transform: scale(0.85);
    }
  }
  .live-banner {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--accent);
    background: var(--accent-soft);
    font-size: 12px;
  }
  .live-text {
    min-width: 0;
    flex: 1;
  }
  .live-sub {
    font-size: 11px;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
  .plan-objective {
    font-size: 12.5px;
    color: var(--text);
    line-height: 1.55;
  }
  .activity-stream {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 6px 0;
    border-top: 1px solid var(--line);
    border-bottom: 1px solid var(--line);
  }
  .activity-block {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .activity-row {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--muted);
    border: none;
    background: transparent;
    padding: 2px 0;
    text-align: left;
    align-self: flex-start;
  }
  .activity-row.clickable:hover {
    color: var(--text);
  }
  .act-verb {
    color: var(--muted);
  }
  .act-file {
    color: var(--text);
    font-weight: 600;
  }
  .act-chevron {
    color: var(--faint);
    font-weight: 600;
  }
  .activity-detail {
    font-size: 12.5px;
    color: var(--text);
    line-height: 1.55;
    padding-left: 2px;
  }
  .diff-stat {
    font-size: 11px;
    font-weight: 600;
    padding: 0 5px;
    border-radius: 4px;
  }
  .diff-stat.add {
    color: var(--addt);
    background: var(--add);
  }
  .diff-stat.del {
    color: var(--delt);
    background: var(--del);
  }
  .file-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .file-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11px;
  }
  .file-chip:hover {
    background: var(--hover);
  }
  .completion-box {
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    background: var(--bg);
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .summary {
    font-size: 12.5px;
    color: var(--text);
    white-space: pre-wrap;
    line-height: 1.6;
  }
  .completion-prompt {
    font-size: 12.5px;
    color: var(--text);
    line-height: 1.55;
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
    padding: 5px 12px;
    border-radius: 6px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
    font-weight: 500;
  }
  .btn:hover:not(:disabled) {
    background: var(--hover);
  }
  .btn.primary {
    background: var(--text);
    color: var(--panel);
    border-color: var(--text);
    font-weight: 600;
  }
  .btn.ghost {
    margin-left: auto;
    color: var(--muted);
  }
</style>
