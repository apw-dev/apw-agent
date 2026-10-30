<script lang="ts">
  import { AlertTriangle } from "lucide-svelte";
  import { conflictBanner } from "../../lib/stores/tasks";
  import { t } from "../../lib/i18n";
</script>

{#if $conflictBanner}
  <section class="card conflict" id="artifact-conflict-box" data-testid="conflict-card">
    <div class="hdr">
      <AlertTriangle size={14} />
      <span>{$t("Workspace Conflict / Drift Detected", "พบข้อขัดแย้งของไฟล์ (Workspace Conflict / Drift)")}</span>
    </div>
    <div class="msg selectable">{$conflictBanner.message}</div>
    <div class="hint">
      {$t(
        "Project files were modified after this task started. Please inspect the Diff or re-run the task on the latest file state.",
        "ไฟล์ในโปรเจกต์ถูกแก้ไขหลังจากงานนี้เริ่มต้น กรุณาตรวจสอบ Diff หรือรันงานใหม่บนสถานะไฟล์ล่าสุด"
      )}
    </div>
  </section>
{/if}

<style>
  .card.conflict {
    border: 1px solid var(--err);
    border-radius: 10px;
    background: var(--err-soft);
    padding: 9px 11px;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .hdr {
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 600;
    font-size: 12px;
    color: var(--err);
  }
  .msg {
    font-size: 12px;
    color: var(--text);
    white-space: pre-wrap;
  }
  .hint {
    font-size: 11px;
    color: var(--muted);
  }
</style>
