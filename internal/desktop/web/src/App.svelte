<script lang="ts">
  import { onMount } from "svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import Thread from "./components/Thread.svelte";
  import ReviewPanel from "./components/ReviewPanel.svelte";
  import SettingsModal from "./components/SettingsModal.svelte";
  import AutonomyView from "./components/AutonomyView.svelte";
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
  import { loadTasks, loadApprovals } from "./lib/stores/tasks";
  import {
    initSettingsStore,
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
      if (st && prev !== "connected") {
        await ensureActiveSession();
        await Promise.all([loadSessions(), loadTasks(), loadApprovals(), loadAutonomy()]);
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

  function onPointerMove(e: PointerEvent) {
    if (!resizingSide) return;
    if (resizingSide === "left") {
      const next = Math.max(180, Math.min(380, e.clientX));
      leftWidth.set(next);
    } else if (resizingSide === "right") {
      const next = Math.max(280, Math.min(580, window.innerWidth - e.clientX));
      rightWidth.set(next);
    }
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
      leftWidth.update((w) => Math.max(180, Math.min(380, w + delta)));
    } else {
      rightWidth.update((w) => Math.max(280, Math.min(580, w - delta)));
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
  ondragover={(e) => {
    if (inDesktopShell()) e.preventDefault();
  }}
  ondrop={(e) => {
    if (inDesktopShell()) e.preventDefault();
  }}
/>

{#if $connectionState === "connecting" && !$connectionError}
  <div class="connecting-screen" data-testid="connecting-screen">
    <div class="logo">A</div>
    <h2>กำลังเริ่มต้นบริการ APW (apwd)…</h2>
    <p>กำลังเชื่อมต่อกับ Workspace ของคุณ</p>
  </div>
{:else if $connectionState === "connecting" && $connectionError}
  <div class="connecting-screen" data-testid="connecting-screen">
    <div class="logo">A</div>
    <h2>กำลังเชื่อมต่อกับบริการ APW…</h2>
    <p class="mono err-txt selectable">{$connectionError}</p>
    <button type="button" class="retry-btn" onclick={bootstrap}>ลองเชื่อมต่อใหม่</button>
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
        aria-label="ปรับขนาดแถบด้านซ้าย"
        onkeydown={(e) => onResizerKeyDown("left", e)}
      ></button>
    {/if}

    <div class="center-pane">
      {#if $centerView === "autonomy"}
        <AutonomyView />
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
        aria-label="ปรับขนาดแถบด้านขวา"
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
    flex: 1;
    min-width: 0;
    height: 100%;
    display: flex;
    flex-direction: column;
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
  .logo {
    width: 36px;
    height: 36px;
    border-radius: 9px;
    background: var(--accent);
    color: var(--accent-fg);
    font-weight: 700;
    font-size: 18px;
    display: grid;
    place-items: center;
    margin-bottom: 4px;
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
