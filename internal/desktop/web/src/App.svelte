<script lang="ts">
  import { onMount } from "svelte";
  import { PanelLeftOpen, PanelRightOpen } from "lucide-svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import Thread from "./components/Thread.svelte";
  import ReviewPanel from "./components/ReviewPanel.svelte";
  import SettingsModal from "./components/SettingsModal.svelte";
  import AutonomyView from "./components/AutonomyView.svelte";
  import AgentBoardView from "./components/AgentBoardView.svelte";
  import ContextMenu from "./components/ContextMenu.svelte";
  import {
    connectionState,
    connectionError,
    loadStatus,
    loadAutonomy
  } from "./lib/stores/workspace";
  import {
    ensureActiveSession,
    loadSessions,
    createNewConversation
  } from "./lib/stores/sessions";
  import { loadTasks, loadApprovals, runningTasks } from "./lib/stores/tasks";
  import {
    initSettingsStore,
    refreshProjectFolders,
    leftWidth,
    rightWidth,
    leftCollapsed,
    rightCollapsed,
    toggleLeftSidebar,
    toggleRightPanel,
    settingsModalOpen,
    workspacePopoverOpen,
    centerView,
    contextMenu,
    persistUIPreferences
  } from "./lib/stores/settings";
  import {
    inDesktopShell,
    isBlockedBrowserShortcut,
    chooseWorkspace,
    openExternalURL
  } from "./lib/shell";
  import { t } from "./lib/i18n";

  let resizingSide = $state<"left" | "right" | null>(null);

  async function bootstrap() {
    await initSettingsStore();
    const st = await loadStatus();
    if (st) {
      await ensureActiveSession();
      await Promise.all([loadSessions(), loadTasks(), loadApprovals(), loadAutonomy()]);
    }
  }

  onMount(() => {
    void bootstrap();
    const statusTimer = setInterval(async () => {
      const prev = $connectionState;
      const st = await loadStatus();
      void refreshProjectFolders();
      if (st && prev !== "connected") {
        await ensureActiveSession();
        await Promise.all([loadSessions(), loadTasks(), loadApprovals(), loadAutonomy()]);
      } else if (st && $runningTasks.length > 0) {
        void loadTasks();
        void loadSessions();
      }
    }, 4000);

    return () => clearInterval(statusTimer);
  });

  function onGlobalKeyDown(e: KeyboardEvent) {
    const key = (e.key || "").toLowerCase();
    const code = e.code || "";
    const ctrl = e.ctrlKey || e.metaKey;

    if (inDesktopShell()) {
      if (isBlockedBrowserShortcut(e)) {
        e.preventDefault();
        e.stopPropagation();
        return;
      }
      if (ctrl && !e.shiftKey && (key === "o" || code === "KeyO")) {
        e.preventDefault();
        e.stopPropagation();
        void chooseWorkspace();
        return;
      }
    }

    if (ctrl && !e.shiftKey && (key === "b" || code === "KeyB")) {
      e.preventDefault();
      toggleLeftSidebar();
    } else if (ctrl && !e.shiftKey && (key === "j" || code === "KeyJ")) {
      e.preventDefault();
      toggleRightPanel();
    } else if (ctrl && !e.shiftKey && (key === "k" || code === "KeyK")) {
      e.preventDefault();
      const input = document.getElementById("task-input") as HTMLTextAreaElement | null;
      input?.focus();
    } else if (ctrl && !e.shiftKey && (key === "n" || code === "KeyN")) {
      e.preventDefault();
      void createNewConversation();
    } else if (ctrl && !e.shiftKey && (key === "," || code === "Comma")) {
      e.preventDefault();
      settingsModalOpen.update((v) => !v);
    } else if (e.key === "Escape") {
      settingsModalOpen.set(false);
      workspacePopoverOpen.set(false);
      contextMenu.set(null);
    }
  }

  function onGlobalClick(e: MouseEvent) {
    if (!inDesktopShell()) return;
    const target = e.target as HTMLElement | null;
    const link = target?.closest?.('a[href^="http:"], a[href^="https:"]') as HTMLAnchorElement | null;
    if (link) {
      e.preventDefault();
      openExternalURL(link.href);
    }
  }

  function onGlobalContextMenu(e: MouseEvent) {
    if (!inDesktopShell()) return;
    e.preventDefault();
    const sel = window.getSelection()?.toString() || "";
    const target = e.target as HTMLElement | null;
    const fileEl = target?.closest?.("[data-file-path]") as HTMLElement | null;
    const filePath = fileEl?.getAttribute("data-file-path") || undefined;
    if (sel || filePath) {
      contextMenu.set({ x: e.clientX, y: e.clientY, copyText: sel, filePath });
    }
  }

  // Side panels resize freely; they only keep a small minimum and leave the center pane room.
  const LEFT_MIN = 160;
  const RIGHT_MIN = 240;
  const CENTER_MIN = 320;
  const RESIZERS = 12;

  function clampLeft(w: number): number {
    const right = $rightCollapsed ? 0 : $rightWidth;
    return Math.round(Math.max(LEFT_MIN, Math.min(w, window.innerWidth - right - CENTER_MIN - RESIZERS)));
  }

  function clampRight(w: number): number {
    const left = $leftCollapsed ? 0 : $leftWidth;
    return Math.round(Math.max(RIGHT_MIN, Math.min(w, window.innerWidth - left - CENTER_MIN - RESIZERS)));
  }

  function onPointerMove(e: PointerEvent) {
    if (!resizingSide) return;
    if (resizingSide === "left") {
      leftWidth.set(clampLeft(e.clientX));
    } else if (resizingSide === "right") {
      rightWidth.set(clampRight(window.innerWidth - e.clientX));
    }
  }

  function onWindowResize() {
    leftWidth.set(clampLeft($leftWidth));
    rightWidth.set(clampRight($rightWidth));
  }

  function onPointerUp() {
    if (resizingSide) {
      resizingSide = null;
      persistUIPreferences();
    }
  }
  function onResizerKeyDown(side: "left" | "right", e: KeyboardEvent) {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const delta = e.key === "ArrowRight" ? 16 : -16;
    if (side === "left") {
      leftWidth.update((w) => clampLeft(w + delta));
    } else {
      rightWidth.update((w) => clampRight(w - delta));
    }
    persistUIPreferences();
  }
</script>

<svelte:window
  onkeydown={onGlobalKeyDown}
  onclick={onGlobalClick}
  oncontextmenu={onGlobalContextMenu}
  onpointermove={onPointerMove}
  onpointerup={onPointerUp}
  onresize={onWindowResize}
  ondragover={(e) => {
    if (inDesktopShell()) e.preventDefault();
  }}
  ondrop={(e) => {
    if (inDesktopShell()) e.preventDefault();
  }}
/>

{#if $connectionState === "connecting" && !$connectionError}
  <div class="connecting-screen" data-testid="connecting-screen">
    <img src="/bee-icon.png" alt="APW" class="logo-img" />
    <h2>{$t("Starting APW service (apwd)…", "กำลังเริ่มต้นบริการ APW (apwd)…")}</h2>
    <p>{$t("Connecting to your Workspace", "กำลังเชื่อมต่อกับ Workspace ของคุณ")}</p>
  </div>
{:else if $connectionState === "connecting" && $connectionError}
  <div class="connecting-screen" data-testid="connecting-screen">
    <img src="/bee-icon.png" alt="APW" class="logo-img" />
    <h2>{$t("Connecting to APW service…", "กำลังเชื่อมต่อกับบริการ APW…")}</h2>
    <p class="mono err-txt selectable">{$connectionError}</p>
    <button type="button" class="retry-btn" onclick={bootstrap}>{$t("Retry Connection", "ลองเชื่อมต่อใหม่")}</button>
  </div>
{:else}
  <div class="app-shell" data-testid="app-shell">
    {#if !$leftCollapsed}
      <div class="left-pane" style={`width:${$leftWidth}px`}>
        <Sidebar />
      </div>

      <button
        type="button"
        id="left-resizer"
        class="resizer"
        onpointerdown={() => (resizingSide = "left")}
        aria-label={$t("Resize left sidebar", "ปรับขนาดแถบด้านซ้าย")}
        onkeydown={(e) => onResizerKeyDown("left", e)}
      ></button>
    {/if}

    <div class="center-pane">
      {#if $leftCollapsed}
        <button
          type="button"
          class="reopen-pane-btn left"
          onclick={() => toggleLeftSidebar()}
          title={$t("Show left sidebar (Ctrl+B)", "แสดงแถบด้านซ้าย (Ctrl+B)")}
          aria-label={$t("Show left sidebar", "แสดงแถบด้านซ้าย")}
          data-testid="reopen-left-sidebar-btn"
        >
          <img src="/bee-icon.png" alt="" class="reopen-bee" />
          <PanelLeftOpen size={15} />
        </button>
      {/if}
      {#if $rightCollapsed}
        <button
          type="button"
          class="reopen-pane-btn right"
          onclick={() => toggleRightPanel()}
          title={$t("Show right panel (Ctrl+J)", "แสดงแถบด้านขวา (Ctrl+J)")}
          aria-label={$t("Show right panel", "แสดงแถบด้านขวา")}
          data-testid="reopen-right-panel-btn"
        >
          <PanelRightOpen size={15} />
        </button>
      {/if}
      {#if $centerView === "autonomy"}
        <AutonomyView />
      {:else if $centerView === "board"}
        <AgentBoardView />
      {:else}
        <Thread />
      {/if}
    </div>

    {#if !$rightCollapsed}

      <button
        type="button"
        id="right-resizer"
        class="resizer"
        onpointerdown={() => (resizingSide = "right")}
        aria-label={$t("Resize right panel", "ปรับขนาดแถบด้านขวา")}
        onkeydown={(e) => onResizerKeyDown("right", e)}
      ></button>
      <div class="right-pane" style={`width:${$rightWidth}px`}>
        <ReviewPanel />
      </div>
    {/if}
  </div>

  <SettingsModal />
  <ContextMenu />
{/if}

<style>
  .app-shell {
    width: 100%;
    height: 100%;
    display: flex;
    min-height: 0;
    min-width: 0;
    background: var(--bg);
  }
  .left-pane {
    height: 100%;
    flex: none;
    border-right: 1px solid var(--line);
  }
  .center-pane {
    position: relative;
    flex: 1;
    min-width: 0;
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  .reopen-pane-btn {
    position: absolute;
    top: 10px;
    z-index: 40;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 5px 9px;
    border-radius: 7px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    box-shadow: var(--shadow-sm);
    cursor: pointer;
  }
  .reopen-pane-btn.left {
    left: 12px;
  }
  .reopen-pane-btn.right {
    right: 12px;
  }
  .reopen-pane-btn:hover {
    background: var(--hover);
    border-color: var(--line-strong);
  }
  .reopen-bee {
    width: 16px;
    height: 16px;
    object-fit: contain;
  }
  .right-pane {
    height: 100%;
    flex: none;
    border-left: 1px solid var(--line);
  }
  .resizer {
    width: 4px;
    border: none;
    padding: 0;
    cursor: col-resize;
    background: transparent;
    flex: none;
  }
  .resizer:hover {
    background: var(--accent-soft);
  }
  .connecting-screen {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 24px;
    text-align: center;
    background: var(--bg);
    color: var(--text);
  }
  .logo-img {
    width: 64px;
    height: 64px;
    object-fit: contain;
    margin-bottom: 6px;
  }
  .connecting-screen h2 {
    margin: 0;
    font-size: 16px;
  }
  .connecting-screen p {
    margin: 0;
    font-size: 12.5px;
    color: var(--muted);
    max-width: 440px;
  }
  .err-txt {
    color: var(--err) !important;
  }
  .retry-btn {
    margin-top: 8px;
    padding: 6px 14px;
    border-radius: 6px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
  }
</style>
