<script lang="ts">
  import { ArrowRight, Copy, Check, RotateCcw } from "lucide-svelte";
  import MarkdownView from "./MarkdownView.svelte";
  import type { TurnView } from "../lib/api";
  import { tasks, selectTask, submitComposerInstruction, ACTIVE_TASK_STATUSES } from "../lib/stores/tasks";
  import { parseInstructionSkillPills } from "../lib/stores/search";
  import { t } from "../lib/i18n";

  let { turn }: { turn: TurnView } = $props();
  let copied = $state(false);
  let nowMs = $state(Date.now());

  const parsedInstruction = $derived(parseInstructionSkillPills(turn.instruction || ""));
  const linkedJob = $derived(turn.task_id ? $tasks.find((t) => t.id === turn.task_id) || null : null);
  const isJobRunning = $derived(Boolean(linkedJob && ACTIVE_TASK_STATUSES.has(linkedJob.status)));

  $effect(() => {
    if (!isJobRunning) return;
    nowMs = Date.now();
    const timer = setInterval(() => {
      nowMs = Date.now();
    }, 1000);
    return () => clearInterval(timer);
  });

  function formatLiveClock(sec: number): string {
    const s = Math.max(0, Math.floor(sec));
    const hrs = Math.floor(s / 3600);
    const mins = Math.floor((s % 3600) / 60);
    const rem = s % 60;
    const mm = String(mins).padStart(2, "0");
    const ss = String(rem).padStart(2, "0");
    if (hrs > 0) return `${hrs}:${mm}:${ss}`;
    return `${mm}:${ss}`;
  }

  const liveJobElapsedText = $derived.by(() => {
    if (!linkedJob) return "";
    if (isJobRunning) {
      const startStr = linkedJob.created_at || linkedJob.updated_at || turn.timestamp;
      if (!startStr) return "00:00";
      const ms = new Date(startStr).getTime();
      if (Number.isNaN(ms)) return "00:00";
      return formatLiveClock(Math.max(0, Math.floor((nowMs - ms) / 1000)));
    }
    const durNs = linkedJob.report?.duration;
    if (typeof durNs === "number" && durNs > 0) {
      const sec = Math.max(1, Math.round(durNs / 1e9));
      return formatLiveClock(sec);
    }
    return "";
  });

  function formatRelativeTime(ts?: string): string {
    if (!ts) return "";
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return "";
    const diffSec = Math.max(0, Math.floor((Date.now() - d.getTime()) / 1000));
    if (diffSec < 45) return "just now";
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    return d.toLocaleDateString();
  }

  const relativeTime = $derived(formatRelativeTime(turn.timestamp));

  async function onCopyInstruction() {
    try {
      await navigator.clipboard.writeText(turn.instruction || "");
      copied = true;
      setTimeout(() => {
        copied = false;
      }, 1500);
    } catch {
      // ignore clipboard errors
    }
  }

  const statusText = $derived.by(() => {
    if (linkedJob) {
      const st = linkedJob.status;
      if (ACTIVE_TASK_STATUSES.has(st)) return $t("Working…", "กำลังทำงาน…");
      if (st === "preview_ready") return $t("Preview ready · awaiting review", "Preview เสร็จแล้ว · รอตรวจทาน");
      if (st === "ready_to_apply") return $t("Verified · ready to apply", "ตรวจสอบผ่านแล้ว · พร้อม Apply");
      if (st === "succeeded")
        return linkedJob.report?.applied || turn.applied
          ? $t("Applied", "Applied เสร็จสมบูรณ์")
          : $t("Completed", "เสร็จสมบูรณ์");
      if (st === "interrupted") return $t("⏸ Paused · ready to resume", "⏸ พักงานชั่วคราว · พร้อมทำต่อ");
      if (st === "failed") return $t("Failed", "ไม่สำเร็จ");
      if (st === "cancelled") return $t("Cancelled", "ยกเลิกแล้ว");
      return st;
    }
    if (turn.applied) return $t("Applied", "Applied เสร็จสมบูรณ์");
    return turn.success ? $t("Completed", "เสร็จสิ้น") : $t("Pending review / follow-up", "รอตรวจสอบ / ดำเนินการต่อ");
  });

  const statusKind = $derived.by(() => {
    if (linkedJob) {
      const st = linkedJob.status;
      if (ACTIVE_TASK_STATUSES.has(st)) return "running";
      if (st === "interrupted") return "warn";
      if (st === "failed" || st === "cancelled") return "err";
      return "ok";
    }
    return turn.success || turn.applied ? "ok" : "neutral";
  });
</script>

<div class="turn-block" data-testid="conversation-turn">
  <div class="user-msg-group">
    <div class="user-bubble selectable">
      {#if parsedInstruction.skills.length > 0}
        {#each parsedInstruction.skills as sk (sk)}
          <span class="skill-pill mono" data-testid="skill-pill">
            <span class="skill-icon">&lt;&gt;</span>
            <span>{sk}</span>
          </span>
        {/each}
      {/if}
      <span>{parsedInstruction.cleanText || turn.instruction}</span>
    </div>
    <div class="user-meta-bar">
      {#if relativeTime}
        <span class="meta-time mono">{relativeTime}</span>
      {/if}
      <button
        type="button"
        class="meta-icon-btn"
        onclick={onCopyInstruction}
        title={$t("Copy instruction", "คัดลอกคำสั่ง")}
        aria-label={$t("Copy instruction", "คัดลอกคำสั่ง")}
      >
        {#if copied}
          <Check size={12} />
        {:else}
          <Copy size={12} />
        {/if}
      </button>
      <button
        type="button"
        class="meta-icon-btn"
        onclick={() => submitComposerInstruction(turn.instruction)}
        title={$t("Resend instruction", "ส่งคำสั่งนี้อีกครั้ง")}
        aria-label={$t("Resend instruction", "ส่งคำสั่งนี้อีกครั้ง")}
      >
        <RotateCcw size={12} />
      </button>
    </div>
  </div>

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
            class:warn={statusKind === "warn"}
            class:err={statusKind === "err"}
          >
            {statusText}{#if liveJobElapsedText} · ⏱ {liveJobElapsedText}{/if}
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
  .user-msg-group {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 4px;
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
  .user-meta-bar {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding-right: 4px;
    color: var(--faint);
    font-size: 11px;
  }
  .meta-icon-btn {
    border: none;
    background: transparent;
    color: var(--faint);
    padding: 2px 4px;
    border-radius: 4px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .meta-icon-btn:hover {
    color: var(--text);
    background: var(--hover);
  }
  .skill-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 1px 7px;
    margin-right: 6px;
    border-radius: 5px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    color: var(--text);
    font-size: 11.5px;
    font-weight: 600;
    vertical-align: baseline;
  }
  .skill-icon {
    color: var(--accent);
    font-weight: 700;
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
  .status-tag.warn {
    color: var(--warn);
  }
  .status-tag.err {
    color: var(--err);
  }
</style>
