<script lang="ts">
  import { ArrowLeft, FileCode2 } from "lucide-svelte";
  import {
    parseUnifiedDiff,
    tokenizeCodeLines,
    MAX_HIGHLIGHT_LINES,
    type HighlightToken
  } from "../lib/diff";
  import type { TaskJob } from "../lib/api";
  import { resolvedTheme, selectedDiffFile, rightPanelView } from "../lib/stores/settings";
  import { fetchWorkspaceFile } from "../lib/stores/workspace";

  let { job }: { job: TaskJob } = $props();

  let tokens = $state<HighlightToken[][] | null>(null);

  const fallbackFiles = $derived.by(() => {
    const mod = job.report?.modified_files || [];
    if (mod.length) return mod;
    return job.report?.plan?.affected_files || [];
  });
  const fileDiffs = $derived(parseUnifiedDiff(job.report?.diff || "", fallbackFiles));
  const currentFile = $derived(
    $selectedDiffFile && fileDiffs.some((f) => f.file === $selectedDiffFile)
      ? $selectedDiffFile
      : fileDiffs[0]?.file || ""
  );
  const currentDiff = $derived(fileDiffs.find((f) => f.file === currentFile) || null);
  const exceedsHighlightLimit = $derived(
    Boolean(currentDiff && currentDiff.lines.length > MAX_HIGHLIGHT_LINES)
  );

  $effect(() => {
    const diff = currentDiff;
    const isDark = $resolvedTheme === "dark";
    if (!diff || diff.lines.length === 0 || diff.lines.length > MAX_HIGHLIGHT_LINES) {
      tokens = null;
      return;
    }
    let cancelled = false;
    const codeLines = diff.lines.map((l) => (l.kind === "hunk" ? "" : l.text));
    void tokenizeCodeLines(codeLines, diff.file, isDark).then((res) => {
      if (!cancelled) {
        tokens = res;
      }
    });
    return () => {
      cancelled = true;
    };
  });

  async function onOpenFullFile() {
    if (!currentFile) return;
    const res = await fetchWorkspaceFile(currentFile, job.id);
    if (res.ok) {
      rightPanelView.set("file");
    } else {
      const wsRes = await fetchWorkspaceFile(currentFile);
      if (wsRes.ok) rightPanelView.set("file");
    }
  }
</script>

<div class="diff-view" data-testid="diff-view">
  <div class="sub-hdr">
    <button
      type="button"
      class="back-btn"
      onclick={() => rightPanelView.set("cards")}
      data-testid="diff-back-btn"
    >
      <ArrowLeft size={13} />
      <span>กลับ</span>
    </button>

    <select
      class="file-select mono"
      aria-label="เลือกไฟล์ Diff"
      value={currentFile}
      onchange={(e) => selectedDiffFile.set((e.currentTarget as HTMLSelectElement).value)}
      data-testid="diff-file-select"
    >
      {#each fileDiffs as fd (fd.file)}
        <option value={fd.file}>{fd.file} (+{fd.added} −{fd.deleted})</option>
      {/each}
    </select>

    {#if currentFile}
      <button type="button" class="full-btn" onclick={onOpenFullFile} title="ดูไฟล์เต็ม">
        <FileCode2 size={13} />
        <span>ไฟล์เต็ม</span>
      </button>
    {/if}
  </div>

  {#if exceedsHighlightLimit}
    <div class="plain-banner" data-testid="plain-fallback-banner">
      Diff มีขนาดเกิน {MAX_HIGHLIGHT_LINES} บรรทัด — แสดงผลแบบข้อความธรรมดาเพื่อความรวดเร็ว
    </div>
  {/if}

  <div class="diff-scroll mono selectable" data-testid="diff-lines">
    {#if !currentDiff || currentDiff.lines.length === 0}
      <div class="empty">ไม่มีบรรทัด Diff สำหรับไฟล์นี้</div>
    {:else}
      {#each currentDiff.lines as ln, idx (idx)}
        {#if ln.kind === "hunk"}
          <div class="hunk">{ln.text}</div>
        {:else}
          <div class="ln" class:add={ln.kind === "add"} class:del={ln.kind === "del"}>
            <span class="g">{ln.newLine ?? ln.oldLine ?? ""}</span>
            <span class="m">{ln.kind === "add" ? "+" : ln.kind === "del" ? "−" : " "}</span>
            <span class="code">
              {#if tokens && tokens[idx] && tokens[idx].length > 0}
                {#each tokens[idx] as tok, ti (ti)}
                  <span style={tok.color ? `color:${tok.color}` : undefined}>{tok.content}</span>
                {/each}
              {:else}
                {ln.text}
              {/if}
            </span>
          </div>
        {/if}
      {/each}
    {/if}
  </div>
</div>

<style>
  .diff-view {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--panel);
  }
  .sub-hdr {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    border-bottom: 1px solid var(--line);
    background: var(--bg);
  }
  .back-btn,
  .full-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 11.5px;
    flex: none;
  }
  .back-btn:hover,
  .full-btn:hover {
    background: var(--hover);
  }
  .file-select {
    flex: 1;
    min-width: 0;
    padding: 3px 6px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 11px;
  }
  .plain-banner {
    padding: 5px 10px;
    background: var(--warn-soft);
    color: var(--warn);
    font-size: 11px;
    border-bottom: 1px solid var(--line);
  }
  .diff-scroll {
    flex: 1;
    overflow: auto;
    font-size: 11px;
    line-height: 1.65;
  }
  .empty {
    padding: 16px;
    color: var(--faint);
  }
  .hunk {
    color: var(--faint);
    background: var(--panel-subtle);
    padding: 2px 8px;
    font-size: 10.5px;
  }
  .ln {
    display: flex;
    white-space: pre;
  }
  .ln .g {
    width: 34px;
    text-align: right;
    padding-right: 6px;
    color: var(--faint);
    flex: none;
    user-select: none;
  }
  .ln .m {
    width: 14px;
    flex: none;
    color: var(--faint);
    user-select: none;
  }
  .ln .code {
    padding-right: 12px;
  }
  .ln.add {
    background: var(--add);
  }
  .ln.add .g {
    background: var(--addg);
  }
  .ln.del {
    background: var(--del);
  }
  .ln.del .g {
    background: var(--delg);
  }
</style>
