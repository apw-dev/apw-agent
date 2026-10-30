<script lang="ts">
  import { ArrowUp, Cpu, Folder, FolderPlus, ChevronDown, Check } from "lucide-svelte";
  import MentionMenu from "./MentionMenu.svelte";
  import {
    composerMode,
    submitComposerInstruction,
    runProjectVerify,
    type ComposerMode
  } from "../lib/stores/tasks";
  import {
    workspaceStatus,
    activeProjectPath,
    loadStatus,
    fetchWorkspaceFiles,
    fetchWorkspaceSymbols,
    fetchWorkspaceGraph
  } from "../lib/stores/workspace";
  import { createNewConversation } from "../lib/stores/sessions";
  import {
    settingsModalOpen,
    centerView,
    rightCollapsed,
    rightPanelView,
    composerDraftText,
    recentWorkspaces,
    projectFolders,
    refreshProjectFolders
  } from "../lib/stores/settings";
  import { chooseWorkspace, getNativeShellState, inDesktopShell } from "../lib/shell";
  import { t } from "../lib/i18n";
  import {
    mentionCandidates,
    mentionSelectedIndex,
    detectActiveToken,
    queryMentionCandidates,
    clearMentionCandidates,
    parseInstructionSkillPills,
    type MentionCandidate
  } from "../lib/stores/search";

  let text = $state("");
  let textareaEl: HTMLTextAreaElement | undefined = $state();
  let busy = $state(false);
  let submitError = $state("");
  const activeSkills = $derived(parseInstructionSkillPills(text).skills);

  const currentProjectPath = $derived(
    $activeProjectPath ||
      $workspaceStatus?.workspace ||
      $recentWorkspaces[0] ||
      $projectFolders.find((p) => p.active)?.path ||
      $projectFolders[0]?.path ||
      ""
  );

  const currentProjectName = $derived.by(() => {
    if (!currentProjectPath) return "apw-agent";
    const clean = currentProjectPath.replace(/\\/g, "/").replace(/\/+$/, "");
    return clean.split("/").pop() || clean;
  });

  let projectMenuOpen = $state(false);
  let customProjectInput = $state("");

  function normPath(p?: string): string {
    return (p || "").replace(/\\/g, "/").replace(/\/+$/, "").toLowerCase();
  }

  function folderBaseName(p: string): string {
    const clean = p.replace(/\\/g, "/").replace(/\/+$/, "");
    return clean.split("/").pop() || clean;
  }

  const selectableProjects = $derived.by(() => {
    const map = new Map<string, { path: string; name: string }>();
    for (const p of $projectFolders || []) {
      if (p?.path && p.path !== "__current__") {
        map.set(normPath(p.path), { path: p.path, name: p.name || folderBaseName(p.path) });
      }
    }
    for (const r of $recentWorkspaces || []) {
      if (r && !map.has(normPath(r))) {
        map.set(normPath(r), { path: r, name: folderBaseName(r) });
      }
    }
    if (currentProjectPath && !map.has(normPath(currentProjectPath))) {
      map.set(normPath(currentProjectPath), {
        path: currentProjectPath,
        name: folderBaseName(currentProjectPath)
      });
    }
    return Array.from(map.values()).sort((a, b) =>
      a.name.toLowerCase().localeCompare(b.name.toLowerCase())
    );
  });

  const resolvedCustomProjectPath = $derived.by(() => {
    const raw = customProjectInput.trim();
    if (!raw) return "";
    if (raw.includes("\\") || raw.includes("/") || raw.includes(":")) {
      return raw;
    }
    if (currentProjectPath) {
      const useBackslash = currentProjectPath.includes("\\");
      const sep = useBackslash ? "\\" : "/";
      const normalized = currentProjectPath.replace(/[\\/]+$/, "");
      const lastSep = Math.max(normalized.lastIndexOf("\\"), normalized.lastIndexOf("/"));
      if (lastSep > 0) {
        return normalized.slice(0, lastSep) + sep + raw;
      }
    }
    return raw;
  });

  async function selectProjectFromChip(targetPath: string) {
    const clean = targetPath.trim();
    if (!clean) return;
    projectMenuOpen = false;
    customProjectInput = "";
    await createNewConversation(clean);
  }

  async function handleBrowseNewProject() {
    projectMenuOpen = false;
    await chooseWorkspace();
    const shell = await getNativeShellState();
    const chosen = shell?.current_workspace || "";
    if (chosen) {
      await createNewConversation(chosen);
    } else {
      await Promise.all([loadStatus(), refreshProjectFolders()]);
    }
  }

  async function handleApplyCustomProjectPath() {
    const target = resolvedCustomProjectPath.trim();
    if (!target) return;
    await selectProjectFromChip(target);
  }

  function removeSkillPill(slug: string) {
    const escaped = slug.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const re = new RegExp(`(?:@skill:${escaped}|\\[<>[\\s]*${escaped}\\])\\s*`, "gi");
    text = text.replace(re, "");
    textareaEl?.focus();
  }

  function triggerSkillPicker() {
    const prefix = text && !text.endsWith(" ") ? text + " @skill:" : text + "@skill:";
    text = prefix;
    queueMicrotask(() => {
      if (textareaEl) {
        textareaEl.focus();
        textareaEl.setSelectionRange(text.length, text.length);
        const tok = detectActiveToken(text, text.length);
        void queryMentionCandidates(tok, handleImmediateAction);
      }
    });
  }

  const MODES: Array<{ value: ComposerMode; label: string }> = [
    { value: "execute", label: "Execute" },
    { value: "preview", label: "Preview" },
    { value: "review", label: "Review" },
    { value: "plan", label: "Plan" },
    { value: "unverified", label: "Unverified" }
  ];

  function handleImmediateAction(actionName: string) {
    text = "";
    clearMentionCandidates();
    switch (actionName) {
      case "verify":
        void runProjectVerify();
        break;
      case "files":
        void fetchWorkspaceFiles().then(() => {
          rightCollapsed.set(false);
          rightPanelView.set("files");
        });
        break;
      case "symbols":
        void fetchWorkspaceSymbols().then(() => {
          rightCollapsed.set(false);
          rightPanelView.set("symbols");
        });
        break;
      case "graph":
        void fetchWorkspaceGraph().then(() => {
          rightCollapsed.set(false);
          rightPanelView.set("graph");
        });
        break;
      case "autonomy":
        centerView.set("autonomy");
        break;
      case "board":
        centerView.set("board");
        break;
      case "settings":
        settingsModalOpen.set(true);
        break;
      case "new":
        void createNewConversation();
        break;
    }
  }

  function adjustTextareaHeight() {
    if (!textareaEl) return;
    textareaEl.style.height = "auto";
    const next = Math.min(260, Math.max(44, textareaEl.scrollHeight));
    textareaEl.style.height = `${next}px`;
    textareaEl.style.overflowY = textareaEl.scrollHeight > 260 ? "auto" : "hidden";
  }

  $effect(() => {
    void text;
    adjustTextareaHeight();
  });

  $effect(() => {
    const draft = $composerDraftText;
    if (draft) {
      text = draft;
      composerDraftText.set("");
      queueMicrotask(() => {
        adjustTextareaHeight();
        textareaEl?.focus();
      });
    }
  });

  function onInput() {
    submitError = "";
    adjustTextareaHeight();
    const cursor = textareaEl?.selectionStart ?? text.length;
    const tok = detectActiveToken(text, cursor);
    void queryMentionCandidates(tok, handleImmediateAction);
  }

  function applyCandidate(cand: MentionCandidate) {
    if (cand.action) {
      cand.action();
      return;
    }
    const cursor = textareaEl?.selectionStart ?? text.length;
    const tok = detectActiveToken(text, cursor);
    if (!tok) return;
    const replacement = tok.kind === "slash" ? cand.insert : "@" + cand.insert + " ";
    text = text.slice(0, tok.atStart) + replacement + text.slice(tok.atEnd);
    const nextPos = tok.atStart + replacement.length;
    clearMentionCandidates();
    queueMicrotask(() => {
      if (textareaEl) {
        textareaEl.focus();
        textareaEl.setSelectionRange(nextPos, nextPos);
      }
    });
  }

  async function onSend() {
    const val = text.trim();
    if (!val || busy) return;
    if (val === "/verify") {
      handleImmediateAction("verify");
      return;
    }
    if (val === "/files") {
      handleImmediateAction("files");
      return;
    }
    if (val === "/symbols") {
      handleImmediateAction("symbols");
      return;
    }
    if (val === "/graph") {
      handleImmediateAction("graph");
      return;
    }
    if (val === "/autonomy") {
      handleImmediateAction("autonomy");
      return;
    }
    if (val === "/board") {
      handleImmediateAction("board");
      return;
    }
    if (val === "/settings") {
      handleImmediateAction("settings");
      return;
    }
    if (val === "/new") {
      handleImmediateAction("new");
      return;
    }
    busy = true;
    submitError = "";
    const currentText = text;
    text = "";
    clearMentionCandidates();
    const res = await submitComposerInstruction(currentText);
    if (!res.ok) {
      text = currentText;
      submitError = res.message;
    }
    busy = false;
  }

  function onKeyDown(e: KeyboardEvent) {
    const cands = $mentionCandidates;
    if (cands.length > 0) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        mentionSelectedIndex.update((i) => (i + 1) % cands.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        mentionSelectedIndex.update((i) => (i - 1 + cands.length) % cands.length);
        return;
      }
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
        e.preventDefault();
        applyCandidate(cands[$mentionSelectedIndex]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        clearMentionCandidates();
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void onSend();
    }
  }
</script>

<div class="composer-wrap">
  {#if submitError}
    <div class="submit-err" data-testid="composer-error">{submitError}</div>
  {/if}

  <div class="composer">
    <MentionMenu onSelect={applyCandidate} />

    {#if activeSkills.length > 0}
      <div class="skill-pills-bar" data-testid="composer-skill-pills">
        {#each activeSkills as sk (sk)}
          <span class="skill-pill mono" data-testid="composer-skill-pill">
            <span class="skill-icon">&lt;&gt;</span>
            <span>{sk}</span>
            <button
              type="button"
              class="remove-skill-btn"
              onclick={() => removeSkillPill(sk)}
              aria-label={$t(`Remove Skill ${sk}`, `ลบ Skill ${sk}`)}
              title={$t(`Remove Skill ${sk}`, `ลบ Skill ${sk}`)}
            >
              ×
            </button>
          </span>
        {/each}
      </div>
    {/if}

    <textarea
      id="task-input"
      bind:this={textareaEl}
      bind:value={text}
      oninput={onInput}
      onkeydown={onKeyDown}
      rows="2"
      placeholder={$t(
        "Ask anything… Use @ to mention files/skills (@skill:), / for commands (Ctrl+K)",
        "สั่งงาน… ใช้ @ อ้างไฟล์/Skill (@skill:), / สำหรับคำสั่ง (Ctrl+K)"
      )}
      data-testid="composer-input"
    ></textarea>

    <div class="toolbar">
      <div class="project-chip-wrap">
        <button
          type="button"
          class="chip project-chip"
          onclick={(e) => {
            e.stopPropagation();
            projectMenuOpen = !projectMenuOpen;
          }}
          title={currentProjectPath
            ? $t(
                `Click to switch project or select a new path (${currentProjectPath})`,
                `คลิกเพื่อเปลี่ยนโปรเจกต์ หรือเลือก Path ใหม่ (${currentProjectPath})`
              )
            : $t("Click to select a project", "คลิกเพื่อเลือกโปรเจกต์")}
          data-testid="composer-project-chip"
        >
          <Folder size={12} class="project-chip-icon" />
          <span class="project-chip-name">{currentProjectName}</span>
          <ChevronDown size={11} class="project-chip-caret" />
        </button>

        {#if projectMenuOpen}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="project-menu-backdrop"
            onclick={() => (projectMenuOpen = false)}
          ></div>
          <div class="project-menu" data-testid="composer-project-menu">
            <div class="project-menu-hdr">{$t("Select project for this conversation", "เลือกโปรเจกต์สำหรับบทสนทนานี้")}</div>
            <div class="project-menu-list">
              {#each selectableProjects as proj (proj.path)}
                {@const isCur = normPath(proj.path) === normPath(currentProjectPath)}
                <button
                  type="button"
                  class="project-menu-item"
                  class:active={isCur}
                  onclick={() => void selectProjectFromChip(proj.path)}
                  title={proj.path}
                >
                  <Folder size={13} class="proj-item-icon" />
                  <div class="proj-item-info">
                    <span class="proj-item-name">{proj.name}</span>
                    <span class="proj-item-path mono">{proj.path}</span>
                  </div>
                  {#if isCur}
                    <Check size={13} class="proj-item-check" />
                  {/if}
                </button>
              {/each}
            </div>

            {#if inDesktopShell()}
              <button
                type="button"
                class="project-browse-btn"
                onclick={() => void handleBrowseNewProject()}
                data-testid="composer-browse-project-btn"
              >
                <FolderPlus size={13} />
                <span>{$t("Browse project folder from disk…", "เลือกโฟลเดอร์โปรเจกต์จากเครื่อง… (Browse)")}</span>
              </button>
            {/if}

            <div class="project-custom-box">
              <label class="project-custom-lbl" for="composer-custom-project-input">
                {$t("Or enter a new project path / folder name:", "หรือระบุ Path / ชื่อโปรเจกต์ใหม่ที่ยังไม่มี:")}
              </label>
              <div class="project-custom-row">
                <input
                  id="composer-custom-project-input"
                  type="text"
                  class="project-custom-input mono"
                  bind:value={customProjectInput}
                  placeholder={$t("e.g. D:\\APW_Project\\new-app or folder name", "เช่น D:\\APW_Project\\new-app หรือชื่อโฟลเดอร์ใหม่")}
                  onkeydown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      void handleApplyCustomProjectPath();
                    } else if (e.key === "Escape") {
                      e.preventDefault();
                      projectMenuOpen = false;
                    }
                  }}
                  data-testid="composer-custom-project-input"
                />
                <button
                  type="button"
                  class="project-custom-btn"
                  disabled={!resolvedCustomProjectPath}
                  onclick={() => void handleApplyCustomProjectPath()}
                  data-testid="composer-custom-project-btn"
                >
                  {$t("Open / Create", "เปิด/สร้าง")}
                </button>
              </div>
              {#if resolvedCustomProjectPath}
                <div class="project-custom-preview mono">
                  Path: {resolvedCustomProjectPath}
                </div>
              {/if}
            </div>
          </div>
        {/if}
      </div>

      <select
        class="chip"
        aria-label={$t("Execution mode", "โหมดการทำงาน")}
        value={$composerMode}
        onchange={(e) => composerMode.set((e.currentTarget as HTMLSelectElement).value as ComposerMode)}
        data-testid="mode-select"
      >
        {#each MODES as m (m.value)}
          <option value={m.value}>{m.label}</option>
        {/each}
      </select>

      <button
        type="button"
        class="chip skill-picker-chip mono"
        onclick={triggerSkillPicker}
        title={$t("Attach a Skill to instruction (@skill:)", "เลือกแนบ Skill เข้าคำสั่ง (@skill:)")}
        data-testid="skill-picker-btn"
      >
        <span class="skill-icon">&lt;&gt;</span>
        <span>Skills</span>
      </button>

      <button
        type="button"
        class="chip model-chip"
        onclick={() => settingsModalOpen.set(true)}
        data-testid="brain-model-chip"
      >
        <Cpu size={12} />
        <span>
          Brain: {$workspaceStatus?.brain2_model || "Architect"} / {$workspaceStatus?.brain1_model || "Reflex"}
        </span>
      </button>

      <button
        type="button"
        class="send-btn"
        onclick={onSend}
        disabled={busy || !text.trim()}
        aria-label={$t("Send instruction", "ส่งคำสั่ง")}
        data-testid="composer-send-btn"
      >
        <ArrowUp size={15} />
      </button>
    </div>
  </div>
</div>

<style>
  .composer-wrap {
    width: 100%;
    max-width: 760px;
    padding: 0 20px;
    margin: 0 auto 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .submit-err {
    padding: 6px 10px;
    border-radius: 8px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 12px;
  }
  .composer {
    position: relative;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--panel);
    padding: 9px 11px;
    box-shadow: var(--shadow-sm);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .composer:focus-within {
    border-color: var(--line-strong);
  }
  .skill-pills-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .skill-pill {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 2px 8px;
    border-radius: 6px;
    border: 1px solid var(--line-strong);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11.5px;
    font-weight: 600;
  }
  .skill-icon {
    color: var(--accent);
    font-weight: 700;
  }
  .remove-skill-btn {
    border: none;
    background: transparent;
    color: var(--faint);
    padding: 0 2px;
    font-size: 13px;
    line-height: 1;
  }
  .remove-skill-btn:hover {
    color: var(--err);
  }
  textarea {
    width: 100%;
    min-height: 44px;
    max-height: 260px;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    resize: none;
    line-height: 1.55;
    overflow-y: hidden;
  }
  textarea:focus,
  textarea:focus-visible {
    outline: none;
    box-shadow: none;
  }
  textarea::placeholder {
    color: var(--faint);
  }
  .toolbar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    border: 1px solid var(--line);
    border-radius: 999px;
    padding: 2px 9px;
    font-size: 11.5px;
    background: var(--panel);
    color: var(--muted);
  }
  .chip:hover {
    background: var(--hover);
    color: var(--text);
  }
  .project-chip-wrap {
    position: relative;
    display: inline-flex;
  }
  .project-chip {
    background: var(--panel-subtle);
    border-color: var(--line-strong);
    color: var(--text);
    font-weight: 600;
    max-width: 210px;
    cursor: pointer;
  }
  .project-chip:hover {
    border-color: var(--accent);
  }
  .project-chip :global(.project-chip-icon) {
    color: var(--accent);
    flex-shrink: 0;
  }
  .project-chip :global(.project-chip-caret) {
    color: var(--faint);
    flex-shrink: 0;
  }
  .project-chip-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .project-menu-backdrop {
    position: fixed;
    inset: 0;
    z-index: 80;
  }
  .project-menu {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 0;
    width: 340px;
    max-width: calc(100vw - 48px);
    background: var(--panel);
    border: 1px solid var(--line-strong);
    border-radius: 10px;
    box-shadow: var(--shadow-md);
    padding: 8px;
    z-index: 90;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .project-menu-hdr {
    font-size: 11px;
    font-weight: 700;
    color: var(--faint);
    padding: 2px 6px;
  }
  .project-menu-list {
    max-height: 200px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .project-menu-item {
    width: 100%;
    border: none;
    background: transparent;
    color: var(--text);
    padding: 6px 8px;
    border-radius: 6px;
    display: flex;
    align-items: center;
    gap: 8px;
    text-align: left;
    cursor: pointer;
  }
  .project-menu-item:hover,
  .project-menu-item.active {
    background: var(--hover);
  }
  .project-menu-item :global(.proj-item-icon) {
    color: var(--accent);
    flex-shrink: 0;
  }
  .proj-item-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1;
  }
  .proj-item-name {
    font-size: 12px;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .proj-item-path {
    font-size: 10.5px;
    color: var(--faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .project-menu-item :global(.proj-item-check) {
    color: var(--accent);
    flex-shrink: 0;
  }
  .project-browse-btn {
    width: 100%;
    border: 1px dashed var(--line-strong);
    background: var(--panel-subtle);
    color: var(--text);
    padding: 6px 8px;
    border-radius: 6px;
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 11.5px;
    font-weight: 600;
    cursor: pointer;
  }
  .project-browse-btn:hover {
    background: var(--hover);
    border-color: var(--accent);
  }
  .project-custom-box {
    border-top: 1px solid var(--line);
    padding-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .project-custom-lbl {
    font-size: 11px;
    color: var(--muted);
    padding: 0 2px;
  }
  .project-custom-row {
    display: flex;
    gap: 5px;
  }
  .project-custom-input {
    flex: 1;
    min-width: 0;
    padding: 5px 8px;
    border-radius: 6px;
    border: 1px solid var(--line-strong);
    background: var(--bg);
    color: var(--text);
    font-size: 11.5px;
  }
  .project-custom-input:focus {
    border-color: var(--accent);
  }
  .project-custom-btn {
    padding: 5px 10px;
    border-radius: 6px;
    border: none;
    background: var(--accent);
    color: var(--accent-fg);
    font-size: 11.5px;
    font-weight: 600;
    flex-shrink: 0;
    cursor: pointer;
  }
  .project-custom-btn:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .project-custom-preview {
    font-size: 10.5px;
    color: var(--muted);
    padding: 0 2px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .model-chip {
    max-width: 340px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .send-btn {
    margin-left: auto;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    border: none;
    background: var(--accent);
    color: var(--accent-fg);
    display: grid;
    place-items: center;
  }
  .send-btn:hover:not(:disabled) {
    background: var(--accent-hover);
  }
  .send-btn:disabled {
    opacity: 0.45;
    cursor: default;
  }
</style>
