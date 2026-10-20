<script lang="ts">
  import { ChevronDown, ChevronRight } from "lucide-svelte";
  import {
    workspaceStatus,
    connectionState,
    fetchWorkspaceSymbols,
    fetchWorkspaceGraph
  } from "../../lib/stores/workspace";
  import { collapsedCards, toggleCardCollapsed, rightPanelView } from "../../lib/stores/settings";

  const collapsed = $derived(Boolean($collapsedCards["workspace"]));
</script>

<section class="card" data-testid="workspace-card">
  <button type="button" class="card-hdr" onclick={() => toggleCardCollapsed("workspace")}>
    <span class="title">
      {#if collapsed}
        <ChevronRight size={13} />
      {:else}
        <ChevronDown size={13} />
      {/if}
      <span>Workspace</span>
    </span>
    <span class="stack">{$workspaceStatus?.stack || "Project"}</span>
  </button>

  {#if !collapsed}
    <div class="body">
      <div class="row">
        <span>Sandbox</span>
        <b class="mono">{$workspaceStatus?.sandbox_mode || "off"}</b>
      </div>
      <div class="row">
        <span>Trust</span>
        <b class={$workspaceStatus?.trusted ? "ok" : "warn"}>
          {$workspaceStatus?.trusted ? "trusted" : "untrusted"}
        </b>
      </div>
      <div class="row">
        <span>apwd</span>
        <b class={$connectionState === "connected" ? "ok" : "warn"}>
          ● {$connectionState === "connected" ? "connected" : "connecting…"}
        </b>
      </div>
      <div class="tools-row">
        <button
          type="button"
          class="mini-btn"
          onclick={async () => {
            await fetchWorkspaceSymbols();
            rightPanelView.set("symbols");
          }}
          data-testid="open-symbols-btn"
        >
          /symbols
        </button>
        <button
          type="button"
          class="mini-btn"
          onclick={async () => {
            await fetchWorkspaceGraph();
            rightPanelView.set("graph");
          }}
          data-testid="open-graph-btn"
        >
          /graph
        </button>
      </div>
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
  .stack {
    font-weight: 400;
    font-size: 11px;
    color: var(--faint);
  }
  .body {
    margin-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
  }
  .row {
    display: flex;
    justify-content: space-between;
    color: var(--muted);
  }
  .row b {
    font-weight: 500;
    color: var(--text);
  }
  .row b.ok {
    color: var(--ok);
  }
  .row b.warn {
    color: var(--warn);
  }
  .tools-row {
    display: flex;
    gap: 6px;
    margin-top: 4px;
  }
  .mini-btn {
    flex: 1;
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--muted);
    font-family: "Cascadia Code", Consolas, monospace;
    font-size: 11px;
  }
  .mini-btn:hover {
    color: var(--text);
    background: var(--hover);
  }
</style>
