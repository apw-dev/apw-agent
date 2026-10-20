<script lang="ts">
  import Message from "./Message.svelte";
  import TaskCard from "./TaskCard.svelte";
  import ApprovalCard from "./ApprovalCard.svelte";
  import Composer from "./Composer.svelte";
  import { sessionTurns, currentSessionId } from "../lib/stores/sessions";
  import {
    tasks,
    selectedTask,
    taskEvents,
    pendingApprovals,
    latestPlans,
    submitComposerInstruction
  } from "../lib/stores/tasks";

  const visibleTasks = $derived.by(() => {
    const sid = $currentSessionId;
    const all = $tasks;
    const sel = $selectedTask;
    const bySession = sid ? all.filter((j) => j.session_id === sid) : [];
    const map = new Map<string, (typeof all)[number]>();
    for (const j of bySession) map.set(j.id, j);
    if (sel) map.set(sel.id, sel);
    if (map.size === 0 && all.length > 0) {
      for (const j of all.slice(0, 4)) map.set(j.id, j);
    }
    return Array.from(map.values());
  });
</script>

<main class="thread-view" aria-label="บทสนทนาและงาน">
  <div class="thread-scroll" id="feed">
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

      {#each $sessionTurns as turn (turn.index + "-" + (turn.task_id || turn.timestamp))}
        <Message {turn} />
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

      {#each visibleTasks as job (job.id)}
        <TaskCard {job} events={$taskEvents[job.id] || []} />
      {/each}
    </div>
  </div>

  <Composer />
</main>

<style>
  .thread-view {
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
  .col {
    max-width: 760px;
    margin: 0 auto;
    padding: 0 20px;
    display: flex;
    flex-direction: column;
    gap: 14px;
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
