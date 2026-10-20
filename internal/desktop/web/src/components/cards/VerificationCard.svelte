<script lang="ts">
  import { ChevronDown, ChevronRight } from "lucide-svelte";
  import type { TaskJob } from "../../lib/api";
  import { collapsedCards, toggleCardCollapsed } from "../../lib/stores/settings";

  let { job }: { job: TaskJob } = $props();

  let expandedLogs = $state<Record<string, boolean>>({});
  const collapsed = $derived(Boolean($collapsedCards["verification"]));

  interface VerificationItem {
    id: string;
    label: string;
    passed: boolean;
    log?: string;
  }

  const items = $derived.by<VerificationItem[]>(() => {
    const out: VerificationItem[] = [];
    const attempts = job.report?.reflex_attempts || [];
    const lastAttempt = attempts.length > 0 ? attempts[attempts.length - 1] : null;
    if (lastAttempt?.Result) {
      const r = lastAttempt.Result;
      const diagnostics = (r.diagnostics || [])
        .map((d) => (d.file ? `${d.file}${d.line ? ":" + d.line : ""}: ` : "") + d.message)
        .join("\n");
      out.push({
        id: "reflex-" + lastAttempt.Number,
        label: r.failed_stage ? `Reflex (${r.failed_stage})` : `Reflex round #${lastAttempt.Number}`,
        passed: r.passed,
        log: r.raw_output || diagnostics || undefined
      });
    }

    const checks = job.report?.acceptance || [];
    for (let i = 0; i < checks.length; i++) {
      out.push({
        id: "acc-" + i,
        label: checks[i].criterion || `Acceptance #${i + 1}`,
        passed: checks[i].passed,
        log: checks[i].evidence || undefined
      });
    }
    return out;
  });

  const passedCount = $derived(items.filter((i) => i.passed).length);

  function toggleLog(id: string) {
    expandedLogs = { ...expandedLogs, [id]: !expandedLogs[id] };
  }
</script>

<section class="card" id="acceptance-structured" data-testid="verification-card">
  <button type="button" class="card-hdr" onclick={() => toggleCardCollapsed("verification")}>
    <span class="title">
      {#if collapsed}
        <ChevronRight size={13} />
      {:else}
        <ChevronDown size={13} />
      {/if}
      <span>Verification</span>
    </span>
    <span class="summary">
      {#if job.report?.unverified || job.mode === "unverified"}
        UNVERIFIED
      {:else if items.length > 0}
        {passedCount}/{items.length}
      {:else}
        —
      {/if}
    </span>
  </button>

  {#if !collapsed}
    <div class="body">
      {#if job.report?.unverified || job.mode === "unverified"}
        <div class="unverified-note">งานนี้รันในโหมด Unverified (ไม่ได้รัน build หรือ test)</div>
      {:else if items.length === 0}
        <div class="empty">ยังไม่มีผลตรวจสอบสำหรับงานนี้</div>
      {:else}
        {#each items as item (item.id)}
          <div class="v-item">
            <button
              type="button"
              class="v-row"
              onclick={() => item.log && toggleLog(item.id)}
              data-testid="verification-item"
            >
              <span class="v-label mono">{item.label}</span>
              <b class={item.passed ? "ok" : "err"}>{item.passed ? "pass" : "fail"}</b>
            </button>
            {#if (!item.passed || expandedLogs[item.id]) && item.log}
              <pre class="v-log mono selectable" data-testid="verification-log">{item.log}</pre>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</section>

<style>
  .card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 8px 10px;
  }
  .card-hdr {
    width: 100%;
    border: none;
    background: transparent;
    padding: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
  }
  .title {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .summary {
    font-weight: 500;
    font-size: 11px;
    color: var(--faint);
  }
  .body {
    margin-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .empty,
  .unverified-note {
    font-size: 11.5px;
    color: var(--faint);
    padding: 3px 0;
  }
  .unverified-note {
    color: var(--warn);
  }
  .v-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .v-row {
    width: 100%;
    border: none;
    background: transparent;
    padding: 3px 0;
    display: flex;
    justify-content: space-between;
    align-items: center;
    color: var(--muted);
    font-size: 12px;
  }
  .v-label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ok {
    color: var(--ok);
    font-weight: 600;
  }
  .err {
    color: var(--err);
    font-weight: 600;
  }
  .v-log {
    margin: 0;
    padding: 6px 8px;
    border-radius: 6px;
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 10.5px;
    max-height: 150px;
    overflow: auto;
    white-space: pre-wrap;
  }
</style>
