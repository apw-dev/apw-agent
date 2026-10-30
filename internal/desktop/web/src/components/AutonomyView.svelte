<script lang="ts">
  import { ArrowLeft, Play, Pause, Trash2, Plus } from "lucide-svelte";
  import {
    autonomyState,
    loadAutonomy,
    updateGovernorState,
    addGoal,
    deleteGoal,
    addSchedule,
    toggleSchedule,
    deleteSchedule,
    createProceduralSkill
  } from "../lib/stores/workspace";
  import { centerView } from "../lib/stores/settings";
  import { t, tNow } from "../lib/i18n";

  let goalTitle = $state("");
  let goalInstr = $state("");
  let schedName = $state("");
  let schedInstr = $state("");
  let schedInterval = $state(3600);
  let skillSlug = $state("");
  let skillInstr = $state("");
  let errorMsg = $state("");

  $effect(() => {
    void loadAutonomy();
  });

  async function onToggleGovernor() {
    const gov = $autonomyState?.governor;
    if (!gov) return;
    errorMsg = "";
    const res = await updateGovernorState({
      max_tasks_per_hour: gov.max_tasks_per_hour || 6,
      max_consecutive_failures: gov.max_consecutive_failures || 3,
      paused: !gov.paused,
      pause_reason: !gov.paused ? tNow("Paused by user via Desktop UI", "หยุดชั่วคราวโดยผู้ใช้ผ่าน Desktop UI") : ""
    });
    if (!res.ok) errorMsg = res.message;
  }

  async function onCreateGoal() {
    if (!goalTitle.trim() || !goalInstr.trim()) return;
    errorMsg = "";
    const res = await addGoal(goalTitle.trim(), goalInstr.trim());
    if (!res.ok) {
      errorMsg = res.message;
      return;
    }
    goalTitle = "";
    goalInstr = "";
  }

  async function onCreateSchedule() {
    if (!schedName.trim() || !schedInstr.trim()) return;
    errorMsg = "";
    const res = await addSchedule(schedName.trim(), schedInstr.trim(), Number(schedInterval) || 3600);
    if (!res.ok) {
      errorMsg = res.message;
      return;
    }
    schedName = "";
    schedInstr = "";
  }

  async function onCreateSkill() {
    if (!skillSlug.trim() || !skillInstr.trim()) return;
    errorMsg = "";
    const res = await createProceduralSkill({
      slug: skillSlug.trim(),
      title: skillSlug.trim(),
      instruction: skillInstr.trim()
    });
    if (!res.ok) {
      errorMsg =
        res.status === 409
          ? `${tNow("Skill with this slug already exists", "มี Skill ชื่อนี้อยู่แล้ว")} (${res.message})`
          : res.message;
      return;
    }
    skillSlug = "";
    skillInstr = "";
  }
</script>

<main class="autonomy-view" id="autonomy-dialog" data-testid="autonomy-view">
  <div class="scroll">
    <div class="container">
      <div class="top-bar">
        <button type="button" class="back-btn" onclick={() => centerView.set("thread")}>
          <ArrowLeft size={14} />
          <span>{$t("Back to Conversation", "กลับไปที่บทสนทนา")}</span>
        </button>
        <h2>24/7 Autonomy & Procedural Memory</h2>
      </div>

      {#if errorMsg}
        <div class="err-banner" data-testid="autonomy-error">{errorMsg}</div>
      {/if}

      <section class="sec">
        <div class="sec-hdr">
          <div>
            <h3>Safety Governor</h3>
            <p class="sub">
              {$t("Max quota", "โควตาสูงสุด")} {$autonomyState?.governor?.max_tasks_per_hour ?? 6} {$t(
                "tasks/hr · Consecutive failures",
                "งาน/ชม. · ล้มเหลวติดกัน"
              )}
              {$autonomyState?.governor?.consecutive_failures ?? 0}/{$autonomyState?.governor
                ?.max_consecutive_failures ?? 3}
            </p>
          </div>
          <button
            type="button"
            class="btn"
            onclick={onToggleGovernor}
            data-testid="toggle-governor-btn"
          >
            {#if $autonomyState?.governor?.paused}
              <Play size={13} />
              <span>{$t("Resume", "เปิดทำงานต่อ (Resume)")}</span>
            {:else}
              <Pause size={13} />
              <span>{$t("Pause", "หยุดชั่วคราว (Pause)")}</span>
            {/if}
          </button>
        </div>
      </section>

      <section class="sec">
        <h3>Goal Backlog ({$autonomyState?.goals?.length ?? 0})</h3>
        <div class="form-row">
          <input bind:value={goalTitle} placeholder={$t("Goal title...", "ชื่อเป้าหมาย...")} />
          <input
            bind:value={goalInstr}
            placeholder={$t("Instruction for the Agent...", "คำสั่งให้ Agent ดำเนินการ...")}
            class="flex-2"
          />
          <button type="button" class="btn primary" onclick={onCreateGoal} data-testid="add-goal-btn">
            <Plus size={13} />
            <span>{$t("Add Goal", "เพิ่ม Goal")}</span>
          </button>
        </div>
        <div class="list">
          {#if ($autonomyState?.goals || []).length === 0}
            <div class="muted">{$t("No goals configured yet", "ยังไม่มี Goal ที่ตั้งไว้")}</div>
          {/if}
          {#each $autonomyState?.goals || [] as g (g.id)}
            <div class="item">
              <div class="item-main">
                <b>{g.title}</b> <span class="mono tag">[{g.status}]</span>
                <div class="mono muted">{g.instruction}</div>
              </div>
              <button type="button" class="icon-btn" onclick={() => deleteGoal(g.id)}>
                <Trash2 size={13} />
              </button>
            </div>
          {/each}
        </div>
      </section>

      <section class="sec">
        <h3>Recurring Schedules ({$autonomyState?.schedules?.length ?? 0})</h3>
        <div class="form-row">
          <input bind:value={schedName} placeholder={$t("Schedule name...", "ชื่อตารางงาน...")} />
          <input bind:value={schedInstr} placeholder={$t("Instruction...", "คำสั่ง...")} class="flex-2" />
          <input type="number" bind:value={schedInterval} min="60" style="width:90px" />
          <button type="button" class="btn primary" onclick={onCreateSchedule}>
            <Plus size={13} />
            <span>{$t("Add", "เพิ่ม")}</span>
          </button>
        </div>
        <div class="list">
          {#if ($autonomyState?.schedules || []).length === 0}
            <div class="muted">{$t("No recurring schedules yet", "ยังไม่มีตารางรันอัตโนมัติ")}</div>
          {/if}
          {#each $autonomyState?.schedules || [] as s (s.id)}
            <div class="item">
              <div class="item-main">
                <b>{s.name}</b> <span class="muted">({$t("every", "ทุก")} {Math.round(s.interval_sec / 60)} {$t("min", "นาที")})</span>
                <div class="mono muted">{s.instruction}</div>
              </div>
              <button type="button" class="btn" onclick={() => toggleSchedule(s.id, !s.enabled)}>
                {s.enabled ? "ON" : "OFF"}
              </button>
              <button type="button" class="icon-btn" onclick={() => deleteSchedule(s.id)}>
                <Trash2 size={13} />
              </button>
            </div>
          {/each}
        </div>
      </section>

      <section class="sec">
        <h3>Procedural Skills & Episodes ({$autonomyState?.skills?.length ?? 0} skills)</h3>
        <div class="form-row">
          <input
            bind:value={skillSlug}
            placeholder={$t("slug e.g. go-table-test", "slug เช่น go-table-test")}
            class="mono"
          />
          <input
            bind:value={skillInstr}
            placeholder={$t("Skill workflow or guidelines...", "ขั้นตอนหรือแนวทางของ Skill...")}
            class="flex-2"
          />
          <button type="button" class="btn primary" onclick={onCreateSkill} data-testid="add-skill-btn">
            {$t("Create Skill", "สร้าง Skill")}
          </button>
        </div>
        {#if $autonomyState?.skills?.length}
          <div class="skills-chips mono">
            {#each $autonomyState.skills as sk (sk)}
              <span class="sk-chip">.apw/skills/{sk}</span>
            {/each}
          </div>
        {/if}
        <div class="list">
          {#if ($autonomyState?.episodes || []).length === 0}
            <div class="muted">{$t("No autonomous episodes recorded yet", "ยังไม่มีประวัติการทำงานอัตโนมัติ")}</div>
          {/if}
          {#each ($autonomyState?.episodes || []).slice(0, 8) as ep, i (i)}
            <div class="item">
              <div class="item-main selectable">
                <b class="mono">[{ep.status || ep.outcome || "done"}]</b> {ep.instruction}
                <div class="muted">{ep.lesson}</div>
              </div>
            </div>
          {/each}
        </div>
      </section>
    </div>
  </div>
</main>

<style>
  .autonomy-view {
    flex: 1;
    height: 100%;
    background: var(--bg);
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .scroll {
    flex: 1;
    overflow-y: auto;
    padding: 20px;
  }
  .container {
    max-width: 760px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .top-bar {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .top-bar h2 {
    margin: 0;
    font-size: 16px;
  }
  .back-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 10px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
  }
  .err-banner {
    padding: 8px 12px;
    border-radius: 8px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 12px;
  }
  .sec {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .sec h3 {
    margin: 0;
    font-size: 13px;
  }
  .sec-hdr {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .sub,
  .muted {
    margin: 2px 0 0;
    font-size: 11.5px;
    color: var(--muted);
  }
  .form-row {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  input {
    flex: 1;
    min-width: 120px;
    padding: 6px 9px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text);
    font-size: 12px;
  }
  input.flex-2 {
    flex: 2;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 8px 10px;
    border-radius: 8px;
    background: var(--bg);
    border: 1px solid var(--line);
    font-size: 12px;
  }
  .item-main {
    min-width: 0;
    flex: 1;
  }
  .tag {
    font-size: 10.5px;
    color: var(--faint);
  }
  .skills-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .sk-chip {
    padding: 2px 8px;
    border-radius: 5px;
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 11px;
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
  }
  .btn.primary {
    background: var(--text);
    color: var(--panel);
    border-color: var(--text);
  }
  .icon-btn {
    border: none;
    background: transparent;
    color: var(--err);
    padding: 6px;
    min-width: 32px;
    min-height: 32px;
    display: inline-grid;
    place-items: center;
    border-radius: 5px;
  }
</style>
