<script lang="ts">
  import { ChevronDown, ChevronRight, FileCode2 } from "lucide-svelte";
  import type { TaskJob } from "../../lib/api";
  import { parseUnifiedDiff } from "../../lib/diff";
  import {
    collapsedCards,
    toggleCardCollapsed,
    selectedDiffFile,
    rightPanelView
  } from "../../lib/stores/settings";
  import { fetchWorkspaceFile } from "../../lib/stores/workspace";

  let { job }: { job: TaskJob } = $props();

  const collapsed = $derived(Boolean($collapsedCards["changes"]));
  const fallbackFiles = $derived.by(() => {
    const mod = job.report?.modified_files || [];
    if (mod.length) return mod;
    return job.report?.plan?.affected_files || [];
  });
  const fileDiffs = $derived(parseUnifiedDiff(job.report?.diff || "", fallbackFiles));
  const totalAdded = $derived(fileDiffs.reduce((acc, f) => acc + f.added, 0));
  const totalDeleted = $derived(fileDiffs.reduce((acc, f) => acc + f.deleted, 0));
  const activeFile = $derived(
    $selectedDiffFile && fileDiffs.some((f) => f.file === $selectedDiffFile)
      ? $selectedDiffFile
      : fileDiffs[0]?.file || ""
  );
  const previewDiff = $derived(fileDiffs.find((f) => f.file === activeFile) || null);

  function openFullDiff(file: string) {
    selectedDiffFile.set(file);
    rightPanelView.set("diff");
  }

  async function openFullFile(file: string) {
    const res = await fetchWorkspaceFile(file, job.id);
    if (res.ok) {
      rightPanelView.set("file");
    } else {
      const wsRes = await fetchWorkspaceFile(file);
      if (wsRes.ok) rightPanelView.set("file");
    }
  }
</script>

<section class="card" data-testid="changes-card">
  <button type="button" class="card-hdr" onclick={() => toggleCardCollapsed("changes")}>
    <span class="title">
      {#if collapsed}
        <ChevronRight size={13} />
      {:else}
        <ChevronDown size={13} />
      {/if}
      <span>Changes ({fileDiffs.length})</span>
    </span>
    <span class="counts mono">
      <span class="add">+{totalAdded}</span>
      <span class="del">−{totalDeleted}</span>
    </span>
  </button>

  {#if !collapsed}
    <div class="body">
      {#if fileDiffs.length === 0}
        <div class="empty">ไม่มีไฟล์ที่แก้ไขในงานนี้</div>
      {:else}
        {#each fileDiffs as fd (fd.file)}
          <div class="file-block">
            <div class="file-row" class:active={fd.file === activeFile}>
              <button
                type="button"
                class="file-btn mono"
                onclick={() => openFullDiff(fd.file)}
                data-testid="changes-file-btn"
              >
                <span class="fname">{fd.file}</span>
                <span class="fcounts">
                  {#if fd.added > 0}<span class="add">+{fd.added}</span>{/if}
                  {#if fd.deleted > 0}<span class="del">−{fd.deleted}</span>{/if}
                </span>
              </button>
              <button
                type="button"
                class="view-file-btn"
                title="ดูไฟล์เต็ม"
                onclick={() => openFullFile(fd.file)}
                data-testid="view-full-file-btn"
              >
                <FileCode2 size={12} />
              </button>
            </div>

            {#if fd.file === activeFile && previewDiff && previewDiff.lines.length > 0}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div
                class="mini-diff mono selectable"
                onclick={() => openFullDiff(fd.file)}
                data-testid="changes-mini-diff"
              >
                {#each previewDiff.lines.filter((l) => l.kind !== "hunk").slice(0, 6) as ln, i (i)}
                  <div class="ln" class:add-ln={ln.kind === "add"} class:del-ln={ln.kind === "del"}>
                    <span class="g">{ln.newLine ?? ln.oldLine ?? ""}</span>
                    <span class="m">{ln.kind === "add" ? "+" : ln.kind === "del" ? "−" : " "}</span>
                    <span class="code">{ln.text}</span>
                  </div>
                {/each}
              </div>
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
  .counts {
    display: inline-flex;
    gap: 6px;
    font-size: 11px;
    font-weight: 500;
  }
  .add {
    color: var(--addt);
  }
  .del {
    color: var(--delt);
  }
  .body {
    margin-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .empty {
    font-size: 11.5px;
    color: var(--faint);
    padding: 4px 0;
  }
  .file-block {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .file-row {
    display: flex;
    align-items: center;
    gap: 4px;
    border-radius: 5px;
    padding: 2px 4px;
  }
  .file-row.active,
  .file-row:hover {
    background: var(--bg);
  }
  .file-btn {
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    color: var(--text);
    padding: 3px 4px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    font-size: 11.5px;
    text-align: left;
  }
  .fname {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .fcounts {
    display: inline-flex;
    gap: 5px;
    flex: none;
  }
  .view-file-btn {
    border: none;
    background: transparent;
    color: var(--faint);
    padding: 3px;
    border-radius: 4px;
    display: grid;
    place-items: center;
  }
  .view-file-btn:hover {
    color: var(--text);
    background: var(--hover);
  }
  .mini-diff {
    border: 1px solid var(--line);
    border-radius: 6px;
    overflow: hidden;
    font-size: 10.5px;
    line-height: 1.55;
    cursor: pointer;
  }
  .ln {
    display: flex;
    white-space: pre;
    overflow: hidden;
  }
  .ln .g {
    width: 28px;
    text-align: right;
    padding-right: 5px;
    color: var(--faint);
    flex: none;
  }
  .ln .m {
    width: 12px;
    flex: none;
    color: var(--faint);
  }
  .ln .code {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ln.add-ln {
    background: var(--add);
  }
  .ln.add-ln .g {
    background: var(--addg);
  }
  .ln.del-ln {
    background: var(--del);
  }
  .ln.del-ln .g {
    background: var(--delg);
  }
</style>
