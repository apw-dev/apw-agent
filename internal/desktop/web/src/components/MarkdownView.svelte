<script lang="ts">
  import { Play, Copy, Check, ExternalLink } from "lucide-svelte";
  import { parseMarkdownBlocks, type InlineToken } from "../lib/markdown";
  import { fetchWorkspaceFile } from "../lib/stores/workspace";
  import { rightCollapsed, rightPanelView } from "../lib/stores/settings";
  import { openExternalURL } from "../lib/shell";
  import {
    isRunnableCommandSnippet,
    runCommandInAPW,
    openCommandInExternalTerminal,
    copyTextToClipboard
  } from "../lib/stores/terminal";

  let { text = "", taskId }: { text: string; taskId?: string } = $props();
  let copiedKey = $state("");

  const blocks = $derived(parseMarkdownBlocks(text));

  async function flashCopy(key: string, value: string) {
    const ok = await copyTextToClipboard(value);
    if (ok) {
      copiedKey = key;
      setTimeout(() => {
        if (copiedKey === key) copiedKey = "";
      }, 1500);
    }
  }

  async function openWorkspacePath(filePath: string) {
    rightCollapsed.set(false);
    const res = await fetchWorkspaceFile(filePath, taskId);
    if (res.ok) {
      rightPanelView.set("file");
      return;
    }
    const wsRes = await fetchWorkspaceFile(filePath);
    if (wsRes.ok) {
      rightPanelView.set("file");
    }
  }

  async function onLinkClick(tok: Extract<InlineToken, { kind: "link" }>) {
    if (tok.filePath) {
      await openWorkspacePath(tok.filePath);
      return;
    }
    if (tok.href.startsWith("http://") || tok.href.startsWith("https://")) {
      openExternalURL(tok.href);
    }
  }
</script>

{#snippet renderInline(tokens: InlineToken[])}
  {#each tokens as tok, idx}
    {#if tok.kind === "text"}
      {tok.text}
    {:else if tok.kind === "bold"}
      <strong>{tok.text}</strong>
    {:else if tok.kind === "code"}
      {#if isRunnableCommandSnippet(tok.text)}
        <span class="md-inline-cmd-pod mono">
          <code class="md-inline-cmd-txt">{tok.text}</code>
          <button
            type="button"
            class="inline-cmd-btn run"
            onclick={() => runCommandInAPW(tok.text)}
            title={`รันใน APW: ${tok.text}`}
          >
            ▶ รัน
          </button>
          <button
            type="button"
            class="inline-cmd-btn"
            onclick={() => flashCopy(`inline-${idx}-${tok.text}`, tok.text)}
            title={`คัดลอก: ${tok.text}`}
          >
            {copiedKey === `inline-${idx}-${tok.text}` ? "✓" : "คัดลอก"}
          </button>
        </span>
      {:else if /^https?:\/\/(?:localhost|127\.0\.0\.1):\d+/i.test(tok.text.trim())}
        <button
          type="button"
          class="md-link mono"
          onclick={() => openExternalURL(tok.text.trim())}
          title={`เปิด ${tok.text.trim()}`}
        >
          🌐 {tok.text.trim()}
        </button>
      {:else if tok.filePath}
        <button
          type="button"
          class="md-file-chip mono"
          data-file-path={tok.filePath}
          onclick={() => openWorkspacePath(tok.filePath!)}
          title={`เปิดดูไฟล์ ${tok.filePath}`}
        >
          {tok.text}
        </button>
      {:else}
        <code class="md-inline-code mono">{tok.text}</code>
      {/if}
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
  {#each blocks as block, bIdx}
    {#if block.kind === "heading"}
      {#if block.level === 1}
        <h3 class="md-h1">{@render renderInline(block.tokens)}</h3>
      {:else if block.level === 2}
        <h4 class="md-h2">{@render renderInline(block.tokens)}</h4>
      {:else}
        <h5 class="md-h3">{@render renderInline(block.tokens)}</h5>
      {/if}
    {:else if block.kind === "codeblock"}
      {@const runnable = isRunnableCommandSnippet(block.code, block.lang)}
      <div class="md-codeblock" class:runnable-pod={runnable}>
        <div class="md-code-hdr">
          <span class="md-code-lang mono">{block.lang || (runnable ? "terminal" : "code")}</span>
          <div class="md-code-actions">
            {#if runnable}
              <button
                type="button"
                class="code-act-btn run"
                onclick={() => runCommandInAPW(block.code)}
                title="รันคำสั่งนี้ใน APW Live Console"
              >
                <Play size={11} />
                <span>รันใน APW</span>
              </button>
              <button
                type="button"
                class="code-act-btn"
                onclick={() => openCommandInExternalTerminal(block.code)}
                title="เปิดหน้าต่าง PowerShell แยกเพื่อรันคำสั่งนี้"
              >
                <ExternalLink size={11} />
                <span>เปิด Terminal</span>
              </button>
            {/if}
            <button
              type="button"
              class="code-act-btn"
              onclick={() => flashCopy(`cb-${bIdx}`, block.code)}
              title="คัดลอกโค้ด/คำสั่ง"
            >
              {#if copiedKey === `cb-${bIdx}`}
                <Check size={11} />
                <span>คัดลอกแล้ว</span>
              {:else}
                <Copy size={11} />
                <span>คัดลอก</span>
              {/if}
            </button>
          </div>
        </div>
        <pre class="mono"><code>{block.code}</code></pre>
      </div>
    {:else if block.kind === "table"}
      <div class="md-table-wrap">
        <table class="md-table">
          <thead>
            <tr>
              {#each block.headers as hdr, hIdx}
                <th class:col-num={hIdx === 0 && hdr.length === 1 && hdr[0].kind === "text" && hdr[0].text === "#"}>
                  {@render renderInline(hdr)}
                </th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each block.rows as row}
              <tr>
                {#each row as cell, cIdx}
                  <td class:col-num={cIdx === 0 && cell.length === 1 && cell[0].kind === "text" && /^\d+$/.test(cell[0].text)}>
                    {#if cIdx === 0 && cell.length === 1 && cell[0].kind === "text" && /^\d+$/.test(cell[0].text)}
                      <span class="row-num-badge mono">{cell[0].text}</span>
                    {:else}
                      {@render renderInline(cell)}
                    {/if}
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if block.kind === "blockquote"}
      <blockquote class="md-blockquote">
        {#each block.lines as qLine}
          <p class="md-p">{@render renderInline(qLine)}</p>
        {/each}
      </blockquote>
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
    margin-top: 8px;
    padding-bottom: 4px;
    border-bottom: 1px solid var(--line);
  }
  .md-h3 {
    font-size: 12.5px;
    color: var(--muted);
  }
  .md-p {
    margin: 0;
  }
  .md-table-wrap {
    width: 100%;
    overflow-x: auto;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--panel);
    margin: 4px 0;
  }
  .md-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 12px;
    line-height: 1.55;
  }
  .md-table th {
    text-align: left;
    padding: 7px 10px;
    font-weight: 600;
    color: var(--muted);
    background: var(--panel-subtle);
    border-bottom: 1px solid var(--line);
    white-space: nowrap;
  }
  .md-table td {
    padding: 8px 10px;
    border-bottom: 1px solid var(--line);
    vertical-align: top;
    color: var(--text);
  }
  .md-table tbody tr:last-child td {
    border-bottom: none;
  }
  .md-table tbody tr:nth-child(even) {
    background: color-mix(in srgb, var(--panel-subtle) 50%, transparent);
  }
  .md-table tbody tr:hover {
    background: var(--hover);
  }
  .md-table .col-num {
    width: 34px;
    text-align: center;
  }
  .row-num-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 20px;
    padding: 0 5px;
    border-radius: 5px;
    background: var(--panel-subtle);
    border: 1px solid var(--line);
    font-size: 11px;
    font-weight: 600;
    color: var(--muted);
  }
  .md-blockquote {
    margin: 2px 0;
    padding: 6px 10px;
    border-left: 3px solid var(--accent);
    background: var(--panel-subtle);
    border-radius: 0 6px 6px 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .md-file-chip {
    display: inline-flex;
    align-items: center;
    padding: 1px 6px;
    border-radius: 4px;
    background: var(--accent-soft);
    border: 1px solid color-mix(in srgb, var(--accent) 30%, var(--line));
    color: var(--accent);
    font-size: 11.5px;
    font-weight: 500;
    cursor: pointer;
  }
  .md-file-chip:hover {
    background: var(--hover);
    text-decoration: underline;
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
  .md-inline-cmd-pod {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 1px 4px 1px 6px;
    border-radius: 6px;
    background: var(--panel-subtle);
    border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line));
    font-size: 11.5px;
    vertical-align: middle;
  }
  .md-inline-cmd-txt {
    color: var(--text);
    font-weight: 500;
  }
  .inline-cmd-btn {
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    padding: 0 5px;
    border-radius: 4px;
    font-size: 10px;
    line-height: 1.5;
  }
  .inline-cmd-btn.run {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 45%, var(--line));
    font-weight: 600;
  }
  .inline-cmd-btn:hover {
    background: var(--hover);
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
    border-radius: 8px;
    background: var(--panel-subtle);
    overflow: hidden;
  }
  .md-codeblock.runnable-pod {
    border-color: color-mix(in srgb, var(--accent) 40%, var(--line));
  }
  .md-code-hdr {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 4px 8px;
    border-bottom: 1px solid var(--line);
    background: var(--panel);
  }
  .md-code-lang {
    font-size: 10.5px;
    font-weight: 600;
    color: var(--faint);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .md-code-actions {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .code-act-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 7px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--muted);
    font-size: 10.5px;
    font-weight: 500;
  }
  .code-act-btn.run {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
  }
  .code-act-btn:hover {
    background: var(--hover);
    color: var(--text);
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
