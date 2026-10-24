<script lang="ts">
  import { ArrowDown } from "lucide-svelte";
  import Message from "./Message.svelte";
  import TaskCard from "./TaskCard.svelte";
  import ApprovalCard from "./ApprovalCard.svelte";
  import RunConsoleDock from "./RunConsoleDock.svelte";
  import Composer from "./Composer.svelte";
  import { sessionTurns, currentSessionId } from "../lib/stores/sessions";
  import type { TurnView, TaskJob } from "../lib/api";
  import {
    tasks,
    selectedTask,
    taskEvents,
    pendingApprovals,
    latestPlans,
    submitComposerInstruction
  } from "../lib/stores/tasks";

  let scrollEl: HTMLDivElement | undefined = $state();
  let showScrollDown = $state(false);
  let lastFeedSignature = "";
  let lastApprovalCount = 0;
  let lastTurnCount = 0;
  let lastTaskCount = 0;

  function onFeedScroll() {
    if (!scrollEl) return;
    const distanceFromBottom = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
    showScrollDown = distanceFromBottom > 80;
  }

  function scrollToBottom(smooth = true) {
    if (!scrollEl) return;
    scrollEl.scrollTo({
      top: scrollEl.scrollHeight,
      behavior: smooth ? "smooth" : "auto"
    });
    showScrollDown = false;
  }

  const visibleTasks = $derived.by(() => {
    const sid = $currentSessionId;
    const all = $tasks;
    const sel = $selectedTask;
    const turnTaskIds = new Set($sessionTurns.map((t) => t.task_id).filter(Boolean));
    // /v1/tasks returns newest first; reverse so base order is oldest -> newest
    const chronologicalAll = [...all].reverse();
    const bySession = sid
      ? chronologicalAll.filter((j) => j.session_id === sid || turnTaskIds.has(j.id))
      : [];
    const map = new Map<string, (typeof all)[number]>();
    for (const j of bySession) map.set(j.id, j);
    if (sel && (!sid || sel.session_id === sid || turnTaskIds.has(sel.id))) map.set(sel.id, sel);
    const list = Array.from(map.values());
    list.sort((a, b) => {
      const ta = a.created_at ? new Date(a.created_at).getTime() : 0;
      const tb = b.created_at ? new Date(b.created_at).getTime() : 0;
      if (ta && tb && ta !== tb) return ta - tb;
      return 0;
    });
    return list;
  });

  interface ConversationEntry {
    key: string;
    turn: TurnView;
    job?: TaskJob;
    sortTimeMs: number;
    sortOrder: number;
  }

  const conversationEntries = $derived.by((): ConversationEntry[] => {
    const allTasksById = new Map($tasks.map((j) => [j.id, j]));
    const entries: ConversationEntry[] = [];
    const seenTaskIds = new Set<string>();
    let order = 0;

    for (const turn of $sessionTurns) {
      const job = turn.task_id ? allTasksById.get(turn.task_id) : undefined;
      if (turn.task_id) {
        seenTaskIds.add(turn.task_id);
      }
      const tStr = job?.created_at || turn.timestamp || "";
      const ms = tStr ? new Date(tStr).getTime() : 0;
      entries.push({
        key: "turn-" + turn.index + "-" + (turn.task_id || turn.timestamp),
        turn,
        job,
        sortTimeMs: Number.isNaN(ms) ? 0 : ms,
        sortOrder: order++
      });
    }

    for (const job of visibleTasks) {
      if (seenTaskIds.has(job.id)) continue;
      seenTaskIds.add(job.id);
      const tStr = job.created_at || job.updated_at || "";
      const ms = tStr ? new Date(tStr).getTime() : 0;
      const syntheticTurn: TurnView = {
        index: order + 1,
        task_id: job.id,
        timestamp: tStr,
        instruction: job.instruction || job.id,
        success:
          job.status === "succeeded" ||
          job.status === "ready_to_apply" ||
          job.status === "preview_ready",
        applied: Boolean(job.report?.applied),
        summary: job.report?.summary || ""
      };
      entries.push({
        key: "task-" + job.id,
        turn: syntheticTurn,
        job,
        sortTimeMs: Number.isNaN(ms) ? 0 : ms,
        sortOrder: order++
      });
    }

    entries.sort((a, b) => {
      if (a.sortTimeMs > 0 && b.sortTimeMs > 0 && a.sortTimeMs !== b.sortTimeMs) {
        return a.sortTimeMs - b.sortTimeMs;
      }
      return a.sortOrder - b.sortOrder;
    });

    return entries;
  });

  $effect(() => {
    const evMap = $taskEvents;
    const evTotal = visibleTasks.reduce((sum, j) => sum + (evMap[j.id]?.length || 0), 0);
    const statusSig = visibleTasks.map((j) => `${j.id}:${j.status}`).join("|");
    const approvalCount = $pendingApprovals.length;
    const turnCount = $sessionTurns.length;
    const taskCount = visibleTasks.length;
    const sig = `${turnCount}|${taskCount}|${approvalCount}|${$latestPlans.length}|${evTotal}|${statusSig}`;

    const forceScroll =
      approvalCount > lastApprovalCount || turnCount > lastTurnCount || taskCount > lastTaskCount;
    lastApprovalCount = approvalCount;
    lastTurnCount = turnCount;
    lastTaskCount = taskCount;

    if (sig !== lastFeedSignature) {
      lastFeedSignature = sig;
      queueMicrotask(() => {
        if (forceScroll || !showScrollDown) {
          scrollToBottom(forceScroll);
        } else {
          onFeedScroll();
        }
      });
    }
  });
</script>

<main class="thread-view" aria-label="บทสนทนาและงาน">
  <div class="thread-scroll" id="feed" bind:this={scrollEl} onscroll={onFeedScroll}>
    <div class="col">
      {#if $sessionTurns.length === 0 && visibleTasks.length === 0 && $latestPlans.length === 0}
        <div class="empty-state">
          <h2>APW Desktop</h2>
          <p>สั่งงานตรวจสอบ แก้ไข หรือสร้างฟีเจอร์ใหม่ใน Isolated Git Worktree พร้อมตรวจสอบอัตโนมัติ</p>
          <div class="quick-row">
            <button
              type="button"
              class="q-chip"
              onclick={() => submitComposerInstruction("ตรวจสอบโครงสร้างโค้ดและสรุปจุดสำคัญ", "review")}
            >
              รีวิวโค้ดในโปรเจกต์
            </button>
            <button
              type="button"
              class="q-chip"
              onclick={() => submitComposerInstruction("เตรียมการเปลี่ยนแปลงในโหมด Preview", "preview")}
            >
              ทดลองสร้าง Preview Diff
            </button>
          </div>
        </div>
      {/if}

      {#each conversationEntries as entry (entry.key)}
        <div class="conv-pair">
          <Message turn={entry.turn} />
          {#if entry.job}
            <TaskCard job={entry.job} events={$taskEvents[entry.job.id] || []} />
          {/if}
        </div>
      {/each}

      {#each $latestPlans as item (item.id)}
        <div class="plan-card" data-testid="plan-card">
          <div class="plan-hdr">ExecutionPlan · {item.plan.plan_id}</div>
          <div class="plan-obj selectable">{item.plan.objective}</div>
          {#if item.plan.subtasks?.length}
            <ul class="plan-list selectable">
              {#each item.plan.subtasks as st (st.id)}
                <li><b class="mono">[{st.action}]</b> {st.target_file || st.id}: {st.description}</li>
              {/each}
            </ul>
          {/if}
        </div>
      {/each}

      {#each $pendingApprovals as req (req.id)}
        <ApprovalCard request={req} />
      {/each}
    </div>
  </div>

  {#if showScrollDown}
    <div class="scroll-down-anchor">
      <button
        type="button"
        class="scroll-down-btn"
        onclick={() => scrollToBottom(true)}
        aria-label="เลื่อนลงล่างสุด"
        title="เลื่อนลงล่างสุด"
        data-testid="scroll-down-btn"
      >
        <ArrowDown size={16} />
      </button>
    </div>
  {/if}

  <RunConsoleDock />

  <Composer />
</main>

<style>
  .thread-view {
    position: relative;
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    height: 100%;
    background: var(--bg);
  }
  .thread-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 20px 0;
  }
  .scroll-down-anchor {
    display: flex;
    justify-content: center;
    height: 0;
    overflow: visible;
    position: relative;
    z-index: 20;
  }
  .scroll-down-btn {
    position: absolute;
    bottom: 8px;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    color: var(--text);
    box-shadow: var(--shadow-sm);
    display: grid;
    place-items: center;
    transition: transform 0.15s ease, background 0.15s ease;
  }
  .scroll-down-btn:hover {
    background: var(--hover);
    transform: translateY(-1px);
  }
  .col {
    max-width: 760px;
    margin: 0 auto;
    padding: 0 20px;
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .conv-pair {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .empty-state {
    margin: 48px auto 16px;
    text-align: center;
    max-width: 460px;
    color: var(--muted);
  }
  .empty-state h2 {
    margin: 0 0 6px;
    font-size: 17px;
    color: var(--text);
  }
  .empty-state p {
    margin: 0 0 14px;
    font-size: 13px;
  }
  .quick-row {
    display: flex;
    justify-content: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .q-chip {
    padding: 6px 12px;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
  }
  .q-chip:hover {
    background: var(--hover);
  }
  .plan-card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .plan-hdr {
    font-weight: 600;
    font-size: 12px;
    color: var(--accent);
  }
  .plan-obj {
    font-size: 13px;
  }
  .plan-list {
    margin: 4px 0 0;
    padding-left: 18px;
    font-size: 12px;
    color: var(--muted);
  }
</style>
