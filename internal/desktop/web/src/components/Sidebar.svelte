<script lang="ts">
  import {
    Plus,
    Settings,
    Clock,
    Folder,
    FolderPlus,
    Trash2,
    History,
    PanelLeft,
    ShieldCheck,
    ShieldAlert,
    FileText,
    Loader2
  } from "lucide-svelte";
  import {
    workspaceStatus,
    activeProjectPath,
    setActiveProjectPath,
    autonomyState,
    loadStatus,
    loadAutonomy,
    toggleWorkspaceTrust,
    initInstructionsFile,
    loadInstructions,
    fetchWorkspaceFile
  } from "../lib/stores/workspace";
  import {
    sessions,
    groupedSessions,
    currentSessionId,
    localPendingTurns,
    selectSession,
    createNewConversation,
    deleteSession,
    loadSessions,
    ensureActiveSession
  } from "../lib/stores/sessions";
  import {
    runningTasks,
    tasks,
    loadTasks,
    loadApprovals
  } from "../lib/stores/tasks";
  import {
    settingsModalOpen,
    centerView,
    projectFolders,
    refreshProjectFolders,
    toggleLeftSidebar,
    rightCollapsed,
    rightPanelView,
    updateLanguage
  } from "../lib/stores/settings";
  import { locale, t } from "../lib/i18n";
  import {
    inDesktopShell,
    chooseWorkspace,
    openWorkspaceDir,
    switchProjectSession,
    removeProjectFolder
  } from "../lib/shell";

  let expandedProjects = $state<Record<string, boolean>>({});
  let collapsedFolders = $state<Record<string, boolean>>({});

  function workspaceName(p?: string): string {
    if (!p) return "apw-agent";
    const clean = p.replace(/\\/g, "/").replace(/\/+$/, "");
    return clean.split("/").pop() || clean;
  }

  function normPath(p?: string): string {
    return (p || "").replace(/\\/g, "/").replace(/\/+$/, "").toLowerCase();
  }

  function relativeTimeShort(iso?: string): string {
    if (!iso) return "";
    const ts = new Date(iso).getTime();
    if (!Number.isFinite(ts) || ts <= 0) return "";
    const diffSec = Math.max(1, Math.floor((Date.now() - ts) / 1000));
    if (diffSec < 60) return "now";
    const mins = Math.floor(diffSec / 60);
    if (mins < 60) return `${mins}m`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}d`;
    const months = Math.floor(days / 30);
    return `${months}mo`;
  }

  const safeSessions = $derived(Array.isArray($sessions) ? $sessions : []);

  const activeRunningSessionIds = $derived.by(() => {
    const set = new Set<string>();
    for (const job of $runningTasks) {
      if (job.session_id) set.add(job.session_id);
    }
    if ($runningTasks.length > 0 && $currentSessionId) {
      const pending = $localPendingTurns[$currentSessionId];
      if (pending && pending.length > 0) {
        set.add($currentSessionId);
      }
    }
    return set;
  });

  const mergedActiveSessions = $derived.by(() => {
    const known = new Set(safeSessions.map((s) => s.id));
    const extras = [];
    for (const job of $runningTasks) {
      const sid = job.session_id;
      if (sid && !known.has(sid)) {
        known.add(sid);
        extras.push({
          id: sid,
          updated_at: job.created_at || new Date().toISOString(),
          turn_count: 1,
          first_instruction: job.instruction || sid,
          last_instruction: job.instruction || sid
        });
      }
    }
    return [...extras, ...safeSessions];
  });

  const curActivePath = $derived($activeProjectPath || $workspaceStatus?.workspace || "");

  const allProjects = $derived.by(() => {
    const curWs = curActivePath;
    const list = ($projectFolders || []).filter(Boolean);
    if (list.length === 0) {
      return [
        {
          path: curWs || "__current__",
          name: workspaceName(curWs),
          active: true,
          sessions: []
        }
      ];
    }
    const hasCur = curWs && list.some((p) => normPath(p.path) === normPath(curWs));
    if (curWs && !hasCur) {
      return [
        ...list,
        {
          path: curWs,
          name: workspaceName(curWs),
          active: true,
          sessions: []
        }
      ].sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()));
    }
    return list;
  });

  async function refreshActiveProjectState(targetSessionId?: string) {
    await Promise.all([refreshProjectFolders(), loadStatus()]);
    await Promise.all([loadSessions(), loadTasks(), loadApprovals(), loadAutonomy()]);
    if (targetSessionId) {
      await selectSession(targetSessionId);
    } else {
      await ensureActiveSession();
    }
  }

  async function handleSelectProjectFolder(path: string, isActive: boolean) {
    if (isActive) {
      collapsedFolders[path] = !(collapsedFolders[path] ?? false);
      return;
    }
    centerView.set("thread");
    if (path && path !== "__current__") {
      setActiveProjectPath(path);
    }
    currentSessionId.set("");
    await openWorkspaceDir(path);
    await refreshActiveProjectState();
  }

  async function handleSelectProjectConversation(path: string, sessionId: string) {
    centerView.set("thread");
    if (path && path !== "__current__") {
      setActiveProjectPath(path);
    }
    currentSessionId.set(sessionId);
    await switchProjectSession(path, sessionId);
    await refreshActiveProjectState(sessionId);
  }

  async function handleNewConversationInProject(path: string) {
    centerView.set("thread");
    collapsedFolders[path] = false;
    await createNewConversation(path);
  }

  async function handleAddProjectFolder() {
    await chooseWorkspace();
    await refreshActiveProjectState();
  }

  async function handleRemoveProjectFolder(path: string) {
    if (!path || path === "__current__") return;
    await removeProjectFolder(path);
    await refreshActiveProjectState();
  }

  async function handleOpenProjectInstructions() {
    if (!$workspaceStatus) return;
    if (!$workspaceStatus.instructions_loaded) {
      await initInstructionsFile();
      await loadStatus();
    } else {
      await loadInstructions();
    }
    const path = $workspaceStatus?.instructions_path || "APW.md";
    const res = await fetchWorkspaceFile(path);
    if (res.ok) {
      rightCollapsed.set(false);
      rightPanelView.set("file");
    }
  }
</script>

<aside class="sidebar" aria-label={$t("Left navigation sidebar", "แถบนำทางด้านซ้าย")}>
  <!-- Agent Manager Brand & Window Bar (Not a single-workspace IDE switcher) -->
  <div class="agent-toolbar">
    <div class="brand-left">
      <img src="/bee-icon.png" alt="APW" class="brand-bee-icon" />
      <span class="brand-title">apw-agents</span>
    </div>
    <div class="toolbar-right">
      <button
        type="button"
        class="lang-toggle-btn"
        onclick={() => updateLanguage($locale === "en" ? "th" : "en")}
        title={$t("Switch language to Thai (ภาษาไทย)", "สลับเป็นภาษาอังกฤษ (English)")}
        aria-label={$t("Switch language", "สลับภาษา")}
        data-testid="sidebar-lang-toggle"
      >
        {$locale.toUpperCase()}
      </button>
      <button
        type="button"
        class="toolbar-icon-btn"
        onclick={() => toggleLeftSidebar()}
        title={$t("Hide left sidebar (Ctrl+B)", "ซ่อนแถบด้านซ้าย (Ctrl+B)")}
        aria-label={$t("Hide left sidebar", "ซ่อนแถบด้านซ้าย")}
      >
        <PanelLeft size={14} />
      </button>
    </div>
  </div>

  <button
    type="button"
    class="new-conv-btn"
    onclick={() => {
      centerView.set("thread");
      void createNewConversation();
    }}
    data-testid="new-conversation-btn"
  >
    <span class="new-left">
      <Plus size={14} />
      <span>{$t("New Conversation", "บทสนทนาใหม่")}</span>
    </span>
    <kbd>Ctrl+N</kbd>
  </button>

  <!-- Global Agent Manager Navigation (Antigravity Style) -->
  <div class="global-nav">
    <button
      type="button"
      class="global-nav-item"
      class:active={$centerView === "board"}
      onclick={() => centerView.set($centerView === "board" ? "thread" : "board")}
      data-testid="open-agent-board-btn"
    >
      <History size={14} />
      <span>{$t("Conversation History", "ประวัติงาน & กระดานรีวิว")}</span>
    </button>
    <button
      type="button"
      class="global-nav-item"
      class:active={$centerView === "autonomy"}
      onclick={() => centerView.set($centerView === "autonomy" ? "thread" : "autonomy")}
      data-testid="open-autonomy-btn"
    >
      <Clock size={14} />
      <span>{$t("Automations", "ระบบอัตโนมัติ")}</span>
      {#if $autonomyState?.governor?.paused}
        <span class="auto-badge paused">{$t("paused", "หยุดพัก")}</span>
      {:else}
        <span class="auto-badge on">24/7</span>
      {/if}
    </button>
    <button
      type="button"
      class="global-nav-item"
      onclick={() => settingsModalOpen.set(true)}
      data-testid="open-settings-btn"
    >
      <Settings size={14} />
      <span>{$t("Customizations", "การตั้งค่าระบบ")}</span>
      <kbd>Ctrl+,</kbd>
    </button>
  </div>

  <div class="nav-scroll" id="session-list">
    <!-- Multi-Project Header (Antigravity Style) -->
    <div class="projects-header">
      <span class="projects-title">{$t("Projects", "โปรเจกต์")}</span>
      {#if inDesktopShell()}
        <div class="projects-actions">
          <button
            type="button"
            class="proj-hdr-btn"
            onclick={() => void handleAddProjectFolder()}
            title={$t("Add project folder to Projects (Ctrl+O)", "เพิ่มโฟลเดอร์โปรเจกต์เข้าสู่ Projects (Ctrl+O)")}
          >
            <FolderPlus size={13} />
          </button>
        </div>
      {/if}
    </div>

    <!-- Unified Fixed-Order Multi-Project Tree -->
    {#each allProjects as proj (proj.path)}
      {@const isActive = curActivePath
        ? normPath(proj.path) === normPath(curActivePath)
        : Boolean(proj.active)}
      {@const isExpanded = expandedProjects[proj.path] ?? false}
      {@const isCollapsed = collapsedFolders[proj.path] ?? false}
      {@const projSessions = Array.isArray(proj.sessions) ? proj.sessions : []}
      {@const visibleActiveSess = isExpanded
        ? mergedActiveSessions
        : mergedActiveSessions.slice(0, 6)}
      {@const visibleInactiveSess = isExpanded ? projSessions : projSessions.slice(0, 6)}
      {@const folderIsRunning = isActive ? $runningTasks.length > 0 : Boolean(proj.running)}

      <div class="project-group" class:active-project={isActive}>
        <div class="project-folder-row" class:active={isActive}>
          <button
            type="button"
            class="project-folder-btn"
            onclick={() => void handleSelectProjectFolder(proj.path, isActive)}
            title={proj.path}
          >
            <Folder size={14} class="folder-icon" />
            <span class="folder-name">{proj.name}</span>
            {#if folderIsRunning}
              <span class="spin-wrap" title={$t("Running", "กำลังทำงานอยู่")}>
                <Loader2 size={12} class="spin-loader" />
              </span>
            {/if}
          </button>

          {#if isActive}
            <button
              type="button"
              class="trust-mini-btn"
              class:ok={$workspaceStatus?.trusted}
              class:warn={!$workspaceStatus?.trusted}
              onclick={(e) => {
                e.stopPropagation();
                void toggleWorkspaceTrust(!$workspaceStatus?.trusted);
              }}
              title={$workspaceStatus?.trusted
                ? $t("Trusted (click to switch to Untrusted)", "Trusted (คลิกเพื่อสลับเป็น Untrusted)")
                : $t("Untrusted (click to allow build/test in this project)", "Untrusted (คลิกเพื่ออนุญาตรัน build/test ในโปรเจกต์นี้)")}
              data-testid="trust-toggle-btn"
            >
              {#if $workspaceStatus?.trusted}
                <ShieldCheck size={11} />
              {:else}
                <ShieldAlert size={11} />
              {/if}
            </button>
            <button
              type="button"
              class="win-pop-btn"
              onclick={(e) => {
                e.stopPropagation();
                void handleOpenProjectInstructions();
              }}
              title={$workspaceStatus?.instructions_loaded
                ? $t(`Open ${$workspaceStatus.instructions_path || "APW.md"}`, `เปิดอ่าน ${$workspaceStatus.instructions_path || "APW.md"}`)
                : $t("Create project instructions file APW.md", "สร้างไฟล์คำแนะนำโปรเจกต์ APW.md")}
              data-testid="apw-md-btn"
            >
              <FileText size={11} />
            </button>
          {/if}

          <button
            type="button"
            class="win-pop-btn"
            onclick={(e) => {
              e.stopPropagation();
              void handleNewConversationInProject(proj.path);
            }}
            title={$t(`New conversation in ${proj.name}`, `สร้างบทสนทนาใหม่ใน ${proj.name}`)}
          >
            <Plus size={12} />
          </button>

          {#if inDesktopShell() && proj.path && proj.path !== "__current__" && allProjects.length > 1}
            <button
              type="button"
              class="win-pop-btn danger"
              onclick={(e) => {
                e.stopPropagation();
                void handleRemoveProjectFolder(proj.path);
              }}
              title={$t(`Remove ${proj.name} from Projects list (keeps files on disk)`, `ลบ ${proj.name} ออกจากรายการ Projects (ไม่ลบไฟล์ในเครื่อง)`)}
              data-testid="remove-project-btn"
            >
              <Trash2 size={11} />
            </button>
          {/if}
        </div>

        {#if !isCollapsed}
          {#if isActive}
            {#if $runningTasks.length === 0 && $groupedSessions.length === 0 && $tasks.length === 0 && allProjects.length === 1}
              <div class="nav-empty">{$t("No conversations yet", "ยังไม่มีบทสนทนาหรือรายการงาน")}</div>
            {/if}

            {#each visibleActiveSess as sess, idx (`${proj.path}:act:${sess.id}:${idx}`)}
              {@const sessRunning = activeRunningSessionIds.has(sess.id)}
              <div
                class="nav-row"
                class:active={$currentSessionId === sess.id && $centerView === "thread"}
              >
                <button
                  type="button"
                  class="nav-item"
                  class:active={$currentSessionId === sess.id && $centerView === "thread"}
                  onclick={() => {
                    centerView.set("thread");
                    void selectSession(sess.id);
                  }}
                  data-testid="session-item"
                  title={sess.last_instruction ? `${$t("Latest", "ล่าสุด")}: ${sess.last_instruction}` : sess.id}
                >
                  {#if sessRunning}
                    <span class="spin-wrap" title={$t("Working...", "กำลังประมวลผล...")}>
                      <Loader2 size={13} class="spin-loader" />
                    </span>
                  {/if}
                  <span class="label"
                    >{sess.first_instruction || sess.last_instruction || sess.id}</span
                  >
                  {#if sessRunning}
                    <span class="rel-time running-txt">{$t("working", "กำลังทำ")}</span>
                  {:else if sess.updated_at}
                    <span class="rel-time">{relativeTimeShort(sess.updated_at)}</span>
                  {/if}
                </button>
                <button
                  type="button"
                  class="del-btn"
                  onclick={(e) => {
                    e.stopPropagation();
                    void deleteSession(sess.id);
                  }}
                  title={$t("Delete conversation", "ลบ Conversation")}
                  aria-label={$t(`Delete conversation ${sess.last_instruction || sess.id}`, `ลบ Conversation ${sess.last_instruction || sess.id}`)}
                  data-testid="delete-session-btn"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            {/each}

            {#if mergedActiveSessions.length > 6}
              <button
                type="button"
                class="see-all-btn"
                onclick={() => (expandedProjects[proj.path] = !isExpanded)}
              >
                {isExpanded ? $t("Show less", "ย่อรายการ") : $t(`See all (${mergedActiveSessions.length})`, `ดูทั้งหมด (${mergedActiveSessions.length})`)}
              </button>
            {/if}
          {:else}
            {#each visibleInactiveSess as sess, idx (`${proj.path}:${sess.id}:${idx}`)}
              <div class="nav-row">
                <button
                  type="button"
                  class="nav-item"
                  onclick={() => void handleSelectProjectConversation(proj.path, sess.id)}
                  title={`${proj.name}: ${sess.title}`}
                >
                  {#if sess.running}
                    <span class="spin-wrap" title={$t("Working...", "กำลังประมวลผล...")}>
                      <Loader2 size={13} class="spin-loader" />
                    </span>
                  {/if}
                  <span class="label">{sess.title}</span>
                  {#if sess.running}
                    <span class="rel-time running-txt">{$t("working", "กำลังทำ")}</span>
                  {:else if sess.updated_at}
                    <span class="rel-time">{relativeTimeShort(sess.updated_at)}</span>
                  {/if}
                </button>
              </div>
            {/each}

            {#if projSessions.length > 6}
              <button
                type="button"
                class="see-all-btn"
                onclick={() => (expandedProjects[proj.path] = !isExpanded)}
              >
                {isExpanded ? $t("Show less", "ย่อรายการ") : $t(`See all (${projSessions.length})`, `ดูทั้งหมด (${projSessions.length})`)}
              </button>
            {/if}
          {/if}
        {/if}
      </div>
    {/each}
  </div>
</aside>

<style>
  .sidebar {
    width: 100%;
    height: 100%;
    background: var(--side-solid);
    display: flex;
    flex-direction: column;
    padding: 8px;
    gap: 2px;
    position: relative;
  }
  .agent-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 4px 8px 6px;
  }
  .brand-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .brand-bee-icon {
    width: 20px;
    height: 20px;
    object-fit: contain;
    flex-shrink: 0;
  }
  .brand-title {
    font-weight: 700;
    font-size: 13.5px;
    color: var(--text);
    letter-spacing: -0.01em;
  }
  .toolbar-right {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .lang-toggle-btn {
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.03em;
    padding: 2px 6px;
    border-radius: 5px;
    cursor: pointer;
    line-height: 1.2;
  }
  .lang-toggle-btn:hover {
    background: var(--hover);
    color: var(--text);
    border-color: var(--line-strong);
  }
  .toolbar-icon-btn {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 4px 5px;
    border-radius: 5px;
    display: inline-grid;
    place-items: center;
    cursor: pointer;
  }
  .toolbar-icon-btn:hover {
    background: var(--hover);
    color: var(--text);
  }
  .global-nav {
    display: flex;
    flex-direction: column;
    gap: 1px;
    margin-bottom: 10px;
  }
  .global-nav-item {
    width: 100%;
    padding: 6px 8px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    gap: 9px;
    font-size: 12.5px;
    cursor: pointer;
    text-align: left;
  }
  .global-nav-item:hover,
  .global-nav-item.active {
    background: var(--hover);
    color: var(--text);
  }
  .global-nav-item kbd {
    margin-left: auto;
  }
  .trust-mini-btn {
    border: none;
    background: transparent;
    padding: 3px 5px;
    border-radius: 4px;
    display: inline-grid;
    place-items: center;
    cursor: pointer;
  }
  .trust-mini-btn.ok {
    color: var(--ok);
  }
  .trust-mini-btn.warn {
    color: var(--warn);
  }
  .trust-mini-btn:hover {
    background: var(--hover);
  }
  .new-conv-btn {
    margin: 6px 0 8px;
    padding: 6px 8px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--panel);
    color: var(--text);
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 12.5px;
    font-weight: 500;
  }
  .new-conv-btn:hover {
    border-color: var(--line-strong);
    background: var(--hover);
  }
  .new-left {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  kbd {
    color: var(--faint);
    font-family: inherit;
    font-size: 10.5px;
  }
  .nav-scroll {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .projects-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 8px 4px;
  }
  .projects-title {
    font-size: 11px;
    font-weight: 600;
    color: var(--faint);
  }
  .projects-actions {
    display: flex;
    align-items: center;
    gap: 2px;
  }
  .proj-hdr-btn {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 3px 5px;
    border-radius: 5px;
    display: inline-grid;
    place-items: center;
    cursor: pointer;
  }
  .proj-hdr-btn:hover {
    background: var(--hover);
    color: var(--text);
  }
  .project-group {
    display: flex;
    flex-direction: column;
    gap: 1px;
    margin-bottom: 6px;
  }
  .project-folder-row {
    display: flex;
    align-items: center;
    border-radius: 6px;
  }
  .project-folder-row:hover {
    background: var(--hover);
  }
  .project-folder-btn {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 5px 8px;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 12.5px;
    font-weight: 600;
    text-align: left;
    cursor: pointer;
  }
  .project-folder-row.active .project-folder-btn {
    color: var(--text);
  }
  .folder-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .win-pop-btn {
    border: none;
    background: transparent;
    color: var(--faint);
    padding: 4px 6px;
    border-radius: 5px;
    opacity: 0;
    display: inline-grid;
    place-items: center;
    cursor: pointer;
  }
  .project-folder-row:hover .win-pop-btn {
    opacity: 1;
  }
  .win-pop-btn:hover {
    color: var(--text);
    background: var(--hover);
  }
  .win-pop-btn.danger:hover {
    color: var(--err);
    background: var(--err-soft);
  }
  .see-all-btn {
    border: none;
    background: transparent;
    color: var(--faint);
    font-size: 11.5px;
    text-align: left;
    padding: 4px 16px;
    border-radius: 5px;
    cursor: pointer;
  }
  .see-all-btn:hover {
    color: var(--text);
    background: var(--hover);
  }
  .rel-time {
    font-size: 11px;
    color: var(--faint);
    flex-shrink: 0;
    margin-left: 4px;
  }
  .rel-time.running-txt {
    color: var(--accent);
    font-weight: 500;
  }
  .nav-empty {
    font-size: 11.5px;
    color: var(--faint);
    padding: 8px 16px;
  }
  .nav-row {
    display: flex;
    align-items: center;
    border-radius: 6px;
    position: relative;
    flex: none;
  }
  .nav-row:hover {
    background: var(--hover);
  }
  .nav-row.active {
    background: var(--active);
  }
  .nav-item {
    width: 100%;
    flex: none;
    min-width: 0;
    padding: 5px 8px 5px 16px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--text);
    display: flex;
    gap: 7px;
    align-items: center;
    text-align: left;
    font-size: 12.5px;
  }
  .nav-row > .nav-item {
    flex: 1;
    width: auto;
  }
  .nav-item:hover {
    background: var(--hover);
  }
  .nav-item.active {
    background: var(--active);
    font-weight: 500;
  }
  .del-btn {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 4px 6px;
    border-radius: 5px;
    opacity: 0;
    display: inline-grid;
    place-items: center;
    flex: none;
    margin-right: 2px;
  }
  .nav-row:hover .del-btn,
  .nav-row:focus-within .del-btn,
  .del-btn:focus-visible {
    opacity: 1;
  }
  .del-btn:hover {
    color: var(--err);
    background: var(--err-soft);
  }
  .nav-item .label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .spin-wrap {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--accent);
    flex-shrink: 0;
  }
  .spin-wrap :global(.spin-loader) {
    animation: apw-spin 0.9s linear infinite;
  }
  @keyframes apw-spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }
  .auto-badge {
    margin-left: auto;
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 4px;
    font-weight: 600;
  }
  .auto-badge.paused {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .auto-badge.on {
    background: var(--ok-soft);
    color: var(--ok);
  }
</style>
