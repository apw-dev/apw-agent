<script lang="ts">
  import { mentionCandidates, mentionSelectedIndex, type MentionCandidate } from "../lib/stores/search";

  let { onSelect }: { onSelect: (cand: MentionCandidate) => void } = $props();
</script>

{#if $mentionCandidates.length > 0}
  <div class="mention-menu" id="mention-menu" role="listbox" aria-label="ตัวเลือกคำสั่งและไฟล์">
    {#each $mentionCandidates as cand, idx (cand.display + idx)}
      <button
        type="button"
        role="option"
        aria-selected={idx === $mentionSelectedIndex}
        class="item"
        class:active={idx === $mentionSelectedIndex}
        onmousedown={(e) => {
          e.preventDefault();
          onSelect(cand);
        }}
        data-testid="mention-candidate"
      >
        <span class="disp mono">{cand.display}</span>
        <span class="meta">{cand.meta}</span>
      </button>
    {/each}
  </div>
{/if}

<style>
  .mention-menu {
    position: absolute;
    bottom: calc(100% + 6px);
    left: 0;
    right: 0;
    background: var(--panel);
    border: 1px solid var(--line-strong);
    border-radius: 10px;
    box-shadow: var(--shadow-md);
    padding: 4px;
    z-index: 35;
    max-height: 240px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 6px 8px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--text);
    text-align: left;
    font-size: 12px;
  }
  .item:hover,
  .item.active {
    background: var(--accent-soft);
  }
  .disp {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
  }
  .meta {
    font-size: 10.5px;
    color: var(--faint);
    flex: none;
  }
</style>
