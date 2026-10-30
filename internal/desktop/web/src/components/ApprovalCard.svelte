<script lang="ts">
  import { ShieldQuestion, Check, CheckCheck, X } from "lucide-svelte";
  import type { ToolApprovalRequest } from "../lib/api";
  import { decideApproval } from "../lib/stores/tasks";
  import { t } from "../lib/i18n";

  let { request }: { request: ToolApprovalRequest } = $props();
  let busy = $state(false);
  let errorMsg = $state("");

  async function onDecide(decision: "once" | "always" | "deny") {
    if (busy) return;
    busy = true;
    errorMsg = "";
    const res = await decideApproval(request.id, decision);
    if (!res.ok) {
      errorMsg = res.message;
    }
    busy = false;
  }
</script>

<div class="approval-card" role="region" aria-label={$t("Tool approval request", "คำขออนุมัติคำสั่ง")} data-testid="approval-card">
  <div class="hdr">
    <ShieldQuestion size={16} />
    <span class="title">{$t(`Tool Approval Request (${request.tool_name})`, `ขออนุญาตดำเนินการ (${request.tool_name})`)}</span>
    {#if request.task_id}
      <span class="task-id mono">{request.task_id}</span>
    {/if}
  </div>

  <div class="desc selectable">{request.description}</div>

  {#if request.command}
    <pre class="cmd mono selectable">{request.command}</pre>
  {/if}

  {#if errorMsg}
    <div class="err">{errorMsg}</div>
  {/if}

  <div class="actions">
    <button
      type="button"
      class="btn primary"
      onclick={() => onDecide("once")}
      disabled={busy}
      data-testid="approve-once-btn"
    >
      <Check size={13} />
      <span>{$t("Approve Once", "อนุญาตครั้งเดียว (Approve)")}</span>
    </button>
    <button
      type="button"
      class="btn"
      onclick={() => onDecide("always")}
      disabled={busy}
      data-testid="approve-always-btn"
    >
      <CheckCheck size={13} />
      <span>{$t("Always Allow", "อนุญาตเสมอ")}</span>
    </button>
    <button
      type="button"
      class="btn danger"
      onclick={() => onDecide("deny")}
      disabled={busy}
      data-testid="approve-deny-btn"
    >
      <X size={13} />
      <span>{$t("Deny", "ปฏิเสธ (Deny)")}</span>
    </button>
  </div>
</div>

<style>
  .approval-card {
    border: 1px solid var(--accent);
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
    align-items: center;
    gap: 7px;
    font-weight: 600;
    color: var(--accent);
  }
  .task-id {
    margin-left: auto;
    font-size: 11px;
    color: var(--faint);
    font-weight: 400;
  }
  .desc {
    font-size: 12.5px;
    color: var(--text);
  }
  .cmd {
    margin: 0;
    padding: 6px 8px;
    border-radius: 6px;
    background: var(--panel-subtle);
    font-size: 11.5px;
    overflow-x: auto;
  }
  .err {
    font-size: 11.5px;
    color: var(--err);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 11px;
    border-radius: 6px;
    border: 1px solid var(--line);
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
  }
  .btn.danger {
    color: var(--err);
    border-color: var(--err-soft);
  }
</style>
