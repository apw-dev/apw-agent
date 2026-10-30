<script lang="ts">
  import { X, Copy, Check, Terminal, Clock, FileCode, AlertTriangle, Cpu } from "lucide-svelte";
  import type { PromptTrace } from "../lib/api";
  import { copyTextToClipboard } from "../lib/stores/terminal";
  import { t } from "../lib/i18n";

  let {
    trace,
    traces = [],
    onSelectTrace,
    onClose
  }: {
    trace: PromptTrace | null;
    traces?: PromptTrace[];
    onSelectTrace?: (tr: PromptTrace) => void;
    onClose: () => void;
  } = $props();

  let activeTab = $state<"prompt" | "response">("prompt");
  let copied = $state(false);

  async function handleCopy(text: string) {
    if (!text) return;
    const ok = await copyTextToClipboard(text);
    if (ok) {
      copied = true;
      setTimeout(() => (copied = false), 2000);
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      onClose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if trace}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="modal-backdrop" onclick={onClose} role="presentation">
    <div class="modal-container" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <!-- Modal Header -->
      <div class="modal-header">
        <div class="header-left">
          <div class="icon-wrap">
            <Cpu size={18} class="accent-icon" />
          </div>
          <div>
            <div class="title-row">
              <span class="modal-title">External AI Prompt Inspector</span>
              <span class="status-pill mono status-{trace.status}">
                {trace.status.toUpperCase()}
              </span>
              <span class="stage-pill mono">{trace.stage}</span>
            </div>
            <div class="model-meta mono">
              <span class="provider">{trace.provider}</span>
              <span class="sep">/</span>
              <span class="model-name">{trace.model}</span>
              {#if trace.request_id}
                <span class="req-id">({trace.request_id})</span>
              {/if}
            </div>
          </div>
        </div>
        <button type="button" class="close-btn" onclick={onClose} title={$t("Close (Esc)", "ปิด (Esc)")}>
          <X size={18} />
        </button>
      </div>

      <!-- Stats Bar -->
      {#if traces.length > 1}
        <div class="trace-steps-bar mono">
          <span class="trace-steps-lbl">{$t(`Outgoing Prompts (${traces.length}):`, `ลำดับ Prompt ที่ส่งออก (${traces.length} รายการ):`)}</span>
          <div class="trace-steps-list">
            {#each traces as item, idx (item.request_id || idx)}
              <button
                type="button"
                class="trace-step-chip"
                class:active={item.request_id === trace.request_id}
                class:failed={item.status === "failed"}
                onclick={() => onSelectTrace && onSelectTrace(item)}
              >
                <span class="step-idx">#{idx + 1}</span>
                <span class="step-stage">{item.stage}</span>
                <span class="step-model">({item.model})</span>
              </button>
            {/each}
          </div>
        </div>
      {/if}
      <div class="stats-bar mono">
        <div class="stat-item">
          <FileCode size={13} />
          <span>Prompt: <strong>{trace.prompt_bytes ? `${(trace.prompt_bytes / 1024).toFixed(1)} KB` : `${trace.prompt?.length || 0} chars`}</strong></span>
        </div>
        {#if trace.duration_ms}
          <div class="stat-item">
            <Clock size={13} />
            <span>Latency: <strong>{(trace.duration_ms / 1000).toFixed(2)}s</strong></span>
          </div>
        {/if}
        {#if trace.response_bytes}
          <div class="stat-item">
            <Terminal size={13} />
            <span>Response: <strong>{(trace.response_bytes / 1024).toFixed(1)} KB</strong></span>
          </div>
        {/if}
      </div>

      <!-- Tab Buttons & Actions -->
      <div class="tab-bar">
        <div class="tabs">
          <button
            type="button"
            class="tab-btn mono"
            class:active={activeTab === "prompt"}
            onclick={() => (activeTab = "prompt")}
          >
            Outgoing Prompt ({trace.prompt?.length || 0}B)
          </button>
          <button
            type="button"
            class="tab-btn mono"
            class:active={activeTab === "response"}
            onclick={() => (activeTab = "response")}
          >
            {trace.error ? "Error / Raw Response" : "AI Response"}
          </button>
        </div>
        <div class="actions">
          <button
            type="button"
            class="copy-btn mono"
            onclick={() => handleCopy(activeTab === "prompt" ? trace.prompt : (trace.response || trace.error || ""))}
          >
            {#if copied}
              <Check size={13} class="text-green" />
              <span>{$t("Copied", "คัดลอกแล้ว")}</span>
            {:else}
              <Copy size={13} />
              <span>{$t(`Copy ${activeTab === "prompt" ? "Prompt" : "Response"}`, `คัดลอก ${activeTab === "prompt" ? "Prompt" : "Response"}`)}</span>
            {/if}
          </button>
        </div>
      </div>

      <!-- Content Area -->
      <div class="content-body">
        {#if activeTab === "prompt"}
          <pre class="code-view mono">{trace.prompt || $t("(No prompt data)", "(ไม่มีข้อมูล Prompt)")}</pre>
        {:else}
          {#if trace.error}
            <div class="error-banner">
              <AlertTriangle size={15} />
              <span>{trace.error}</span>
            </div>
          {/if}
          <pre class="code-view mono">{trace.response || trace.error || $t("(No response received yet)", "(ยังไม่มีคำตอบส่งกลับมา)")}</pre>
        {/if}
      </div>

      <!-- Modal Footer -->
      <div class="modal-footer">
        <span class="footer-tip mono">{$t(`💡 Dispatched directly to AI model via ${trace.provider} CLI / API for full transparency`, `💡 ข้อมูลนี้ส่งตรงไปยังโมเดล AI ผ่าน ${trace.provider} CLI / API เพื่อความโปร่งใสสูงสุด`)}</span>
        <button type="button" class="btn-primary" onclick={onClose}>{$t("Close", "ปิดหน้าต่าง")}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 9999;
    background: rgba(0, 0, 0, 0.72);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    animation: fadeIn 0.15s ease-out;
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  .modal-container {
    width: 100%;
    max-width: 960px;
    height: 84vh;
    max-height: 840px;
    background: #12151c;
    border: 1px solid rgba(255, 255, 255, 0.12);
    border-radius: 14px;
    box-shadow: 0 24px 60px rgba(0, 0, 0, 0.65), 0 0 0 1px rgba(139, 92, 246, 0.2);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px;
    background: #161a23;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .icon-wrap {
    width: 38px;
    height: 38px;
    border-radius: 10px;
    background: rgba(139, 92, 246, 0.15);
    border: 1px solid rgba(139, 92, 246, 0.35);
    display: flex;
    align-items: center;
    justify-content: center;
    color: #a78bfa;
  }

  .title-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .modal-title {
    font-size: 15px;
    font-weight: 700;
    color: #f3f4f6;
  }

  .status-pill {
    font-size: 10px;
    font-weight: 700;
    padding: 2px 7px;
    border-radius: 4px;
    letter-spacing: 0.5px;
  }

  .status-running {
    background: rgba(34, 197, 94, 0.2);
    color: #4ade80;
    border: 1px solid rgba(34, 197, 94, 0.4);
    animation: pulse 1.5s infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.6; }
  }

  .status-completed {
    background: rgba(34, 197, 94, 0.15);
    color: #86efac;
    border: 1px solid rgba(34, 197, 94, 0.3);
  }

  .status-failed, .status-fallback {
    background: rgba(239, 68, 68, 0.2);
    color: #fca5a5;
    border: 1px solid rgba(239, 68, 68, 0.4);
  }

  .stage-pill {
    font-size: 10.5px;
    color: #94a3b8;
    background: rgba(255, 255, 255, 0.06);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .model-meta {
    font-size: 11.5px;
    color: #94a3b8;
    margin-top: 3px;
    display: flex;
    align-items: center;
    gap: 5px;
  }

  .provider {
    color: #c084fc;
    font-weight: 600;
  }

  .model-name {
    color: #e2e8f0;
  }

  .req-id {
    color: #64748b;
    font-size: 10.5px;
  }

  .close-btn {
    background: transparent;
    border: none;
    color: #94a3b8;
    cursor: pointer;
    padding: 6px;
    border-radius: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s ease;
  }

  .close-btn:hover {
    color: #f1f5f9;
    background: rgba(255, 255, 255, 0.08);
  }

  .stats-bar {
    display: flex;
    align-items: center;
    gap: 20px;
    padding: 8px 20px;
    background: #0f1218;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    font-size: 11px;
    color: #94a3b8;
  }

  .trace-steps-bar {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 20px;
    background: #111520;
    border-bottom: 1px solid rgba(255, 255, 255, 0.07);
    overflow-x: auto;
  }

  .trace-steps-lbl {
    font-size: 10.5px;
    color: #94a3b8;
    white-space: nowrap;
  }

  .trace-steps-list {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: nowrap;
  }

  .trace-step-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 4px 9px;
    border-radius: 6px;
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    color: #cbd5e1;
    font-size: 11px;
    cursor: pointer;
    white-space: nowrap;
  }

  .trace-step-chip:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .trace-step-chip.active {
    background: rgba(139, 92, 246, 0.25);
    border-color: rgba(139, 92, 246, 0.6);
    color: #fff;
    font-weight: 600;
  }

  .trace-step-chip.failed {
    border-color: rgba(239, 68, 68, 0.45);
    color: #fca5a5;
  }

  .step-idx {
    color: #a78bfa;
    font-weight: 700;
  }

  .step-model {
    color: #94a3b8;
    font-size: 10px;
  }

  .stat-item {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .stat-item strong {
    color: #e2e8f0;
  }

  .tab-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 16px;
    background: #141720;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  }

  .tabs {
    display: flex;
    gap: 6px;
  }

  .tab-btn {
    background: transparent;
    border: 1px solid transparent;
    color: #94a3b8;
    padding: 6px 12px;
    border-radius: 6px;
    font-size: 12px;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .tab-btn:hover {
    color: #f1f5f9;
    background: rgba(255, 255, 255, 0.05);
  }

  .tab-btn.active {
    color: #f3f4f6;
    background: rgba(139, 92, 246, 0.2);
    border-color: rgba(139, 92, 246, 0.45);
    font-weight: 600;
  }

  .copy-btn {
    background: rgba(255, 255, 255, 0.06);
    border: 1px solid rgba(255, 255, 255, 0.1);
    color: #cbd5e1;
    font-size: 11.5px;
    padding: 5px 12px;
    border-radius: 6px;
    display: flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .copy-btn:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #fff;
  }

  .text-green {
    color: #4ade80;
  }

  .content-body {
    flex: 1;
    overflow-y: auto;
    padding: 16px 20px;
    background: #0d1016;
  }

  .error-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.35);
    border-radius: 8px;
    color: #fca5a5;
    font-size: 12.5px;
    margin-bottom: 12px;
  }

  .code-view {
    margin: 0;
    font-size: 12px;
    line-height: 1.6;
    color: #e2e8f0;
    white-space: pre-wrap;
    word-break: break-word;
    user-select: text;
  }

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 20px;
    background: #161a23;
    border-top: 1px solid rgba(255, 255, 255, 0.08);
  }

  .footer-tip {
    font-size: 11px;
    color: #64748b;
  }

  .btn-primary {
    background: #8b5cf6;
    border: none;
    color: #fff;
    padding: 6px 16px;
    font-size: 12.5px;
    font-weight: 600;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .btn-primary:hover {
    background: #7c3aed;
  }
</style>