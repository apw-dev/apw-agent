<script lang="ts">
  import { onMount } from "svelte";
  import {
    ChevronDown,
    ChevronRight,
    Folder,
    FolderOpen,
    FileCode,
    FileText,
    RefreshCw,
    Search
  } from "lucide-svelte";
  import { parseUnifiedDiff } from "../lib/diff";
  import { selectedTask } from "../lib/stores/tasks";
  import {
    workspaceFiles,
    workspaceStatus,
    activeFileView,
    fetchWorkspaceFiles,
    fetchWorkspaceFile
  } from "../lib/stores/workspace";
  import { rightPanelView } from "../lib/stores/settings";

  let filterText = $state("");
  let loading = $state(false);
  let collapsedDirs = $state<Record<string, boolean>>({});

  interface TreeRow {
    kind: "dir" | "file";
    name: string;
    path: string;
    depth: number;
    size?: number;
    fileCount?: number;
  }

  const diffStatsByFile = $derived.by(() => {
    const m = new Map<string, { added: number; deleted: number }>();
    const job = $selectedTask;
    if (!job?.report?.diff) return m;
    const diffs = parseUnifiedDiff(job.report.diff, job.report.modified_files || []);
    for (const fd of diffs) {
      m.set(fd.file, { added: fd.added, deleted: fd.deleted });
    }
    return m;
  });

  async function refreshFiles() {
    loading = true;
    await fetchWorkspaceFiles($selectedTask?.id);
    loading = false;
  }

  onMount(() => {
    if ($workspaceFiles.length === 0) {
      void refreshFiles();
    }
  });

  function toggleDir(dirPath: string) {
    collapsedDirs = {
      ...collapsedDirs,
      [dirPath]: !isDirCollapsed(dirPath)
    };
  }

  function isDirCollapsed(dirPath: string): boolean {
    if (filterText.trim() !== "") return false;
    if (Object.prototype.hasOwnProperty.call(collapsedDirs, dirPath)) {
      return Boolean(collapsedDirs[dirPath]);
    }
    // Collapse deeper directories by default so the top-level Workspace folders look clean like Screenshot 2.
    return dirPath.includes("/");
  }

  const treeRows = $derived.by((): TreeRow[] => {
    const q = filterText.trim().toLowerCase();
    const filtered = ($workspaceFiles || []).filter((f) => !q || f.path.toLowerCase().includes(q));

    interface DirNode {
      name: string;
      path: string;
      dirs: Map<string, DirNode>;
      files: Array<{ name: string; path: string; size: number }>;
      totalFiles: number;
    }

    const root: DirNode = { name: "", path: "", dirs: new Map(), files: [], totalFiles: 0 };

    for (const entry of filtered) {
      const parts = entry.path.split("/").filter(Boolean);
      if (parts.length === 0) continue;
      let curr = root;
      curr.totalFiles++;
      for (let i = 0; i < parts.length - 1; i++) {
        const seg = parts[i];
        const subPath = parts.slice(0, i + 1).join("/");
        let next = curr.dirs.get(seg);
        if (!next) {
          next = { name: seg, path: subPath, dirs: new Map(), files: [], totalFiles: 0 };
          curr.dirs.set(seg, next);
        }
        next.totalFiles++;
        curr = next;
      }
      const fileName = parts[parts.length - 1];
      curr.files.push({ name: fileName, path: entry.path, size: entry.size });
    }

    const out: TreeRow[] = [];
    function walk(node: DirNode, depth: number) {
      const sortedDirs = Array.from(node.dirs.values()).sort((a, b) => a.name.localeCompare(b.name));
      for (const d of sortedDirs) {
        out.push({
          kind: "dir",
          name: d.name,
          path: d.path,
          depth,
          fileCount: d.totalFiles
        });
        if (!isDirCollapsed(d.path)) {
          walk(d, depth + 1);
        }
      }
      const sortedFiles = [...node.files].sort((a, b) => a.name.localeCompare(b.name));
      for (const f of sortedFiles) {
        out.push({
          kind: "file",
          name: f.name,
          path: f.path,
          depth,
          size: f.size
        });
      }
    }

    walk(root, 0);
    return out;
  });

  async function openFile(path: string) {
    const res = await fetchWorkspaceFile(path, $selectedTask?.id);
    if (res.ok) {
      rightPanelView.set("file");
    }
  }

  function isCodeFile(name: string): boolean {
    return /\.(go|ts|tsx|js|jsx|svelte|py|rs|css|html|json|yaml|yml|toml|sh|sql)$/i.test(name);
  }
</script>

<div class="file-tree-view" data-testid="files-tree-view">
  <div class="tree-toolbar">
    <div class="search-box">
      <Search size={12} />
      <input
        type="text"
        bind:value={filterText}
        placeholder="ค้นหาไฟล์ใน Workspace…"
        aria-label="ค้นหาไฟล์ใน Workspace"
        data-testid="file-tree-filter"
      />
    </div>
    <button
      type="button"
      class="icon-btn"
      onclick={refreshFiles}
      disabled={loading}
      title="รีเฟรชรายการไฟล์"
      aria-label="รีเฟรชรายการไฟล์"
    >
      <RefreshCw size={12} />
    </button>
  </div>

  <div class="ws-root-hdr">
    <FolderOpen size={13} />
    <span class="truncate">{$workspaceStatus?.workspace?.split(/[\\/]/).pop() || "Workspace"}</span>
    <span class="count mono">{$workspaceFiles.length}</span>
  </div>

  <div class="tree-scroll" role="tree" aria-label="Workspace File Tree">
    {#if loading && $workspaceFiles.length === 0}
      <div class="empty">กำลังโหลดโครงสร้างไฟล์…</div>
    {:else if treeRows.length === 0}
      <div class="empty">ไม่พบไฟล์ที่ตรงกับคำค้นหา</div>
    {:else}
      {#each treeRows as row (row.kind + ":" + row.path)}
        {#if row.kind === "dir"}
          {@const collapsed = isDirCollapsed(row.path)}
          <button
            type="button"
            class="tree-row dir-row"
            style={`padding-left: ${10 + row.depth * 14}px`}
            onclick={() => toggleDir(row.path)}
            data-testid="tree-dir-row"
            data-dir-path={row.path}
          >
            <span class="chev">
              {#if collapsed}
                <ChevronRight size={12} />
              {:else}
                <ChevronDown size={12} />
              {/if}
            </span>
            <span class="dir-icon">
              {#if collapsed}
                <Folder size={13} />
              {:else}
                <FolderOpen size={13} />
              {/if}
            </span>
            <span class="name truncate">{row.name}</span>
            <span class="meta mono">{row.fileCount}</span>
          </button>
        {:else}
          {@const diff = diffStatsByFile.get(row.path)}
          <button
            type="button"
            class="tree-row file-row"
            class:active={$activeFileView?.path === row.path}
            style={`padding-left: ${24 + row.depth * 14}px`}
            onclick={() => openFile(row.path)}
            data-testid="tree-file-row"
            data-file-path={row.path}
          >
            <span class="file-icon">
              {#if isCodeFile(row.name)}
                <FileCode size={13} />
              {:else}
                <FileText size={13} />
              {/if}
            </span>
            <span class="name mono truncate">{row.name}</span>
            {#if diff}
              <span class="diff-badge mono">
                <span class="add">+{diff.added}</span>
                <span class="del">−{diff.deleted}</span>
              </span>
            {/if}
          </button>
        {/if}
      {/each}
    {/if}
  </div>
</div>

<style>
  .file-tree-view {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--panel);
  }
  .tree-toolbar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    border-bottom: 1px solid var(--line);
    background: var(--bg);
  }
  .search-box {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--faint);
  }
  .search-box input {
    width: 100%;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 11.5px;
    outline: none;
  }
  .icon-btn {
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    width: 26px;
    height: 26px;
    border-radius: 6px;
    display: grid;
    place-items: center;
    flex: none;
  }
  .icon-btn:hover:not(:disabled) {
    color: var(--text);
    background: var(--hover);
  }
  .ws-root-hdr {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 10px;
    font-size: 11.5px;
    font-weight: 600;
    color: var(--muted);
    border-bottom: 1px solid var(--line);
    background: var(--panel-subtle);
  }
  .ws-root-hdr .count {
    margin-left: auto;
    font-size: 10.5px;
    color: var(--faint);
  }
  .tree-scroll {
    flex: 1;
    overflow-y: auto;
    padding: 4px 0;
  }
  .tree-row {
    width: 100%;
    border: none;
    background: transparent;
    padding-top: 4px;
    padding-bottom: 4px;
    padding-right: 10px;
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text);
    text-align: left;
  }
  .tree-row:hover {
    background: var(--hover);
  }
  .tree-row.active {
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
  }
  .dir-row {
    font-weight: 500;
  }
  .chev,
  .dir-icon,
  .file-icon {
    display: inline-flex;
    align-items: center;
    color: var(--muted);
    flex: none;
  }
  .name {
    flex: 1;
    min-width: 0;
  }
  .meta {
    font-size: 10.5px;
    color: var(--faint);
  }
  .diff-badge {
    display: inline-flex;
    gap: 4px;
    font-size: 10.5px;
    font-weight: 600;
  }
  .diff-badge .add {
    color: var(--addt);
  }
  .diff-badge .del {
    color: var(--delt);
  }
  .truncate {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .empty {
    padding: 16px;
    font-size: 12px;
    color: var(--faint);
  }
</style>
