<script lang="ts">
  import { ArrowRight } from "lucide-svelte";
  import MarkdownView from "./MarkdownView.svelte";
  import type { TurnView } from "../lib/api";
  import { tasks, selectTask, ACTIVE_TASK_STATUSES } from "../lib/stores/tasks";

  let { turn }: { turn: TurnView } = $props();

  const linkedJob = $derived(turn.task_id ? $tasks.find((t) => t.id === turn.task_id) || null : null);

  const statusText = $derived.by(() => {
    if (linkedJob) {
      const st = linkedJob.status;
      if (ACTIVE_TASK_STATUSES.has(st)) return "กำลังทำงาน…";
      if (st === "preview_ready") return "Preview เสร็จแล้ว · รอตรวจทาน";
      if (st === "ready_to_apply") return "ตรวจสอบผ่านแล้ว · พร้อม Apply";
      if (st === "succeeded") return linkedJob.report?.applied || turn.applied ? "Applied เสร็จสมบูรณ์" : "เสร็จสมบูรณ์";
      if (st === "failed") return "ไม่สำเร็จ";
      if (st === "cancelled") return "ยกเลิกแล้ว";
      return st;
    }
    if (turn.applied) return "Applied เสร็จสมบูรณ์";
    return turn.success ? "เสร็จสิ้น" : "รอตรวจสอบ / ดำเนินการต่อ";
  });

  const statusKind = $derived.by(() => {
    if (linkedJob) {
      const st = linkedJob.status;
      if (ACTIVE_TASK_STATUSES.has(st)) return "running";
      if (st === "failed" || st === "cancelled") return "err";
      return "ok";
    }
    return turn.success || turn.applied ? "ok" : "neutral";
  });
</script>

<div class="turn-block" data-testid="conversation-turn">
  <div class="user-bubble selectable">{turn.instruction}</div>

  {#if (!linkedJob && turn.summary) || turn.task_id}
    <div class="assistant-out selectable">
      {#if !linkedJob && turn.summary}
        <div class="summary-text">
          <MarkdownView text={turn.summary} taskId={turn.task_id} />
        </div>
      {/if}
      {#if turn.task_id}
        <button
          type="button"
          class="task-link mono"
          onclick={() => selectTask(turn.task_id!, true)}
          data-testid="turn-task-link"
        >
          <span>Task {turn.task_id.slice(0, 12)}</span>
          <span
            class="status-tag"
            class:ok={statusKind === "ok"}
            class:running={statusKind === "running"}
            class:err={statusKind === "err"}
          >
            {statusText}
          </span>
          <ArrowRight size={12} />
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .turn-block {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .user-bubble {
    align-self: flex-end;
    background: var(--user-bubble);
    color: var(--text);
    border-radius: 12px;
    padding: 8px 12px;
    max-width: 82%;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .assistant-out {
    display: flex;
    flex-direction: column;
    gap: 6px;
    line-height: 1.6;
    color: var(--text);
  }
  .summary-text {
    white-space: pre-wrap;
    word-break: break-word;
  }
  .task-link {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 9px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    font-size: 11px;
  }
  .task-link:hover {
    border-color: var(--line-strong);
    color: var(--text);
  }
  .status-tag {
    font-weight: 600;
    color: var(--muted);
  }
  .status-tag.ok {
    color: var(--ok);
  }
  .status-tag.running {
    color: var(--accent);
  }
  .status-tag.err {
    color: var(--err);
  }
</style>
