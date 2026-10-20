<script lang="ts">
  import { ArrowRight } from "lucide-svelte";
  import type { TurnView } from "../lib/api";
  import { selectTask } from "../lib/stores/tasks";

  let { turn }: { turn: TurnView } = $props();
</script>

<div class="turn-block" data-testid="conversation-turn">
  <div class="user-bubble selectable">{turn.instruction}</div>

  {#if turn.summary || turn.task_id}
    <div class="assistant-out selectable">
      {#if turn.summary}
        <div class="summary-text">{turn.summary}</div>
      {/if}
      {#if turn.task_id}
        <button
          type="button"
          class="task-link mono"
          onclick={() => selectTask(turn.task_id!, true)}
          data-testid="turn-task-link"
        >
          <span>Task {turn.task_id.slice(0, 12)}</span>
          <span class="status-tag">{turn.applied ? "APPLIED" : turn.success ? "READY" : "STOPPED"}</span>
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
    color: var(--ok);
  }
</style>
