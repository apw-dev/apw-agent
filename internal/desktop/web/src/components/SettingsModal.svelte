<script lang="ts">
  import { X } from "lucide-svelte";
  import {
    settingsModalOpen,
    themeMode,
    updateTheme,
    type ThemeMode
  } from "../lib/stores/settings";
  import {
    modelsState,
    workspaceStatus,
    loadModels,
    saveModelsConfig,
    toggleWorkspaceTrust
  } from "../lib/stores/workspace";

  let activeTab = $state<"general" | "models" | "workspace">("general");
  let architect = $state("");
  let reflex = $state("");
  let approval = $state("ask");
  let sandboxMode = $state("off");
  let busy = $state(false);
  let errorMsg = $state("");
  let savedMsg = $state("");

  $effect(() => {
    if ($settingsModalOpen) {
      errorMsg = "";
      savedMsg = "";
      void loadModels().then((m) => {
        if (m) {
          architect = m.architect;
          reflex = m.reflex;
          approval = m.approval || "ask";
          sandboxMode = m.sandbox_mode || "off";
        }
      });
    }
  });

  async function onSaveModels() {
    if (busy) return;
    busy = true;
    errorMsg = "";
    savedMsg = "";
    const res = await saveModelsConfig({
      architect,
      reflex,
      approval,
      sandbox_mode: sandboxMode
    });
    if (!res.ok) {
      errorMsg = res.message;
    } else {
      savedMsg = "บันทึกการตั้งค่าเรียบร้อยแล้ว";
    }
    busy = false;
  }

  async function onThemeSelect(mode: ThemeMode) {
    await updateTheme(mode);
  }
</script>

{#if $settingsModalOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="backdrop" onclick={() => settingsModalOpen.set(false)}></div>
  <div class="modal" id="settings-dialog" role="dialog" aria-label="ตั้งค่า APW Desktop">
    <div class="hdr">
      <h3>การตั้งค่า (Settings)</h3>
      <button type="button" class="close-btn" onclick={() => settingsModalOpen.set(false)}>
        <X size={15} />
      </button>
    </div>

    <div class="tabs">
      <button
        type="button"
        class="tab"
        class:active={activeTab === "general"}
        onclick={() => (activeTab = "general")}
      >
        General
      </button>
      <button
        type="button"
        class="tab"
        class:active={activeTab === "models"}
        onclick={() => (activeTab = "models")}
      >
        Models
      </button>
      <button
        type="button"
        class="tab"
        class:active={activeTab === "workspace"}
        onclick={() => (activeTab = "workspace")}
      >
        Workspace & Sandbox
      </button>
    </div>

    <div class="content">
      {#if activeTab === "general"}
        <div class="field">
          <div class="lbl">ธีมสีหน้าต่าง (Theme)</div>
          <div class="theme-group">
            {#each [
              { id: "system", label: "ตามระบบ (System)" },
              { id: "light", label: "สว่าง (Light)" },
              { id: "dark", label: "มืด (Dark)" }
            ] as opt (opt.id)}
              <button
                type="button"
                class="theme-btn"
                class:active={$themeMode === opt.id}
                onclick={() => onThemeSelect(opt.id as ThemeMode)}
                data-testid={`theme-${opt.id}-btn`}
              >
                {opt.label}
              </button>
            {/each}
          </div>
        </div>
      {:else if activeTab === "models"}
        <div class="field">
          <label for="sel-architect">Brain 2 (Architect / Deep Planner)</label>
          <select id="sel-architect" bind:value={architect} class="mono">
            {#each $modelsState?.providers || [] as p (p.key)}
              <option value={p.key}>{p.key} — {p.model} ({p.type})</option>
            {/each}
          </select>
        </div>

        <div class="field">
          <label for="sel-reflex">Brain 1 (Reflex / Fast Surgeon)</label>
          <select id="sel-reflex" bind:value={reflex} class="mono">
            {#each $modelsState?.providers || [] as p (p.key)}
              <option value={p.key}>{p.key} — {p.model} ({p.type})</option>
            {/each}
          </select>
        </div>

        <div class="field">
          <label for="sel-approval">นโยบายการ Apply เริ่มต้น (Approval)</label>
          <select id="sel-approval" bind:value={approval}>
            <option value="ask">ask — ถามก่อน Apply ลง Workspace</option>
            <option value="auto">auto — Apply อัตโนมัติหลัง Verify ผ่าน</option>
          </select>
        </div>
      {:else}
        <div class="field">
          <label for="sel-sandbox">โหมดแยกตัวกระบวนการ (Sandbox Mode)</label>
          <select id="sel-sandbox" bind:value={sandboxMode}>
            <option value="off">off — รันใน Git Worktree มาตรฐาน</option>
            <option value="restricted">restricted — Windows Job Object Restricted</option>
            <option value="docker">docker — Container Isolation</option>
          </select>
        </div>

        <div class="field">
          <div class="lbl">สถานะความเชื่อถือของโปรเจกต์ (Workspace Trust)</div>
          <button
            type="button"
            class="trust-toggle"
            onclick={() => toggleWorkspaceTrust(!$workspaceStatus?.trusted)}
          >
            {$workspaceStatus?.trusted
              ? "✓ Trusted — คลิกเพื่อยกเลิกสิทธิ์รัน build/test"
              : "⚠ Untrusted — คลิกเพื่ออนุญาตรัน build/test"}
          </button>
        </div>
      {/if}

      {#if errorMsg}
        <div class="err-msg" data-testid="settings-error">{errorMsg}</div>
      {/if}
      {#if savedMsg}
        <div class="ok-msg">{savedMsg}</div>
      {/if}
    </div>

    <div class="ftr">
      <button type="button" class="btn" onclick={() => settingsModalOpen.set(false)}>ปิด</button>
      {#if activeTab !== "general"}
        <button
          type="button"
          class="btn primary"
          onclick={onSaveModels}
          disabled={busy}
          data-testid="save-settings-btn"
        >
          บันทึก
        </button>
      {/if}
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.35);
    z-index: 60;
  }
  .modal {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: 480px;
    max-width: calc(100vw - 32px);
    background: var(--panel);
    border: 1px solid var(--line-strong);
    border-radius: 12px;
    box-shadow: var(--shadow-md);
    z-index: 70;
    display: flex;
    flex-direction: column;
  }
  .hdr {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 16px;
    border-bottom: 1px solid var(--line);
  }
  .hdr h3 {
    margin: 0;
    font-size: 14px;
  }
  .close-btn {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 4px;
    border-radius: 6px;
  }
  .tabs {
    display: flex;
    gap: 4px;
    padding: 8px 16px;
    border-bottom: 1px solid var(--line);
    background: var(--bg);
  }
  .tab {
    padding: 5px 10px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 12px;
    font-weight: 500;
  }
  .tab.active {
    background: var(--panel);
    color: var(--text);
    box-shadow: var(--shadow-sm);
  }
  .content {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  label,
  .lbl {
    font-size: 12px;
    font-weight: 500;
    color: var(--muted);
  }
  select,
  .trust-toggle {
    padding: 7px 10px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text);
    font-size: 12.5px;
    text-align: left;
  }
  .theme-group {
    display: flex;
    gap: 8px;
  }
  .theme-btn {
    flex: 1;
    padding: 7px 10px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text);
    font-size: 12px;
  }
  .theme-btn.active {
    border-color: var(--accent);
    background: var(--accent-soft);
    font-weight: 600;
  }
  .err-msg {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 12px;
  }
  .ok-msg {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--ok-soft);
    color: var(--ok);
    font-size: 12px;
  }
  .ftr {
    padding: 10px 16px;
    border-top: 1px solid var(--line);
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  .btn {
    padding: 5px 14px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
    font-weight: 500;
  }
  .btn.primary {
    background: var(--text);
    color: var(--panel);
    border-color: var(--text);
  }
</style>
