<script lang="ts">
  import { parseMarkdownBlocks, type InlineToken } from "../lib/markdown";
  import { fetchWorkspaceFile } from "../lib/stores/workspace";
  import { rightCollapsed, rightPanelView } from "../lib/stores/settings";
  import { openExternalURL } from "../lib/shell";

  let { text = "", taskId }: { text: string; taskId?: string } = $props();

  const blocks = $derived(parseMarkdownBlocks(text));

  async function onLinkClick(tok: Extract<InlineToken, { kind: "link" }>) {
    if (tok.filePath) {
      rightCollapsed.set(false);
      const res = await fetchWorkspaceFile(tok.filePath, taskId);
      if (res.ok) {
        rightPanelView.set("file");
        return;
      }
      const wsRes = await fetchWorkspaceFile(tok.filePath);
      if (wsRes.ok) {
        rightPanelView.set("file");
      }
      return;
    }
    if (tok.href.startsWith("http://") || tok.href.startsWith("https://")) {
      openExternalURL(tok.href);
    }
  }
</script>

{#snippet renderInline(tokens: InlineToken[])}
  {#each tokens as tok}
    {#if tok.kind === "text"}
      {tok.text}
    {:else if tok.kind === "bold"}
      <strong>{tok.text}</strong>
    {:else if tok.kind === "code"}
      <code class="md-inline-code mono">{tok.text}</code>
    {:else if tok.kind === "link"}
      {#if tok.filePath || tok.href.startsWith("http://") || tok.href.startsWith("https://")}
        <button
          type="button"
          class="md-link mono"
          data-file-path={tok.filePath || undefined}
          onclick={() => onLinkClick(tok)}
        >
          {tok.label}
        </button>
      {:else}
        <code class="md-inline-code mono">{tok.label}</code>
      {/if}
    {/if}
  {/each}
{/snippet}

<div class="md-body selectable">
  {#each blocks as block}
    {#if block.kind === "heading"}
      {#if block.level === 1}
        <h3 class="md-h1">{@render renderInline(block.tokens)}</h3>
      {:else if block.level === 2}
        <h4 class="md-h2">{@render renderInline(block.tokens)}</h4>
      {:else}
        <h5 class="md-h3">{@render renderInline(block.tokens)}</h5>
      {/if}
    {:else if block.kind === "codeblock"}
      <div class="md-codeblock">
        {#if block.lang}
          <div class="md-code-lang mono">{block.lang}</div>
        {/if}
        <pre class="mono"><code>{block.code}</code></pre>
      </div>
    {:else if block.kind === "list"}
      {#if block.ordered}
        <ol class="md-list">
          {#each block.items as item}
            <li>{@render renderInline(item)}</li>
          {/each}
        </ol>
      {:else}
        <ul class="md-list">
          {#each block.items as item}
            <li>{@render renderInline(item)}</li>
          {/each}
        </ul>
      {/if}
    {:else if block.kind === "acceptance"}
      <div class="md-acceptance" class:pass={block.passed} class:fail={!block.passed}>
        <span class="acc-badge mono">{block.passed ? "✓ PASS" : "✗ FAIL"}</span>
        <span class="acc-text">{@render renderInline(block.tokens)}</span>
      </div>
    {:else if block.kind === "hr"}
      <hr class="md-hr" />
    {:else}
      <p class="md-p">{@render renderInline(block.tokens)}</p>
    {/if}
  {/each}
</div>

<style>
  .md-body {
    display: flex;
    flex-direction: column;
    gap: 8px;
    font-size: 12.5px;
    line-height: 1.65;
    color: var(--text);
    word-break: break-word;
  }
  .md-h1,
  .md-h2,
  .md-h3 {
    margin: 4px 0 0;
    font-weight: 600;
    color: var(--text);
    line-height: 1.45;
  }
  .md-h1 {
    font-size: 14px;
  }
  .md-h2 {
    font-size: 13.5px;
  }
  .md-h3 {
    font-size: 12.5px;
    color: var(--muted);
  }
  .md-p {
    margin: 0;
  }
  .md-list {
    margin: 0;
    padding-left: 18px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .md-inline-code {
    padding: 1px 5px;
    border-radius: 4px;
    background: var(--panel-subtle);
    border: 1px solid var(--line);
    font-size: 11.5px;
    color: var(--text);
  }
  .md-link {
    border: none;
    background: var(--accent-soft);
    color: var(--accent);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11.5px;
    font-weight: 500;
    text-decoration: underline;
  }
  .md-link:hover {
    background: var(--hover);
  }
  .md-codeblock {
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--panel-subtle);
    overflow: hidden;
  }
  .md-code-lang {
    padding: 3px 8px;
    font-size: 10.5px;
    color: var(--faint);
    border-bottom: 1px solid var(--line);
    background: var(--panel);
  }
  .md-codeblock pre {
    margin: 0;
    padding: 8px 10px;
    overflow-x: auto;
    font-size: 11.5px;
    line-height: 1.55;
  }
  .md-acceptance {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 5px 8px;
    border-radius: 6px;
    font-size: 12px;
    border: 1px solid var(--line);
    background: var(--panel);
  }
  .md-acceptance.pass .acc-badge {
    color: var(--ok);
    background: var(--ok-soft);
  }
  .md-acceptance.fail .acc-badge {
    color: var(--err);
    background: var(--err-soft);
  }
  .acc-badge {
    font-size: 10.5px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 4px;
    flex: none;
  }
  .acc-text {
    flex: 1;
    min-width: 0;
  }
  .md-hr {
    border: none;
    border-top: 1px solid var(--line);
    margin: 4px 0;
  }
</style>
