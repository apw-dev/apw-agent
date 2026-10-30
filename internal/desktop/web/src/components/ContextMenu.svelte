<script lang="ts">
  import { Copy, FolderOpen } from "lucide-svelte";
  import { contextMenu } from "../lib/stores/settings";
  import { revealInExplorer, inDesktopShell } from "../lib/shell";
  import { t } from "../lib/i18n";

  async function onCopyText() {
    const text = $contextMenu?.copyText || window.getSelection()?.toString() || "";
    if (text && navigator.clipboard) {
      await navigator.clipboard.writeText(text);
    }
    contextMenu.set(null);
  }

  async function onCopyPath() {
    if ($contextMenu?.filePath && navigator.clipboard) {
      await navigator.clipboard.writeText($contextMenu.filePath);
    }
    contextMenu.set(null);
  }

  async function onOpenExplorer() {
    if ($contextMenu?.filePath) {
      await revealInExplorer($contextMenu.filePath);
    }
    contextMenu.set(null);
  }
</script>

{#if $contextMenu}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="ctx-backdrop" onclick={() => contextMenu.set(null)}></div>
  <div
    class="ctx-menu"
    style={`top:${$contextMenu.y}px;left:${$contextMenu.x}px`}
    role="menu"
    data-testid="app-context-menu"
  >
    <button type="button" class="item" onclick={onCopyText}>
      <Copy size={13} />
      <span>{$t("Copy text", "คัดลอกข้อความ (Copy)")}</span>
    </button>
    {#if $contextMenu.filePath}
      <button type="button" class="item" onclick={onCopyPath}>
        <Copy size={13} />
        <span>{$t("Copy path", "คัดลอกพาธไฟล์ (Copy path)")}</span>
      </button>
      {#if inDesktopShell()}
        <button type="button" class="item" onclick={onOpenExplorer}>
          <FolderOpen size={13} />
          <span>{$t("Reveal in Explorer", "เปิดใน Explorer")}</span>
        </button>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .ctx-backdrop {
    position: fixed;
    inset: 0;
    z-index: 80;
  }
  .ctx-menu {
    position: fixed;
    z-index: 90;
    min-width: 180px;
    background: var(--panel);
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    box-shadow: var(--shadow-md);
    padding: 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 9px;
    border-radius: 5px;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 12px;
    text-align: left;
  }
  .item:hover {
    background: var(--hover);
  }
</style>
