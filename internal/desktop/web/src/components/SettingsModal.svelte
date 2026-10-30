<script lang="ts">
  import { X } from "lucide-svelte";
  import {
    settingsModalOpen,
    themeMode,
    updateTheme,
    updateLanguage,
    type ThemeMode
  } from "../lib/stores/settings";
  import { locale, t, tNow, type AppLocale } from "../lib/i18n";
  import {
    modelsState,
    channelsState,
    workspaceStatus,
    loadModels,
    saveModelsConfig,
    loadChannels,
    saveChannelsConfig,
    toggleWorkspaceTrust
  } from "../lib/stores/workspace";

  let activeTab = $state<"general" | "models" | "messaging" | "workspace">("general");
  let architect = $state("");
  let reflex = $state("");
  let architectFallback = $state("");
  let reflexFallback = $state("");
  let approval = $state("ask");
  let sandboxMode = $state("off");

  let telegramEnabled = $state(false);
  let telegramToken = $state("");
  let telegramAllowRaw = $state("");

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
          architectFallback = m.architect_fallbacks?.[0] || "";
          reflexFallback = m.reflex_fallbacks?.[0] || "";
          approval = m.approval || "ask";
          sandboxMode = m.sandbox_mode || "off";
        }
      });
      void loadChannels().then((c) => {
        if (c?.telegram) {
          telegramEnabled = c.telegram.enabled;
          telegramToken = "";
          telegramAllowRaw = (c.telegram.allow_users || []).join(", ");
        }
      });
    }
  });

  function switchTab(tab: "general" | "models" | "messaging" | "workspace") {
    activeTab = tab;
    errorMsg = "";
    savedMsg = "";
  }

  function parseAllowUsersInput(raw: string): { ok: boolean; ids: number[]; error: string } {
    const parts = raw
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    const ids: number[] = [];
    for (const part of parts) {
      if (!/^\d+$/.test(part)) {
        return {
          ok: false,
          ids: [],
          error: tNow(
            `Invalid Telegram User ID: "${part}" (must be a positive integer)`,
            `Telegram User ID ไม่ถูกต้อง: "${part}" (ต้องเป็นตัวเลขจำนวนเต็มบวก)`
          )
        };
      }
      const n = Number(part);
      if (!Number.isSafeInteger(n) || n <= 0) {
        return {
          ok: false,
          ids: [],
          error: tNow(`Invalid Telegram User ID: "${part}"`, `Telegram User ID ไม่ถูกต้อง: "${part}"`)
        };
      }
      if (!ids.includes(n)) {
        ids.push(n);
      }
    }
    return { ok: true, ids, error: "" };
  }

  async function onRefreshModels() {
    if (busy) return;
    busy = true;
    errorMsg = "";
    savedMsg = "";
    const m = await loadModels(true);
    if (m) {
      architect = m.architect;
      reflex = m.reflex;
      architectFallback = m.architect_fallbacks?.[0] || "";
      reflexFallback = m.reflex_fallbacks?.[0] || "";
      approval = m.approval || "ask";
      sandboxMode = m.sandbox_mode || "off";
      savedMsg = tNow(
        `Synced latest models (${m.providers.length} providers)`,
        `ซิงค์รายการโมเดลล่าสุดเรียบร้อยแล้ว (${m.providers.length} รายการ)`
      );
    } else {
      errorMsg = tNow("Failed to sync model list", "ไม่สามารถซิงค์รายการโมเดลได้");
    }
    busy = false;
  }

  async function onSaveModels() {
    if (busy) return;
    busy = true;
    errorMsg = "";
    savedMsg = "";
    const res = await saveModelsConfig({
      architect,
      reflex,
      architect_fallbacks: architectFallback ? [architectFallback] : [],
      reflex_fallbacks: reflexFallback ? [reflexFallback] : [],
      approval,
      sandbox_mode: sandboxMode
    });
    if (!res.ok) {
      errorMsg = res.message;
    } else {
      savedMsg = tNow("Settings saved successfully", "บันทึกการตั้งค่าเรียบร้อยแล้ว");
    }
    busy = false;
  }

  async function onSaveMessaging() {
    if (busy) return;
    errorMsg = "";
    savedMsg = "";
    const parsed = parseAllowUsersInput(telegramAllowRaw);
    if (!parsed.ok) {
      errorMsg = parsed.error;
      return;
    }
    busy = true;
    const res = await saveChannelsConfig({
      telegram: {
        enabled: telegramEnabled,
        token: telegramToken.trim() || undefined,
        allow_users: parsed.ids
      }
    });
    if (!res.ok) {
      errorMsg = res.message;
    } else {
      telegramToken = "";
      telegramAllowRaw = parsed.ids.join(", ");
      savedMsg = tNow("Messaging settings saved successfully", "บันทึกการตั้งค่า Messaging เรียบร้อยแล้ว");
    }
    busy = false;
  }

  async function onClearTelegramToken() {
    if (busy) return;
    busy = true;
    errorMsg = "";
    savedMsg = "";
    const res = await saveChannelsConfig({
      telegram: {
        enabled: false,
        clear_token: true
      }
    });
    if (!res.ok) {
      errorMsg = res.message;
    } else {
      telegramEnabled = false;
      telegramToken = "";
      savedMsg = tNow(
        "Cleared Telegram Bot Token from .apw/auth.json",
        "ลบ Telegram Bot Token ออกจาก .apw/auth.json เรียบร้อยแล้ว"
      );
    }
    busy = false;
  }

  async function onSaveActiveTab() {
    if (activeTab === "messaging") {
      await onSaveMessaging();
    } else {
      await onSaveModels();
    }
  }

  async function onThemeSelect(mode: ThemeMode) {
    await updateTheme(mode);
  }

  function onLanguageSelect(lang: AppLocale) {
    updateLanguage(lang);
  }
</script>

{#if $settingsModalOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="backdrop" onclick={() => settingsModalOpen.set(false)}></div>
  <div class="modal" id="settings-dialog" role="dialog" aria-label={$t("APW Desktop Settings", "ตั้งค่า APW Desktop")}>
    <div class="hdr">
      <h3>{$t("Settings", "การตั้งค่า (Settings)")}</h3>
      <button type="button" class="close-btn" onclick={() => settingsModalOpen.set(false)}>
        <X size={15} />
      </button>
    </div>

    <div class="tabs">
      <button
        type="button"
        class="tab"
        class:active={activeTab === "general"}
        onclick={() => switchTab("general")}
        data-testid="settings-tab-general"
      >
        {$t("General", "ทั่วไป (General)")}
      </button>
      <button
        type="button"
        class="tab"
        class:active={activeTab === "models"}
        onclick={() => switchTab("models")}
        data-testid="settings-tab-models"
      >
        {$t("Models", "โมเดล (Models)")}
      </button>
      <button
        type="button"
        class="tab"
        class:active={activeTab === "messaging"}
        onclick={() => switchTab("messaging")}
        data-testid="settings-tab-messaging"
      >
        {$t("Messaging", "การแจ้งเตือน (Messaging)")}
      </button>
      <button
        type="button"
        class="tab"
        class:active={activeTab === "workspace"}
        onclick={() => switchTab("workspace")}
        data-testid="settings-tab-workspace"
      >
        {$t("Workspace & Sandbox", "โปรเจกต์ & Sandbox")}
      </button>
    </div>

    <div class="content">
      {#if activeTab === "general"}
        <div class="field">
          <div class="lbl">{$t("Interface Language (ภาษา)", "ภาษาของระบบ (Language)")}</div>
          <div class="theme-group">
            {#each [
              { id: "en", label: "English (International)" },
              { id: "th", label: "ภาษาไทย (Thai)" }
            ] as opt (opt.id)}
              <button
                type="button"
                class="theme-btn"
                class:active={$locale === opt.id}
                onclick={() => onLanguageSelect(opt.id as AppLocale)}
                data-testid={`lang-${opt.id}-btn`}
              >
                {opt.label}
              </button>
            {/each}
          </div>
        </div>

        <div class="field">
          <div class="lbl">{$t("Appearance Theme", "ธีมสีหน้าต่าง (Theme)")}</div>
          <div class="theme-group">
            {#each [
              { id: "system", label: $t("System", "ตามระบบ (System)") },
              { id: "light", label: $t("Light", "สว่าง (Light)") },
              { id: "dark", label: $t("Dark", "มืด (Dark)") }
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
          <div class="lbl-row">
            <label for="sel-architect">Brain 2 (Architect / Deep Planner)</label>
            <button
              type="button"
              class="btn"
              onclick={onRefreshModels}
              disabled={busy}
              data-testid="refresh-models-btn"
            >
              {$t("Sync Latest Models (agy / CLI)", "ซิงค์โมเดลล่าสุด (agy / CLI)")}
            </button>
          </div>
          <select id="sel-architect" bind:value={architect} class="mono">
            {#each $modelsState?.providers || [] as p (p.key)}
              <option value={p.key}>{p.key}: {p.model} ({p.type})</option>
            {/each}
          </select>
        </div>

        <div class="field">
          <label for="sel-architect-fallback">
            {$t("Brain 2 Standby / Fallback Layer #1", "Brain 2 Standby / Fallback Layer #1 (โมเดลสำรอง Architect)")}
          </label>
          <select id="sel-architect-fallback" bind:value={architectFallback} class="mono">
            <option value="">{$t("(No fallback model)", "(ไม่ตั้งค่าโมเดลสำรอง)")}</option>
            {#each $modelsState?.providers || [] as p (p.key)}
              <option value={p.key}>{p.key}: {p.model} ({p.type})</option>
            {/each}
          </select>
        </div>

        <div class="field">
          <label for="sel-reflex">Brain 1 (Reflex / Fast Surgeon)</label>
          <select id="sel-reflex" bind:value={reflex} class="mono">
            {#each $modelsState?.providers || [] as p (p.key)}
              <option value={p.key}>{p.key}: {p.model} ({p.type})</option>
            {/each}
          </select>
        </div>

        <div class="field">
          <label for="sel-reflex-fallback">
            {$t("Brain 1 Standby / Fallback Layer #1", "Brain 1 Standby / Fallback Layer #1 (โมเดลสำรอง Reflex)")}
          </label>
          <select id="sel-reflex-fallback" bind:value={reflexFallback} class="mono">
            <option value="">{$t("(No fallback model)", "(ไม่ตั้งค่าโมเดลสำรอง)")}</option>
            {#each $modelsState?.providers || [] as p (p.key)}
              <option value={p.key}>{p.key}: {p.model} ({p.type})</option>
            {/each}
          </select>
        </div>

        <div class="field">
          <label for="sel-approval">{$t("Default Apply Policy (Approval)", "นโยบายการ Apply เริ่มต้น (Approval)")}</label>
          <select id="sel-approval" bind:value={approval}>
            <option value="ask">{$t("ask: Confirm before applying to Workspace", "ask: ถามก่อน Apply ลง Workspace")}</option>
            <option value="auto">{$t("auto: Apply automatically after verification", "auto: Apply อัตโนมัติหลัง Verify ผ่าน")}</option>
          </select>
        </div>
      {:else if activeTab === "messaging"}
        <div class="field">
          <label class="toggle-row" for="chk-telegram-enabled">
            <input
              id="chk-telegram-enabled"
              type="checkbox"
              bind:checked={telegramEnabled}
              data-testid="telegram-enabled-checkbox"
            />
            <span>{$t("Enable Telegram Bot Gateway (apwd)", "เปิดใช้งาน Telegram Bot Gateway (apwd)")}</span>
          </label>
        </div>

        <div class="field">
          <div class="lbl-row">
            <label for="inp-telegram-token">Telegram Bot Token (.apw/auth.json)</label>
            {#if $channelsState?.telegram.token_configured}
              <div class="token-status" data-testid="telegram-token-status">
                <span class="mono">✓ {$channelsState.telegram.token_masked}</span>
                <button
                  type="button"
                  class="link-btn"
                  onclick={onClearTelegramToken}
                  disabled={busy}
                  data-testid="clear-telegram-token-btn"
                >
                  {$t("Clear Token", "ล้าง Token")}
                </button>
              </div>
            {/if}
          </div>
          <input
            id="inp-telegram-token"
            type="password"
            class="mono text-inp"
            placeholder={$channelsState?.telegram.token_configured
              ? $t("Leave blank to keep existing token, or enter a new one", "เว้นว่างไว้เพื่อใช้ Token เดิม หรือกรอกใหม่เพื่อเปลี่ยน")
              : "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"}
            bind:value={telegramToken}
            data-testid="telegram-token-input"
          />
          <div class="hint">
            {$t(
              "Get a Bot Token from @BotFather on Telegram. Stored exclusively in .apw/auth.json (never written to apw.json).",
              "รับ Bot Token จาก @BotFather บน Telegram ระบบจะบันทึกไว้ใน .apw/auth.json เท่านั้น (ไม่เขียนลง apw.json)"
            )}
          </div>
        </div>

        <div class="field">
          <label for="inp-telegram-allow">{$t("Allowed Telegram User IDs (comma-separated)", "Allowed Telegram User IDs (คั่นด้วยลูกน้ำ)")}</label>
          <input
            id="inp-telegram-allow"
            type="text"
            class="mono text-inp"
            placeholder={$t("e.g. 123456789, 987654321", "เช่น 123456789, 987654321")}
            bind:value={telegramAllowRaw}
            data-testid="telegram-allow-input"
          />
          <div class="hint">
            {$t(
              "Specify Telegram User IDs authorized to control APW (at least 1 ID required when enabled).",
              "ระบุหมายเลข Telegram User ID ที่ได้รับอนุญาตให้สั่งงาน APW (ต้องมีอย่างน้อย 1 ID เมื่อเปิดใช้งาน)"
            )}
          </div>
        </div>
      {:else}
        <div class="field">
          <label for="sel-sandbox">{$t("Process Isolation (Sandbox Mode)", "โหมดแยกตัวกระบวนการ (Sandbox Mode)")}</label>
          <select id="sel-sandbox" bind:value={sandboxMode}>
            <option value="off">{$t("off: Standard Git Worktree", "off: รันใน Git Worktree มาตรฐาน")}</option>
            <option value="restricted">restricted: Windows Job Object Restricted</option>
            <option value="docker">docker: Container Isolation</option>
          </select>
        </div>

        <div class="field">
          <div class="lbl">{$t("Workspace Trust", "สถานะความเชื่อถือของโปรเจกต์ (Workspace Trust)")}</div>
          <button
            type="button"
            class="trust-toggle"
            onclick={() => toggleWorkspaceTrust(!$workspaceStatus?.trusted)}
          >
            {$workspaceStatus?.trusted
              ? $t("✓ Trusted: Click to revoke build/test permissions", "✓ Trusted: คลิกเพื่อยกเลิกสิทธิ์รัน build/test")
              : $t("⚠ Untrusted: Click to allow build/test execution", "⚠ Untrusted: คลิกเพื่ออนุญาตรัน build/test")}
          </button>
        </div>
      {/if}

      {#if errorMsg}
        <div class="err-msg" data-testid="settings-error">{errorMsg}</div>
      {/if}
      {#if savedMsg}
        <div class="ok-msg" data-testid="settings-saved">{savedMsg}</div>
      {/if}
    </div>

    <div class="ftr">
      <button type="button" class="btn" onclick={() => settingsModalOpen.set(false)}>{$t("Close", "ปิด")}</button>
      {#if activeTab !== "general"}
        <button
          type="button"
          class="btn primary"
          onclick={onSaveActiveTab}
          disabled={busy}
          data-testid="save-settings-btn"
        >
          {$t("Save", "บันทึก")}
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
    width: 500px;
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
    padding: 6px;
    min-width: 32px;
    min-height: 32px;
    display: inline-grid;
    place-items: center;
    border-radius: 6px;
  }
  .tabs {
    display: flex;
    gap: 4px;
    padding: 8px 16px;
    border-bottom: 1px solid var(--line);
    background: var(--bg);
    flex-wrap: wrap;
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
  .lbl-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }
  label,
  .lbl {
    font-size: 12px;
    font-weight: 500;
    color: var(--muted);
  }
  .toggle-row {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text);
    font-size: 12.5px;
    cursor: pointer;
  }
  select,
  .text-inp,
  .trust-toggle {
    padding: 7px 10px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text);
    font-size: 12.5px;
    text-align: left;
  }
  .token-status {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 11.5px;
    color: var(--ok);
  }
  .link-btn {
    border: none;
    background: transparent;
    color: var(--err);
    font-size: 11.5px;
    padding: 2px 4px;
    cursor: pointer;
    text-decoration: underline;
  }
  .hint {
    font-size: 11.5px;
    color: var(--muted);
    line-height: 1.4;
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
