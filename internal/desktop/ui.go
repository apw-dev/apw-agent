package desktop

const desktopHTML = `<!DOCTYPE html>
<html lang="th" class="dark">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>APW Desktop — Apiwat Agent (3-Pane AI Developer OS)</title>
<style>` + desktopCSS + `</style>
<style>
  :root {
    color-scheme: dark;
    --bg-sidebar: #0c0c0e;
    --bg-canvas: #111113;
    --bg-elevated: #18181b;
    --bg-subtle: #141417;
    --bg-hover: rgba(255, 255, 255, 0.045);
    --bg-active: rgba(255, 255, 255, 0.075);
    --border-hairline: rgba(255, 255, 255, 0.065);
    --border-default: rgba(255, 255, 255, 0.10);
    --border-focus: rgba(255, 255, 255, 0.22);
    --text-primary: #f4f4f5;
    --text-secondary: #a1a1aa;
    --text-muted: #71717a;
  }
  body {
    background-color: var(--bg-canvas) !important;
    color: var(--text-primary) !important;
    font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'Segoe UI', Roboto, sans-serif;
    letter-spacing: -0.01em;
    -webkit-font-smoothing: antialiased;
  }
  /* Enterprise crisp radii (replaces puffy cartoon squircles) */
  .squircle {
    border-radius: 12px;
  }
  .squircle-sm {
    border-radius: 8px;
  }
  .squircle-lg {
    border-radius: 14px;
  }
  ::-webkit-scrollbar {
    width: 5px;
    height: 5px;
  }
  ::-webkit-scrollbar-track {
    background: transparent;
  }
  ::-webkit-scrollbar-thumb {
    background: rgba(255, 255, 255, 0.10);
    border-radius: 9999px;
  }
  ::-webkit-scrollbar-thumb:hover {
    background: rgba(255, 255, 255, 0.22);
  }
  /* Enterprise Sidebar & Navigation Classes */
  .ent-sidebar {
    background-color: var(--bg-sidebar);
    border-right: 1px solid var(--border-hairline);
  }
  .ent-header {
    height: 44px;
    background-color: var(--bg-canvas);
    border-bottom: 1px solid var(--border-hairline);
  }
  .ent-brand-mark {
    width: 24px;
    height: 24px;
    border-radius: 6px;
    background: #1c1c21;
    border: 1px solid rgba(255, 255, 255, 0.14);
    color: #f4f4f5;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: 'JetBrains Mono', Consolas, monospace;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: -0.03em;
  }
  .ent-new-btn {
    width: 100%;
    height: 32px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.035);
    border: 1px solid var(--border-default);
    color: var(--text-primary);
    font-size: 12px;
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 10px;
    transition: background 0.12s ease, border-color 0.12s ease;
  }
  .ent-new-btn:hover {
    background: rgba(255, 255, 255, 0.07);
    border-color: rgba(255, 255, 255, 0.16);
  }
  .ent-search-wrap {
    position: relative;
    display: flex;
    align-items: center;
  }
  .ent-search-icon {
    position: absolute;
    left: 9px;
    width: 13px;
    height: 13px;
    color: var(--text-muted);
    pointer-events: none;
  }
  .ent-search-input {
    width: 100%;
    height: 30px;
    background: #141418;
    border: 1px solid var(--border-hairline);
    border-radius: 7px;
    padding: 0 10px 0 28px;
    font-size: 12px;
    color: var(--text-primary);
    transition: border-color 0.12s ease, background 0.12s ease;
  }
  .ent-search-input::placeholder {
    color: var(--text-muted);
  }
  .ent-search-input:focus {
    outline: none;
    background: #18181d;
    border-color: var(--border-focus);
  }
  .ent-nav-item {
    width: 100%;
    min-height: 30px;
    padding: 4px 8px;
    border-radius: 7px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 12.5px;
    color: var(--text-secondary);
    transition: background 0.12s ease, color 0.12s ease;
    cursor: pointer;
    text-align: left;
  }
  .ent-nav-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .ent-nav-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .ent-nav-icon {
    width: 15px;
    height: 15px;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .ent-nav-item:hover .ent-nav-icon {
    color: var(--text-secondary);
  }
  .ent-section-label {
    font-size: 10.5px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    padding: 0 8px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .ent-list-row {
    padding: 6px 8px;
    border-radius: 7px;
    border: 1px solid transparent;
    cursor: pointer;
    transition: background 0.12s ease, border-color 0.12s ease;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .ent-list-row:hover {
    background: var(--bg-hover);
  }
  .ent-list-row.active,
  .session-active-card {
    background: var(--bg-active);
    border-color: rgba(255, 255, 255, 0.09);
  }
  .ent-toolbar-pill {
    height: 28px;
    padding: 0 10px;
    border-radius: 7px;
    background: #17171b;
    border: 1px solid var(--border-hairline);
    color: var(--text-secondary);
    font-size: 11.5px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
    max-width: 210px;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: background 0.12s ease, color 0.12s ease, border-color 0.12s ease;
  }
  .ent-toolbar-pill:hover {
    background: #1f1f24;
    color: var(--text-primary);
    border-color: var(--border-default);
  }
  .ent-composer {
    background: #18181c;
    border: 1px solid var(--border-default);
    border-radius: 14px;
    box-shadow: 0 12px 32px -8px rgba(0, 0, 0, 0.55);
    transition: border-color 0.15s ease;
  }
  .ent-composer:focus-within {
    border-color: var(--border-focus);
  }
  .ent-chip-btn {
    height: 26px;
    padding: 0 9px;
    border-radius: 6px;
    background: rgba(255, 255, 255, 0.035);
    border: 1px solid var(--border-hairline);
    color: var(--text-secondary);
    font-size: 11.5px;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    white-space: nowrap;
    transition: background 0.12s ease, color 0.12s ease, border-color 0.12s ease;
  }
  .ent-chip-btn:hover {
    background: rgba(255, 255, 255, 0.075);
    color: var(--text-primary);
    border-color: var(--border-default);
  }
  .ent-send-btn {
    height: 28px;
    padding: 0 12px;
    border-radius: 9999px;
    background: #f4f4f5;
    color: #09090b;
    font-size: 11.5px;
    font-weight: 600;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    transition: opacity 0.12s ease, transform 0.1s ease;
  }
  .ent-send-btn:hover {
    opacity: 0.9;
  }
  .ent-user-bubble {
    background: #202026;
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 14px;
    padding: 10px 14px;
    max-width: 78%;
    margin-left: auto;
  }
  .ent-agent-bubble {
    background: transparent;
    border-bottom: 1px solid var(--border-hairline);
    padding: 12px 2px;
  }
  .ent-tool-row {
    background: #141418;
    border: 1px solid var(--border-hairline);
    border-radius: 8px;
    overflow: hidden;
  }
  .diff-table {
    width: 100%;
    border-collapse: collapse;
    font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
    font-size: 12px;
    line-height: 1.55;
  }
  .diff-file-row {
    background: #16161a;
    color: #f4f4f5;
    border-top: 1px solid var(--border-hairline);
    border-bottom: 1px solid var(--border-hairline);
  }
  .diff-line-add {
    background: rgba(16, 185, 129, 0.11);
    color: #A7F3D0;
  }
  .diff-line-del {
    background: rgba(244, 63, 94, 0.12);
    color: #FECDD3;
  }
  .diff-line-hunk {
    background: rgba(59, 130, 246, 0.10);
    color: #93C5FD;
  }
  .diff-num {
    width: 44px;
    min-width: 44px;
    padding: 1px 8px;
    text-align: right;
    color: rgba(161, 161, 170, 0.42);
    user-select: none;
    border-right: 1px solid var(--border-hairline);
  }
  .diff-code {
    padding: 1px 12px;
    white-space: pre-wrap;
    word-break: break-all;
  }
  .panel-resizer {
    width: 4px;
    cursor: col-resize;
    background: transparent;
    transition: background 0.15s ease;
    flex-shrink: 0;
    z-index: 20;
  }
  .panel-resizer:hover,
  .panel-resizer.dragging {
    background: rgba(255, 255, 255, 0.22);
  }
  .block { display: block; }
  .mt-0\.5 { margin-top: 0.125rem; }
  .px-0\.5 { padding-left: 0.125rem; padding-right: 0.125rem; }
  .max-w-\[200px\] { max-width: 200px; }
  .max-w-3xl { max-width: 48rem; }
  .max-h-\[88vh\] { max-height: 88vh; }
  .max-h-40 { max-height: 10rem; }
  .max-h-44 { max-height: 11rem; }
  .flex-\[2\] { flex: 2 2 0%; }
  .min-w-\[150px\] { min-width: 150px; }
  .min-w-\[180px\] { min-width: 180px; }
  .min-w-\[200px\] { min-width: 200px; }
  .min-w-\[220px\] { min-width: 220px; }
  .bg-black\/75 { background-color: rgba(0, 0, 0, 0.75); }
  .backdrop-blur-sm { backdrop-filter: blur(4px); }
  .space-y-1 > :not([hidden]) ~ :not([hidden]) { margin-top: 0.25rem; }
  .bg-rose-500\/15 { background-color: rgba(244, 63, 94, 0.15); }
  .bg-amber-500\/10 { background-color: rgba(245, 158, 11, 0.10); }
  .bg-amber-500\/15 { background-color: rgba(245, 158, 11, 0.15); }
  .hover\:bg-amber-500\/25:hover { background-color: rgba(245, 158, 11, 0.25); }
  .hover\:bg-rose-500\/25:hover { background-color: rgba(244, 63, 94, 0.25); }
  .hover\:bg-emerald-500\/30:hover { background-color: rgba(16, 185, 129, 0.30); }
  .border-white\/\[0\.12\] { border-color: rgba(255, 255, 255, 0.12); }
  .border-\[\#E07A5F\]\/40 { border-color: rgba(224, 122, 95, 0.40); }
  .border-amber-500\/25 { border-color: rgba(245, 158, 11, 0.25); }
  .hover\:border-white\/\[0\.18\]:hover { border-color: rgba(255, 255, 255, 0.18); }
  .hover\:bg-white\/\[0\.08\]:hover { background-color: rgba(255, 255, 255, 0.08); }
  .hover\:bg-white\/\[0\.14\]:hover { background-color: rgba(255, 255, 255, 0.14); }
  @keyframes pulseGlow {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
  .pulse-dot {
    animation: pulseGlow 1.6s infinite ease-in-out;
  }
</style>
</head>
<body class="h-screen w-screen overflow-hidden select-none flex flex-col">

  <!-- MAIN 3-PANE ENTERPRISE WORKBENCH -->
  <div class="flex-1 flex min-h-0 relative overflow-hidden">

    <!-- =================================================================== -->
    <!-- 1. LEFT SIDEBAR (Antigravity + Claude Desktop Enterprise Navigation)-->
    <!-- =================================================================== -->
    <aside id="left-sidebar" class="w-80 ent-sidebar flex flex-col shrink-0 select-text">
      <!-- Top Bar: Monogram, Title, Collapse -->
      <div class="ent-header px-3.5 flex items-center justify-between shrink-0" style="background:transparent;">
        <div class="flex items-center gap-2.5 min-w-0">
          <div class="ent-brand-mark">A</div>
          <span class="text-xs font-semibold text-white truncate">apw-agent</span>
          <span id="hdr-version" class="text-[10px] font-mono text-slate-400">v0.1.0</span>
        </div>
        <div class="flex items-center gap-1">
          <button onclick="toggleLeftSidebar()" title="Toggle Sidebar (Ctrl+B)" class="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.06] transition">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></svg>
          </button>
        </div>
      </div>

      <!-- Search + New Conversation Action Block -->
      <div class="px-3 pt-2.5 pb-2 space-y-2">
        <div class="ent-search-wrap">
          <svg class="ent-search-icon" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path stroke-linecap="round" d="M20 20l-3.5-3.5"/></svg>
          <input
            id="sidebar-search"
            type="text"
            oninput="onSidebarSearch(this.value)"
            placeholder="Search conversations &amp; tasks..."
            class="ent-search-input"
          />
        </div>
        <button onclick="createNewSession()" class="ent-new-btn">
          <svg class="w-3.5 h-3.5 text-slate-300" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" d="M12 5v14M5 12h14"/></svg>
          <span>New Conversation</span>
        </button>
      </div>

      <!-- Enterprise Navigation Links (Automations, Customizations, Graph) -->
      <div class="px-2 pb-2 space-y-0.5 border-b border-white/[0.06]">
        <!-- Automations (24/7 Runtime) -->
        <button onclick="openAutonomyDialog()" class="ent-nav-item">
          <span class="ent-nav-left">
            <svg class="ent-nav-icon" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path stroke-linecap="round" stroke-linejoin="round" d="M12 7v5l3 2"/></svg>
            <span class="truncate">Automations</span>
          </span>
          <span id="sidebar-autonomy-pill" class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-300">ON</span>
        </button>

        <!-- Customizations (APW.md & .apw/skills) -->
        <div id="instr-card" class="ent-nav-item" onclick="openInstructionsInArtifactPane()">
          <span class="ent-nav-left">
            <svg class="ent-nav-icon" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h10"/></svg>
            <span class="truncate">Customizations</span>
            <span class="text-[10px] font-mono text-slate-400">APW.md</span>
          </span>
          <span class="flex items-center gap-1.5 shrink-0">
            <span id="instr-dot" class="w-1.5 h-1.5 rounded-full bg-slate-500"></span>
            <span id="instr-badge" class="text-[10px] font-mono text-slate-400">...</span>
            <button id="btn-create-instr" onclick="event.stopPropagation(); initInstructions();" class="hidden text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.08] hover:bg-white/[0.14] text-white transition">+ Init</button>
          </span>
          <p id="instr-desc" class="hidden"></p>
        </div>

        <!-- AST Call Graph & Symbols -->
        <div class="ent-nav-item" onclick="openGraphArtifact()">
          <span class="ent-nav-left">
            <svg class="ent-nav-icon" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="6" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M8 7.5l3 8M16 7.5l-3 8"/></svg>
            <span class="truncate">Call Impact Graph</span>
          </span>
          <button onclick="event.stopPropagation(); openSymbolsArtifact();" class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.05] hover:bg-white/[0.12] text-slate-300 transition">Symbols</button>
        </div>
      </div>

      <!-- Active Project / Workspace Compact Bar -->
      <div class="px-3 py-2 border-b border-white/[0.06] space-y-1">
        <div class="flex items-center justify-between text-[11px]">
          <div class="flex items-center gap-1.5 min-w-0">
            <svg class="w-3.5 h-3.5 text-slate-400 shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2V7z"/></svg>
            <span id="hdr-workspace" class="font-mono text-[11px] text-slate-200 truncate" title="">Loading...</span>
          </div>
          <span id="ws-stack-badge" class="text-[10px] font-mono px-1.5 py-0.2 rounded bg-white/[0.06] text-slate-300 shrink-0">Go</span>
        </div>
        <div class="flex items-center justify-between text-[11px]">
          <span id="ws-trust-badge" class="flex items-center gap-1.5 text-emerald-400 font-mono text-[10.5px]">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span> Trusted
          </span>
          <button onclick="onTrustControl()" id="btn-trust-toggle" class="text-[10.5px] text-slate-400 hover:text-white transition">Manage trust</button>
        </div>
      </div>

      <!-- Scrollable Lists: Conversations & Background Tasks -->
      <div class="flex-1 overflow-y-auto px-2 py-2.5 space-y-4">
        <!-- Conversations Section -->
        <div class="space-y-1">
          <div class="ent-section-label">
            <span>Conversations</span>
            <span id="session-count-badge" class="font-mono text-[10px] text-slate-400">1</span>
          </div>
          <div id="session-list" class="space-y-0.5">
            <!-- Populated dynamically -->
          </div>
        </div>

        <!-- Background Tasks Section -->
        <div class="space-y-1 pt-2 border-t border-white/[0.05]">
          <div class="ent-section-label">
            <span class="flex items-center gap-1.5">
              <span>Background Tasks</span>
              <span id="task-count-badge" class="font-mono text-[10px] text-slate-400">0</span>
            </span>
            <button onclick="loadTasks()" class="text-[10px] font-mono text-slate-400 hover:text-white transition" title="Refresh tasks">↻</button>
          </div>
          <div id="task-list" class="space-y-0.5">
            <!-- Populated dynamically -->
          </div>
        </div>
      </div>

      <!-- Sidebar Footer: Settings & Daemon Status -->
      <div class="px-3 py-2 border-t border-white/[0.06] flex items-center justify-between text-[11px] text-slate-400 shrink-0">
        <button onclick="openSettingsDialog()" class="flex items-center gap-2 text-xs text-slate-300 hover:text-white transition">
          <svg class="w-3.5 h-3.5 text-slate-400" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path stroke-linecap="round" stroke-linejoin="round" d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 01-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09a1.65 1.65 0 00-1-1.51 1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09a1.65 1.65 0 001.51-1 1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06a1.65 1.65 0 001.82.33h.09a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06a1.65 1.65 0 00-.33 1.82v.09a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z"/></svg>
          <span>Settings</span>
        </button>
        <div class="flex items-center gap-2 font-mono text-[10.5px] truncate">
          <span id="pill-daemon" class="truncate text-slate-400">Ready</span>
          <span class="text-slate-600">·</span>
          <span id="apply-auto-status" class="text-slate-400" title="Auto-apply status">Ask</span>
        </div>
      </div>
    </aside>

    <!-- Left Splitter Handle -->
    <div id="left-resizer" class="panel-resizer" title="Drag to resize sidebar (Double-click to reset)"></div>

    <!-- =================================================================== -->
    <!-- 2. CENTER CONVERSATION STAGE (Breadcrumb Header, Stream, Composer)  -->
    <!-- =================================================================== -->
    <main class="flex-1 flex flex-col min-w-0 select-text" style="background:var(--bg-canvas);">
      <!-- Single-Line Enterprise Breadcrumb Header -->
      <header class="ent-header px-4 flex items-center justify-between gap-3 shrink-0">
        <div class="flex items-center gap-2.5 min-w-0">
          <button id="btn-show-left" onclick="toggleLeftSidebar()" title="Show Sidebar (Ctrl+B)" class="hidden p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.06] transition">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></svg>
          </button>
          <div class="flex items-center gap-2 min-w-0 truncate">
            <span id="active-session-title" class="text-xs font-medium text-white truncate">New Conversation</span>
            <span class="text-slate-600 text-xs">/</span>
            <span id="active-session-badge" class="text-[11px] font-mono text-slate-400 truncate">local-default</span>
          </div>
        </div>

        <div class="flex items-center gap-1.5 shrink-0">
          <button onclick="openSettingsDialog()" id="pill-brain2" title="Architect Model (System 2)" class="hidden md:inline-flex ent-toolbar-pill font-mono">Architect</button>
          <button onclick="openSettingsDialog()" id="pill-brain1" title="Reflex Model (System 1)" class="hidden md:inline-flex ent-toolbar-pill font-mono">Reflex</button>
          <button onclick="openAutonomyDialog()" title="24/7 Autonomous Runtime &amp; Schedules" class="ent-toolbar-pill">
            <span>Automations</span>
            <span id="autonomy-count-badge" class="px-1.5 py-0.2 rounded bg-white/[0.07] text-slate-200 font-mono text-[10px]">0</span>
          </button>
          <button onclick="toggleRightPane()" id="btn-toggle-right" title="Toggle Inspector / Diff Drawer (Ctrl+J)" class="ent-toolbar-pill" style="color:var(--text-primary);border-color:var(--border-default);">
            <svg class="w-3.5 h-3.5 text-slate-300" fill="none" stroke="currentColor" stroke-width="1.8" viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M15 4v16"/></svg>
            <span>Inspector</span>
            <span id="artifact-count-badge" class="px-1.5 py-0.2 rounded bg-white/[0.09] text-white font-mono text-[10px]">0</span>
          </button>
        </div>
      </header>

      <!-- Hidden Legacy Compatibility Elements -->
      <pre id="banner-box" class="hidden"></pre>
      <div id="symbol-list" class="hidden"></div>

      <!-- Conversation & Tool Execution Stream -->
      <div id="feed" class="flex-1 overflow-y-auto px-6 py-5 space-y-3 max-w-3xl w-full mx-auto">
        <!-- Minimalist Enterprise Overview Header -->
        <div class="squircle border border-white/[0.07] p-4 space-y-2.5" style="background:var(--bg-subtle);">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[10.5px] font-mono uppercase tracking-wider text-slate-400">Triad Workbench · Isolated Git Worktree · Polyglot AST</span>
            <span class="text-[10.5px] font-mono text-slate-500">Ctrl+B Sidebar · Ctrl+J Inspector</span>
          </div>
          <div class="text-xs text-slate-300 leading-relaxed">
            สั่งงานด้วยภาษาธรรมชาติ พิมพ์ <code class="text-white bg-white/[0.06] px-1.5 py-0.5 rounded font-mono">@</code> เพื่ออ้างอิงไฟล์/ฟังก์ชัน/Skill หรือพิมพ์ <code class="text-white bg-white/[0.06] px-1.5 py-0.5 rounded font-mono">/</code> เพื่อเรียกคำสั่งด่วน
          </div>
          <div class="flex flex-wrap items-center gap-1.5 pt-1">
            <button onclick="quickPrompt('ช่วยตรวจสอบสถาปัตยกรรมและจุดที่ควรปรับปรุงใน Workspace นี้', 'review')" class="ent-chip-btn">Review Architecture</button>
            <button onclick="runVerify()" class="ent-chip-btn">Run Verification</button>
            <button onclick="openGraphArtifact()" class="ent-chip-btn">Call Impact Graph</button>
            <button onclick="openAutonomyDialog()" class="ent-chip-btn">24/7 Automations</button>
          </div>
        </div>

        <!-- Turns of the selected session -->
        <div id="session-history" class="space-y-2.5"></div>

        <!-- Pending Approvals Container -->
        <div id="approval-feed" class="space-y-2.5"></div>

        <!-- Live Task & Tool Execution Stream -->
        <div id="live-feed" class="space-y-2.5"></div>
      </div>

      <!-- Unified Enterprise Composer (Claude Desktop + Antigravity Desktop Style) -->
      <div class="px-6 pb-4 pt-1 shrink-0">
        <div class="max-w-3xl mx-auto ent-composer p-3 relative">
          <!-- @file / @#symbol / @skill / /command Autocomplete Popup -->
          <div id="mention-menu" class="hidden mb-2 max-h-48 overflow-y-auto squircle-sm bg-[#121216] border border-white/[0.12] p-1 space-y-0.5 text-xs font-mono shadow-2xl"></div>
          <textarea
            id="task-input"
            rows="2"
            placeholder="Ask or instruct APW Agent... (@ to mention files/symbols/skills, / for commands)"
            class="w-full bg-transparent text-xs text-white placeholder-slate-500 focus:outline-none resize-none font-sans leading-relaxed px-1"
            oninput="onComposerInput(event)"
            onkeydown="onComposerKeyDown(event)"
          ></textarea>

          <!-- Composer Bottom Control Bar -->
          <div class="flex flex-wrap items-center justify-between gap-2 pt-2 mt-1 border-t border-white/[0.06]">
            <div class="flex flex-wrap items-center gap-1.5">
              <button type="button" onclick="triggerComposerPrefix('@')" title="Mention file, symbol, or skill (@)" class="ent-chip-btn font-mono">@ Context</button>
              <button type="button" onclick="triggerComposerPrefix('/')" title="Slash commands (/)" class="ent-chip-btn font-mono">/ Commands</button>

              <select id="approval-mode" onchange="onApprovalModeChange(this.value)" class="ent-chip-btn font-mono cursor-pointer focus:outline-none" style="background:#121216;">
                <option value="ask">Mode: Ask before apply</option>
                <option value="auto">Mode: Auto-apply verified</option>
                <option value="plan">Mode: Plan only</option>
                <option value="unverified">Mode: Unverified edit</option>
              </select>

              <button type="button" onclick="runTask('review')" class="ent-chip-btn">Review</button>
              <button type="button" onclick="runTask('preview')" class="ent-chip-btn">Preview Diff</button>
              <button type="button" onclick="runVerify()" class="ent-chip-btn">Verify</button>
            </div>

            <div class="flex items-center gap-2">
              <button type="button" onclick=" openSettingsDialog()" class="ent-chip-btn font-mono" title="Configure Models">Models</button>
              <button type="button" onclick="runTask('submit')" class="ent-send-btn" title="Submit (Enter)">
                <span>Run</span>
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M5 12h14M12 5l7 7-7 7"/></svg>
              </button>
            </div>
          </div>
          <div id="mode-desc" class="pt-1 text-[10.5px] text-slate-500 truncate"></div>
        </div>
      </div>
    </main>

    <!-- Right Splitter Handle -->
    <div id="right-resizer" class="panel-resizer hidden" title="Drag to resize Inspector (Double-click to reset)"></div>

    <!-- =================================================================== -->
    <!-- 3. RIGHT ARTIFACT / DIFF INSPECTOR DRAWER                           -->
    <!-- =================================================================== -->
    <aside id="right-pane" class="w-0 overflow-hidden bg-[#0c0c0e] border-l border-white/[0.07] flex flex-col shrink-0 select-text">
      <!-- Inspector Header -->
      <div class="ent-header px-3.5 flex items-center justify-between shrink-0" style="background:transparent;">
        <div class="flex items-center gap-2 min-w-0">
          <span class="w-2 h-2 rounded-full bg-slate-300"></span>
          <h3 id="artifact-title" class="text-xs font-mono font-medium text-white truncate">Inspector</h3>
        </div>
        <div class="flex items-center gap-1.5">
          <div class="flex bg-[#16161a] p-0.5 rounded-lg border border-white/[0.07] text-[11px] font-medium">
            <button id="tab-btn-diff" onclick="switchArtifactTab('diff')" class="px-2.5 py-1 rounded-md bg-white/[0.12] text-white transition">Diff</button>
            <button id="tab-btn-code" onclick="switchArtifactTab('code')" class="px-2.5 py-1 rounded-md text-slate-400 hover:text-white transition">Code</button>
            <button id="tab-btn-preview" onclick="switchArtifactTab('preview')" class="px-2.5 py-1 rounded-md text-slate-400 hover:text-white transition">Preview</button>
          </div>
          <button onclick="closeRightPane()" title="Close Inspector (Ctrl+J)" class="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.06] transition">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
      </div>

      <!-- Affected Files Bar -->
      <div id="artifact-files-bar" class="px-3.5 py-2 bg-[#121216] border-b border-white/[0.06] space-y-2 text-xs">
        <div class="flex items-center justify-between gap-2">
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <span class="text-[11px] text-slate-400 shrink-0 font-mono">File:</span>
            <select id="artifact-file-select" onchange="onSelectArtifactFile(this.value)" class="bg-[#18181c] text-slate-200 font-mono text-xs border border-white/[0.08] rounded-lg px-2 py-1 truncate flex-1 focus:outline-none">
              <option value="">(Select file to inspect source)</option>
            </select>
          </div>
          <span id="artifact-status-pill" class="text-[10px] font-mono px-2 py-0.5 rounded bg-white/[0.06] text-slate-300 shrink-0">READY</span>
        </div>
        <div id="diff-file-chips" class="flex flex-wrap items-center gap-1 hidden"></div>
      </div>

      <!-- TAB 1: DIFF VIEW -->
      <div id="artifact-tab-diff" class="flex-1 overflow-auto p-3 space-y-3">
        <div id="artifact-conflict-box" class="hidden squircle-sm bg-rose-500/15 border border-rose-500/40 p-3 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold text-rose-300 font-mono">Workspace Conflict / Drift Detected</span>
            <button id="btn-conflict-resume" class="px-2.5 py-1 rounded-lg bg-amber-600 hover:bg-amber-500 text-white text-[11px] font-semibold transition">Resume &amp; Re-Verify</button>
          </div>
          <div id="artifact-conflict-msg" class="text-xs text-rose-200 font-mono whitespace-pre-wrap leading-relaxed"></div>
        </div>

        <div id="diff-empty-state" class="squircle bg-[#141418] border border-white/[0.06] p-6 text-center text-xs text-slate-400">
          No active diff selected. Run <code class="text-slate-200">Preview Diff</code> or <code class="text-slate-200">Execute</code> to inspect line-by-line changes.
        </div>
        <div id="diff-table-container" class="squircle-sm overflow-hidden border border-white/[0.08] bg-[#0e0e11] hidden">
          <table class="diff-table">
            <tbody id="diff-tbody"></tbody>
          </table>
        </div>
        <div id="acceptance-box" class="hidden squircle-sm bg-[#141418] border border-white/[0.07] p-3 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <div class="text-xs font-semibold text-slate-200 font-mono">Verification &amp; Acceptance Evidence</div>
            <span id="acceptance-badge" class="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">VERIFIED</span>
          </div>
          <div id="acceptance-structured" class="space-y-1.5 text-xs"></div>
          <pre id="acceptance-pre" class="text-[11px] font-mono text-slate-300 whitespace-pre-wrap leading-relaxed bg-[#0c0c0e] p-2.5 squircle-sm border border-white/[0.05]"></pre>
        </div>
      </div>

      <!-- TAB 2: CODE / FILE VIEWER -->
      <div id="artifact-tab-code" class="flex-1 overflow-auto p-3 hidden">
        <div class="squircle-sm border border-white/[0.08] bg-[#0e0e11] overflow-hidden">
          <div class="px-3 py-2 bg-[#16161a] border-b border-white/[0.06] flex items-center justify-between">
            <span class="flex items-center gap-2 min-w-0">
              <span id="code-viewer-filename" class="text-xs font-mono text-slate-200 truncate">code.go</span>
              <span id="code-viewer-meta" class="text-[10px] font-mono text-slate-500 shrink-0"></span>
            </span>
            <button onclick="renderCodeAsHTMLPreview()" class="text-[11px] text-slate-300 hover:text-white underline">Open in HTML Preview →</button>
          </div>
          <pre id="code-viewer-pre" class="p-3.5 text-xs font-mono text-slate-200 overflow-x-auto leading-relaxed whitespace-pre">Select a file above or click Customizations (APW.md) to view source.</pre>
        </div>
      </div>

      <!-- TAB 3: HTML PREVIEW -->
      <div id="artifact-tab-preview" class="flex-1 flex flex-col p-3 hidden">
        <div class="squircle-sm border border-white/[0.08] bg-white flex-1 overflow-hidden flex flex-col">
          <div class="px-3 py-1.5 bg-[#16161a] border-b border-white/[0.08] flex items-center justify-between text-[11px] font-mono text-slate-300">
            <span>Sandboxed HTML Preview</span>
            <button onclick="loadSampleHTMLPreview()" class="text-slate-300 hover:text-white underline">Load Sample Preview</button>
          </div>
          <iframe id="html-preview-frame" sandbox="allow-scripts" class="w-full flex-1 border-0 bg-white"></iframe>
        </div>
      </div>

      <!-- Inspector Footer -->
      <div id="artifact-action-bar" class="px-3.5 py-2.5 border-t border-white/[0.07] bg-[#121216] flex items-center justify-between gap-2 shrink-0">
        <div class="text-[11px] text-slate-400 font-mono truncate" id="artifact-task-id">No pending changes</div>
        <div id="artifact-buttons" class="flex items-center gap-1.5"></div>
      </div>
    </aside>

  </div>

  <!-- TRUST CONFIRMATION DIALOG (in-page; opened by the trust control or an "untrusted" 409) -->
  <div id="trust-dialog" class="hidden fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4 select-text" role="dialog" aria-modal="true" aria-labelledby="trust-title">
    <div class="squircle bg-[#18181c] border border-white/[0.12] shadow-2xl w-full max-w-lg p-5 space-y-3">
      <h3 id="trust-title" class="text-sm font-semibold text-white">Trust Workspace for Command Execution?</h3>
      <div id="trust-workspace" class="text-[11px] font-mono text-slate-300 truncate"></div>
      <ul class="text-xs text-slate-300 leading-relaxed space-y-1.5 list-disc pl-5">
        <li>APW will run project install, build, and test commands inside an isolated worktree under your user account.</li>
        <li>Only trust repositories you recognize. Untrusted workspaces remain safe to use in Plan, Review, Preview, and Unverified modes.</li>
      </ul>
      <p id="trust-sandbox-hint" class="hidden text-[11px] text-amber-300 leading-relaxed">
        Sandbox mode is currently off. Consider setting <code class="text-white">"sandbox": {"mode": "restricted"}</code> in apw.json before trusting.
      </p>
      <div class="flex items-center justify-end gap-2 pt-1">
        <button id="trust-cancel" onclick="closeTrustDialog(false)" class="px-3.5 py-1.5 rounded-lg text-xs font-medium bg-white/[0.06] hover:bg-white/[0.12] text-slate-200 transition">Keep Safe Mode</button>
        <button id="trust-confirm" onclick="closeTrustDialog(true)" class="px-3.5 py-1.5 rounded-lg text-xs font-semibold bg-white text-black hover:opacity-90 transition">Trust Workspace</button>
      </div>
    </div>
  </div>

  <!-- SETTINGS & MODEL SWITCHER DIALOG -->
  <div id="settings-dialog" class="hidden fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4 select-text" role="dialog" aria-modal="true" aria-labelledby="settings-title">
    <div class="squircle bg-[#18181c] border border-white/[0.12] shadow-2xl w-full max-w-lg p-5 space-y-4">
      <div class="flex items-center justify-between border-b border-white/[0.07] pb-3">
        <div>
          <h3 id="settings-title" class="text-sm font-semibold text-white">Models &amp; Runtime Configuration (apw.json)</h3>
          <p class="text-[11px] text-slate-400 mt-0.5">Configure System 2 (Architect), System 1 (Reflex), default approval mode, and OS sandbox.</p>
        </div>
        <button onclick="closeSettingsDialog()" class="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.06] transition">✕</button>
      </div>

      <div class="space-y-3 text-xs">
        <div class="space-y-1">
          <label class="block text-[11px] font-mono text-slate-300">System 2 — Architect Model (Planning &amp; Deep Reasoning)</label>
          <select id="setting-architect" class="w-full bg-[#111114] text-slate-200 font-mono text-xs border border-white/[0.1] rounded-lg px-3 py-2 focus:outline-none"></select>
        </div>

        <div class="space-y-1">
          <label class="block text-[11px] font-mono text-slate-300">System 1 — Reflex Model (Fast Repair &amp; Verification)</label>
          <select id="setting-reflex" class="w-full bg-[#111114] text-slate-200 font-mono text-xs border border-white/[0.1] rounded-lg px-3 py-2 focus:outline-none"></select>
        </div>

        <div class="space-y-1">
          <label class="block text-[11px] font-mono text-slate-300">Default Approval Mode</label>
          <select id="setting-approval" class="w-full bg-[#111114] text-slate-200 font-mono text-xs border border-white/[0.1] rounded-lg px-3 py-2 focus:outline-none">
            <option value="ask">ask — verify in worktree and ask before apply</option>
            <option value="auto">auto — verify and auto-apply when passing</option>
            <option value="plan">plan — generate execution plan only</option>
            <option value="unverified">unverified — edit without running build/test</option>
          </select>
        </div>

        <div class="space-y-1">
          <label class="block text-[11px] font-mono text-slate-300">Verification Sandbox Mode</label>
          <select id="setting-sandbox" class="w-full bg-[#111114] text-slate-200 font-mono text-xs border border-white/[0.1] rounded-lg px-3 py-2 focus:outline-none">
            <option value="off">off — standard user process</option>
            <option value="restricted">restricted — OS Job Object / Low-Integrity sandbox</option>
          </select>
        </div>
      </div>

      <div id="settings-error" class="hidden text-xs text-rose-300 font-mono bg-rose-500/15 border border-rose-500/30 rounded-lg p-2.5"></div>

      <div class="flex items-center justify-end gap-2 pt-2 border-t border-white/[0.07]">
        <button onclick="closeSettingsDialog()" class="px-3.5 py-1.5 rounded-lg text-xs font-medium bg-white/[0.06] hover:bg-white/[0.12] text-slate-200 transition">Cancel</button>
        <button onclick="saveSettingsDialog()" class="px-4 py-1.5 rounded-lg text-xs font-semibold bg-white text-black hover:opacity-90 transition">Save Changes</button>
      </div>
    </div>
  </div>

  <!-- ===================================================================== -->
  <!-- 24/7 AUTONOMY, GOAL BACKLOG, SCHEDULES & PROCEDURAL MEMORY DIALOG     -->
  <!-- ===================================================================== -->
  <div id="autonomy-dialog" class="hidden fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="w-full max-w-3xl max-h-[88vh] flex flex-col squircle bg-[#18181c] border border-white/[0.12] shadow-2xl overflow-hidden select-text">
      <div class="p-4 border-b border-white/[0.08] flex items-center justify-between bg-[#121216]">
        <div>
          <span class="text-[10.5px] font-mono uppercase tracking-wider text-slate-400">24/7 Autonomous Runtime &amp; Procedural Memory</span>
          <h3 class="text-sm font-semibold text-white mt-0.5">Automations, Scheduled Jobs &amp; Workspace Skills</h3>
        </div>
        <button onclick="closeAutonomyDialog()" class="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-white/[0.08] transition" title="Close (Esc)">✕</button>
      </div>

      <div class="flex-1 overflow-y-auto p-5 space-y-5 text-xs">
        <!-- Circuit Breaker Governor Banner -->
        <div class="squircle-sm bg-[#121216] border border-white/[0.08] p-3.5 flex flex-wrap items-center justify-between gap-3">
          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-white">Safety Governor (Circuit Breaker):</span>
              <span id="gov-status-badge" class="font-mono text-[10.5px] px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300">ACTIVE</span>
            </div>
            <p id="gov-reason-text" class="text-[11px] text-slate-400 font-mono">Runs strictly in isolated git worktrees and trips automatically on consecutive failures.</p>
          </div>
          <div class="flex items-center gap-2">
            <button id="btn-gov-toggle" onclick="toggleGovernorPause()" class="px-3 py-1.5 rounded-lg font-mono text-xs font-medium bg-white/[0.06] hover:bg-white/[0.12] text-slate-200 border border-white/[0.1] transition">Pause</button>
          </div>
        </div>

        <!-- Goal Backlog Section -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-slate-200 font-mono">Persistent Goal Backlog</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <input id="goal-title-input" type="text" placeholder="Goal title (e.g. Add unit tests for store)" class="flex-1 min-w-[180px] bg-[#121216] text-slate-200 text-xs border border-white/[0.1] rounded-lg px-3 py-1.5 focus:outline-none">
            <input id="goal-instr-input" type="text" placeholder="Detailed instruction..." class="flex-[2] min-w-[220px] bg-[#121216] text-slate-200 text-xs border border-white/[0.1] rounded-lg px-3 py-1.5 focus:outline-none">
            <button onclick="addAutonomyGoal()" class="px-3.5 py-1.5 rounded-lg bg-white text-black font-semibold text-xs hover:opacity-90 transition">+ Add Goal</button>
          </div>
          <div id="autonomy-goal-list" class="space-y-1.5 max-h-40 overflow-y-auto"></div>
        </div>

        <!-- Recurring Schedules Section -->
        <div class="space-y-2 pt-3 border-t border-white/[0.07]">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-slate-200 font-mono">Recurring Schedules (Cron)</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <input id="sched-name-input" type="text" placeholder="Schedule name (e.g. Audit &amp; Verify)" class="flex-1 min-w-[150px] bg-[#121216] text-slate-200 text-xs border border-white/[0.1] rounded-lg px-3 py-1.5 focus:outline-none">
            <input id="sched-instr-input" type="text" placeholder="Recurring instruction..." class="flex-[2] min-w-[200px] bg-[#121216] text-slate-200 text-xs border border-white/[0.1] rounded-lg px-3 py-1.5 focus:outline-none">
            <select id="sched-interval-input" class="bg-[#121216] text-slate-200 font-mono text-xs border border-white/[0.1] rounded-lg px-2.5 py-1.5 focus:outline-none">
              <option value="900">Every 15m</option>
              <option value="1800">Every 30m</option>
              <option value="3600" selected>Every 1h</option>
              <option value="21600">Every 6h</option>
              <option value="86400">Every 24h</option>
            </select>
            <button onclick="addAutonomySchedule()" class="px-3.5 py-1.5 rounded-lg bg-white/[0.08] hover:bg-white/[0.14] text-white border border-white/[0.12] font-medium text-xs transition">+ Add Schedule</button>
          </div>
          <div id="autonomy-schedule-list" class="space-y-1.5 max-h-40 overflow-y-auto"></div>
        </div>

        <!-- Episodic Memory & Procedural Skills Section -->
        <div class="space-y-2 pt-3 border-t border-white/[0.07]">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-slate-200 font-mono">Episodic Memory &amp; Learned Skills (.apw/skills)</span>
            <span id="autonomy-skill-count" class="text-[11px] font-mono text-slate-400">0 skills</span>
          </div>
          <div id="autonomy-episode-list" class="space-y-1.5 max-h-44 overflow-y-auto"></div>
        </div>
      </div>

      <div class="p-3.5 border-t border-white/[0.08] bg-[#121216] flex items-center justify-end">
        <button onclick="closeAutonomyDialog()" class="px-4 py-1.5 rounded-lg text-xs font-medium bg-white/[0.08] hover:bg-white/[0.14] text-white transition">Done</button>
      </div>
    </div>
  </div>

<script>
const apiToken = "__APW_TOKEN__";

/* Per-viewer conveniences live in localStorage; every access is guarded because it can throw. */
function lsGet(key, fallback) {
  try { const v = window.localStorage.getItem(key); return v === null ? fallback : v; }
  catch (e) { return fallback; }
}
function lsSet(key, value) {
  try { window.localStorage.setItem(key, value); } catch (e) { /* storage unavailable */ }
}

/* Small DOM helper: every dynamic value goes through textContent, never through HTML strings. */
function mk(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = String(text);
  return node;
}
function byId(id) { return document.getElementById(id); }

let currentSessionId = lsGet("apw_desktop_session", "");
let selectedTaskId = lsGet("apw_desktop_task", "");
let leftSidebarOpen = true;
let rightPaneOpen = false;
let currentArtifactJob = null;
let currentDiffFilterFile = "";
let lastStatus = {};
let allLoadedTasks = [];
let approvalMode = lsGet("apw_desktop_mode", "ask");
const watchedTasks = new Set();
const applyingTasks = new Set();
const autoAppliedTasks = new Set();
const renderedApprovalIds = new Set();
const activeStates = new Set(["queued", "planning", "analyzing", "patching", "verifying", "applying"]);

const MODE_HINTS = {
  ask: "ask: สั่งงานแล้ว APW จะ verify (build/test) ใน worktree แยก และถามก่อน apply ลง Workspace",
  auto: "auto: verify แล้ว apply อัตโนมัติเมื่อผ่าน (งานโหมด unverified จะไม่ถูก apply อัตโนมัติ)",
  plan: "plan: วางแผนอย่างเดียว ไม่แก้ไฟล์และไม่รันคำสั่งของโปรเจกต์",
  unverified: "unverified: แก้โดยไม่รัน build/test — ผลยังไม่ได้ตรวจสอบ จะถามก่อน apply เสมอ"
};
const MODE_LABELS = { ask: "ASK", auto: "AUTO", plan: "PLAN", unverified: "UNVERIFIED", review: "REVIEW", preview: "PREVIEW" };

if (!MODE_HINTS[approvalMode]) approvalMode = "ask";
byId("active-session-badge").textContent = currentSessionId || "…";

async function apiFetch(path, options = {}) {
  if (!path.startsWith("/")) throw new Error("Local API path required");
  const headers = { ...(options.headers || {}), Authorization: "Bearer " + apiToken };
  if (options.method === "POST") headers["Content-Type"] = "application/json";
  const request = { ...options, headers };
  // Retry once only for reads: a POST that failed in transit may already have reached the server.
  try { return await window.fetch(path, request); }
  catch (error) {
    if ((options.method || "GET") !== "GET") throw error;
    return window.fetch(path, request);
  }
}

/* Reads a response body once and returns {ok, status, data, error, message}. */
async function readResponse(res) {
  let text = "";
  try { text = await res.text(); } catch (e) { text = ""; }
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch (e) { data = null; }
  const out = { ok: res.ok, status: res.status, data: res.ok ? data : null, error: "", message: "" };
  if (!res.ok) {
    out.error = (data && typeof data.error === "string") ? data.error : "";
    out.message = (data && data.message) || out.error || text.trim() || ("HTTP " + res.status);
  }
  return out;
}

/* POST helper. With retryTrust, a 409 "untrusted" opens the trust dialog and retries once if the user trusts. */
async function postAPI(path, body, retryTrust) {
  let retried = false;
  for (;;) {
    const out = await readResponse(await apiFetch(path, { method: "POST", body: JSON.stringify(body || {}) }));
    if (!out.ok && out.status === 409 && out.error === "untrusted" && retryTrust && !retried) {
      retried = true;
      if (await requestTrust()) continue;
    }
    return out;
  }
}

async function getAPI(path) {
  return readResponse(await apiFetch(path));
}

/* ============================================================================
 * 1. 3-PANE SPLIT VIEW & RESIZABLE PANELS
 * ============================================================================ */
function applySavedLeftWidth() {
  const sb = byId("left-sidebar");
  if (!leftSidebarOpen) {
    sb.style.width = "";
    return;
  }
  const saved = parseInt(lsGet("apw_desktop_left_w", "268"), 10);
  if (!isNaN(saved) && saved >= 230 && saved <= 520) {
    sb.style.width = saved + "px";
  }
}

function applySavedRightWidth() {
  const rp = byId("right-pane");
  if (!rightPaneOpen) {
    rp.style.width = "";
    return;
  }
  const saved = parseInt(lsGet("apw_desktop_right_w", "540"), 10);
  if (!isNaN(saved) && saved >= 360 && saved <= 900) {
    rp.style.width = saved + "px";
  }
}

function toggleLeftSidebar() {
  const sb = byId("left-sidebar");
  const showBtn = byId("btn-show-left");
  const resizer = byId("left-resizer");
  leftSidebarOpen = !leftSidebarOpen;
  if (leftSidebarOpen) {
    sb.classList.remove("w-0", "opacity-0", "overflow-hidden", "border-r-0");
    sb.classList.add("w-80");
    showBtn.classList.add("hidden");
    if (resizer) resizer.classList.remove("hidden");
    applySavedLeftWidth();
  } else {
    sb.style.width = "";
    sb.classList.remove("w-80");
    sb.classList.add("w-0", "opacity-0", "overflow-hidden", "border-r-0");
    showBtn.classList.remove("hidden");
    if (resizer) resizer.classList.add("hidden");
  }
}

function openRightPane(defaultTab) {
  const rp = byId("right-pane");
  const resizer = byId("right-resizer");
  rightPaneOpen = true;
  rp.classList.remove("w-0");
  rp.classList.add("w-[480px]", "xl:w-[560px]");
  if (resizer) resizer.classList.remove("hidden");
  applySavedRightWidth();
  if (defaultTab) switchArtifactTab(defaultTab);
}

function closeRightPane() {
  const rp = byId("right-pane");
  const resizer = byId("right-resizer");
  rightPaneOpen = false;
  rp.style.width = "";
  rp.classList.remove("w-[480px]", "xl:w-[560px]");
  rp.classList.add("w-0");
  if (resizer) resizer.classList.add("hidden");
}

function toggleRightPane() {
  if (rightPaneOpen) closeRightPane();
  else openRightPane();
}

function initResizablePanels() {
  const leftResizer = byId("left-resizer");
  const rightResizer = byId("right-resizer");
  applySavedLeftWidth();

  if (leftResizer) {
    leftResizer.addEventListener("dblclick", () => {
      lsSet("apw_desktop_left_w", "320");
      applySavedLeftWidth();
    });
    leftResizer.addEventListener("mousedown", (e) => {
      if (!leftSidebarOpen) return;
      e.preventDefault();
      leftResizer.classList.add("dragging");
      const onMove = (ev) => {
        const w = Math.max(240, Math.min(520, ev.clientX));
        byId("left-sidebar").style.width = w + "px";
        lsSet("apw_desktop_left_w", String(w));
      };
      const onUp = () => {
        leftResizer.classList.remove("dragging");
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    });
  }

  if (rightResizer) {
    rightResizer.addEventListener("dblclick", () => {
      lsSet("apw_desktop_right_w", "540");
      applySavedRightWidth();
    });
    rightResizer.addEventListener("mousedown", (e) => {
      if (!rightPaneOpen) return;
      e.preventDefault();
      rightResizer.classList.add("dragging");
      const onMove = (ev) => {
        const w = Math.max(360, Math.min(900, window.innerWidth - ev.clientX));
        byId("right-pane").style.width = w + "px";
        lsSet("apw_desktop_right_w", String(w));
      };
      const onUp = () => {
        rightResizer.classList.remove("dragging");
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };
      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    });
  }
}

function switchArtifactTab(tab) {
  ["diff", "code", "preview"].forEach(t => {
    const panel = byId("artifact-tab-" + t);
    const btn = byId("tab-btn-" + t);
    if (t === tab) {
      panel.classList.remove("hidden");
      btn.className = "px-2.5 py-1 rounded-md bg-white/[0.12] text-white transition";
    } else {
      panel.classList.add("hidden");
      btn.className = "px-2.5 py-1 rounded-md text-slate-400 hover:text-white transition";
    }
  });
}

document.addEventListener("keydown", (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "b") {
    e.preventDefault();
    toggleLeftSidebar();
  } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "j") {
    e.preventDefault();
    toggleRightPane();
  } else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    const input = byId("task-input");
    if (input) input.focus();
  } else if (e.key === "Escape") {
    if (!byId("trust-dialog").classList.contains("hidden")) {
      closeTrustDialog(false);
    }
    if (!byId("settings-dialog").classList.contains("hidden")) {
      closeSettingsDialog();
    }
  }
});

/* ============================================================================
 * 2. TRUST & SETTINGS / MODEL SWITCHER DIALOGS
 * ============================================================================ */
let trustResolver = null;

function showTrustDialog() {
  return new Promise(resolve => {
    if (trustResolver) trustResolver(false);
    trustResolver = resolve;
    byId("trust-workspace").textContent = lastStatus.workspace || "";
    byId("trust-sandbox-hint").classList.toggle("hidden", lastStatus.sandbox_mode !== "off");
    byId("trust-dialog").classList.remove("hidden");
    byId("trust-cancel").focus();
  });
}

function closeTrustDialog(result) {
  byId("trust-dialog").classList.add("hidden");
  const resolve = trustResolver;
  trustResolver = null;
  if (resolve) resolve(result);
}

async function setTrusted(value) {
  const out = await postAPI("/api/trust", { "trusted": value }, false);
  if (!out.ok) {
    appendCard("เปลี่ยนสถานะ trust ไม่สำเร็จ", out.message, "ERROR", "#f85149");
    return false;
  }
  await loadStatus();
  return true;
}

/* Asks the user; returns true only when the workspace is trusted afterwards. */
async function requestTrust() {
  if (!(await showTrustDialog())) return false;
  return setTrusted(true);
}

async function onTrustControl() {
  if (lastStatus.trusted) await setTrusted(false);
  else await requestTrust();
}

async function openSettingsDialog() {
  const errEl = byId("settings-error");
  errEl.classList.add("hidden");
  errEl.textContent = "";

  const out = await getAPI("/api/models");
  if (!out.ok) {
    appendCard("โหลดการตั้งค่า Model ไม่สำเร็จ", out.message, "ERROR", "#f85149");
    return;
  }
  const data = out.data || {};
  const providers = data.providers || [];
  const archSel = byId("setting-architect");
  const refSel = byId("setting-reflex");
  archSel.replaceChildren();
  refSel.replaceChildren();

  providers.forEach(p => {
    const label = p.key + " — " + p.model + " [" + p.type + "]";
    const optA = mk("option", "", label);
    optA.value = p.key;
    if (p.key === data.architect) optA.selected = true;
    archSel.appendChild(optA);

    const optR = mk("option", "", label);
    optR.value = p.key;
    if (p.key === data.reflex) optR.selected = true;
    refSel.appendChild(optR);
  });

  byId("setting-approval").value = data.approval || approvalMode || "ask";
  byId("setting-sandbox").value = data.sandbox_mode === "restricted" ? "restricted" : "off";
  byId("settings-dialog").classList.remove("hidden");
}

function closeSettingsDialog() {
  byId("settings-dialog").classList.add("hidden");
}

async function saveSettingsDialog() {
  const errEl = byId("settings-error");
  errEl.classList.add("hidden");
  const body = {
    architect: byId("setting-architect").value,
    reflex: byId("setting-reflex").value,
    approval: byId("setting-approval").value,
    sandbox_mode: byId("setting-sandbox").value
  };
  const out = await postAPI("/api/models", body, false);
  if (!out.ok) {
    errEl.textContent = out.message;
    errEl.classList.remove("hidden");
    return;
  }
  if (body.approval && MODE_HINTS[body.approval]) {
    onApprovalModeChange(body.approval);
  }
  closeSettingsDialog();
  await loadStatus();
  appendCollapsibleToolCard("ModelSettings", "บันทึกการตั้งค่า Models & Sandbox เรียบร้อย",
    "Architect: " + body.architect + " · Reflex: " + body.reflex + " · Approval: " + body.approval + " · Sandbox: " + body.sandbox_mode, "DONE");
}

/* ============================================================================
 * 3. APPROVAL MODES
 * ============================================================================ */
function applyModeUI() {
  byId("approval-mode").value = approvalMode;
  const desc = byId("mode-desc");
  desc.textContent = MODE_HINTS[approvalMode] || "";
  desc.className = "pt-1.5 text-[11px] leading-relaxed " + (approvalMode === "unverified" ? "text-amber-300" : "text-slate-400");
}

function onApprovalModeChange(value) {
  if (!MODE_HINTS[value]) return;
  approvalMode = value;
  lsSet("apw_desktop_mode", value);
  applyModeUI();
}

/* ============================================================================
 * 4. HUMAN-IN-THE-LOOP APPROVAL CARDS (Apply requests)
 * ============================================================================ */
let approvalTimer = null;
let approvalPolling = false;

function startApprovalPolling() {
  if (approvalTimer) return;
  approvalTimer = setInterval(pollApprovals, 600);
  pollApprovals();
}

function stopApprovalPolling() {
  if (approvalTimer) clearInterval(approvalTimer);
  approvalTimer = null;
}

/* Polls only while a task is running or an Apply is waiting for a decision. */
async function pollApprovals() {
  if (approvalPolling) return;
  approvalPolling = true;
  try {
    const pending = await loadApprovals();
    if (pending === 0 && watchedTasks.size === 0 && applyingTasks.size === 0) stopApprovalPolling();
  } finally {
    approvalPolling = false;
  }
}

function closeApprovalCard(card, label, ok) {
  const statusBadge = card.querySelector(".appr-status-badge");
  const btnRow = card.querySelector(".appr-btn-row");
  card.dataset.pending = "0";
  if (statusBadge) {
    statusBadge.textContent = label;
    statusBadge.className = "appr-status-badge text-[11px] font-mono px-2.5 py-0.5 rounded-full " +
      (ok ? "bg-emerald-500/20 text-emerald-300 border border-emerald-500/30" : "bg-rose-500/20 text-rose-300 border border-rose-500/30");
  }
  if (btnRow) btnRow.remove();
}

function renderApprovalCard(req) {
  const existing = byId("appr-card-" + req.id);
  if (req.status !== "pending") {
    if (existing) {
      const label = req.status === "approved_always" ? "✓ อนุญาตเสมอแล้ว" :
                    req.status === "approved_once" ? "✓ อนุญาตครั้งเดียวแล้ว" : "✗ ปฏิเสธแล้ว";
      closeApprovalCard(existing, label, req.status.startsWith("approved"));
    }
    return;
  }
  if (renderedApprovalIds.has(req.id)) return;
  renderedApprovalIds.add(req.id);

  const isApply = String(req.tool_name || "").toLowerCase() === "apply";
  const unverified = /UNVERIFIED/i.test(req.description || "");
  const container = byId("approval-feed");
  const card = mk("div", "squircle bg-[#1A1714] border-2 p-4 shadow-2xl space-y-3 " +
    (unverified ? "border-rose-500/60" : "border-amber-500/40"));
  card.id = "appr-card-" + req.id;
  card.dataset.pending = "1";
  card.dataset.reqId = req.id;

  const header = mk("div", "flex items-center justify-between gap-2");
  const title = mk("div", "flex items-center gap-2");
  title.append(
    mk("span", "w-2.5 h-2.5 rounded-full bg-amber-400 pulse-dot"),
    mk("span", "text-xs font-semibold text-amber-300 uppercase tracking-wider",
      isApply ? "ขออนุมัติ Apply ลง Workspace" : "ขออนุมัติ / Approval")
  );
  header.append(title, mk("span", "appr-status-badge text-[11px] font-mono px-2.5 py-0.5 rounded-full bg-amber-500/20 text-amber-300 border border-amber-500/30", "รอการยืนยัน"));

  const details = mk("div", "squircle-sm bg-[#0E1015] border border-white/[0.07] p-3 font-mono text-xs space-y-1.5");
  const toolLine = mk("div");
  toolLine.append(mk("span", "text-slate-400", "Tool: "), mk("span", "text-sky-300 font-semibold", req.tool_name || "SystemCommand"));
  details.appendChild(toolLine);
  if (req.task_id) {
    const taskLine = mk("div");
    taskLine.append(mk("span", "text-slate-400", "Task: "), mk("span", "text-slate-200", req.task_id));
    details.appendChild(taskLine);
  }
  if (req.command) {
    const cmdLine = mk("div");
    cmdLine.append(mk("span", "text-slate-400", "Command: "), mk("code", "text-amber-200 bg-white/[0.04] px-1.5 py-0.5 rounded", req.command));
    details.appendChild(cmdLine);
  }
  if (req.target_path) {
    const pathLine = mk("div");
    pathLine.append(mk("span", "text-slate-400", "Target: "), mk("span", "text-slate-200", req.target_path));
    details.appendChild(pathLine);
  }
  if (unverified) {
    details.appendChild(mk("div", "text-rose-300 font-sans font-semibold text-[11px] pt-1",
      "⚠ UNVERIFIED — การแก้ไขนี้ยังไม่ได้ผ่านการรัน build/test"));
  }
  if (req.description) {
    details.appendChild(mk("div", "text-slate-400 font-sans text-[11px] pt-1 whitespace-pre-wrap", req.description));
  }

  const btnRow = mk("div", "appr-btn-row flex flex-wrap items-center gap-2 pt-1");
  const choices = [
    { key: "once", label: "อนุญาตครั้งเดียว", cls: "bg-emerald-600 hover:bg-emerald-500 text-white" },
    { key: "always", label: isApply ? "อนุญาต Apply เสมอใน Desktop นี้" : "อนุญาตเสมอใน Desktop นี้", cls: "bg-sky-600 hover:bg-sky-500 text-white" },
    { key: "deny", label: "ปฏิเสธ", cls: "bg-rose-600/25 hover:bg-rose-600/40 text-rose-200 border border-rose-500/40" }
  ];
  choices.forEach(c => {
    const btn = mk("button", "px-3.5 py-1.5 rounded-xl text-xs font-semibold transition " + c.cls, c.label);
    btn.onclick = () => submitApprovalDecision(req.id, c.key);
    btnRow.appendChild(btn);
  });

  card.append(header, details, btnRow);
  container.appendChild(card);

  const feed = byId("feed");
  feed.scrollTop = feed.scrollHeight;
}

async function submitApprovalDecision(id, decision) {
  const out = await postAPI("/api/approvals/" + encodeURIComponent(id) + "/decide", { decision }, false);
  if (!out.ok) {
    appendCard("การส่งคำยืนยันล้มเหลว", out.message, "ERROR", "#f85149");
    return;
  }
  renderApprovalCard(out.data);
  loadApprovals();
}

/* Returns the number of pending approvals (0 when the request fails). */
async function loadApprovals() {
  let res;
  try { res = await apiFetch("/api/approvals"); } catch (e) { return 0; }
  if (!res.ok) return 0;
  const data = await res.json();
  const pending = data.pending || [];
  const pendingIds = new Set(pending.map(r => r.id));
  pending.forEach(renderApprovalCard);
  document.querySelectorAll("#approval-feed [data-pending='1']").forEach(card => {
    if (!pendingIds.has(card.dataset.reqId)) closeApprovalCard(card, "สิ้นสุดแล้ว (ไม่ได้รอคำตอบ)", false);
  });
  const allowed = (data.always_allowed || []).map(x => String(x).toLowerCase());
  const statusEl = byId("apply-auto-status");
  if (statusEl) {
    const on = allowed.includes("apply");
    statusEl.textContent = on ? "เปิด" : "ปิด";
    statusEl.className = on ? "text-emerald-300" : "text-amber-300";
  }
  return pending.length;
}

/* ============================================================================
 * 5. COLLAPSIBLE TOOL CARDS & DETERMINISTIC MESSAGE RENDERER
 * ============================================================================ */
function clearLiveFeed() {
  const live = byId("live-feed");
  if (live) live.replaceChildren();
}

function appendCollapsibleToolCard(toolName, summaryText, detailBody, status, CardID) {
  const scroller = byId("feed");
  const container = byId("live-feed") || scroller;
  let detailsEl = CardID ? byId(CardID) : null;
  const isNew = !detailsEl;

  if (isNew) {
    detailsEl = document.createElement("details");
    if (CardID) detailsEl.id = CardID;
    detailsEl.className = "ent-tool-row group transition";
  }

  const badgeColors = {
    RUNNING: "text-amber-300",
    DONE: "text-emerald-300",
    DENIED: "text-rose-300",
    ERROR: "text-rose-300"
  };
  const badgeCls = badgeColors[status] || "text-slate-400";
  const dotCls = status === "RUNNING" ? "bg-amber-400 pulse-dot" : status === "DONE" ? "bg-emerald-400" : "bg-rose-400";

  const summary = mk("summary", "px-3.5 py-2 cursor-pointer flex items-center justify-between gap-3 hover:bg-white/[0.03] select-none list-none");
  const left = mk("div", "flex items-center gap-2 min-w-0");
  left.append(
    mk("span", "w-1.5 h-1.5 rounded-full shrink-0 " + dotCls),
    mk("span", "text-[11px] font-mono text-slate-400 shrink-0", toolName),
    mk("span", "text-xs text-slate-200 truncate tool-summary", summaryText)
  );
  const right = mk("div", "flex items-center gap-2 shrink-0");
  right.append(
    mk("span", "text-[10px] font-mono " + badgeCls, status),
    mk("span", "text-slate-500 text-xs group-open:rotate-180 transition-transform", "▾")
  );
  summary.append(left, right);
  const detail = mk("div", "px-3.5 py-2.5 border-t border-white/[0.06] bg-[#0c0c0e] font-mono text-xs text-slate-300 whitespace-pre-wrap leading-relaxed tool-detail",
    typeof detailBody === "string" ? detailBody : JSON.stringify(detailBody, null, 2));
  detailsEl.replaceChildren(summary, detail);

  if (isNew) {
    container.appendChild(detailsEl);
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
  }
  return detailsEl;
}

function appendStreamingBubble(role, title, text, badgeText) {
  const scroller = byId("feed");
  const container = byId("live-feed") || scroller;
  const card = document.createElement("div");
  const isUser = role === "user";
  card.className = isUser ? "ent-user-bubble" : "ent-agent-bubble";

  if (!isUser) {
    const header = document.createElement("div");
    header.className = "flex items-center justify-between mb-1.5";
    const left = document.createElement("span");
    left.className = "text-[11px] font-mono font-semibold text-slate-400";
    left.textContent = title || "APW Agent";
    header.appendChild(left);
    if (badgeText) {
      const b = document.createElement("span");
      b.className = "text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.06] text-slate-400";
      b.textContent = badgeText;
      header.appendChild(b);
    }
    card.appendChild(header);
  }

  const bodyEl = document.createElement("div");
  bodyEl.className = "text-xs text-slate-100 whitespace-pre-wrap leading-relaxed font-sans";
  const full = typeof text === "string" ? text : JSON.stringify(text, null, 2);
  bodyEl.textContent = full;

  card.appendChild(bodyEl);
  container.appendChild(card);
  if (scroller) scroller.scrollTop = scroller.scrollHeight;
}

function appendCard(title, body, badgeText, badgeColor) {
  const scroller = byId("feed");
  const container = byId("live-feed") || scroller;
  const card = document.createElement("div");
  card.className = "ent-tool-row p-3.5 space-y-2";
  const h = document.createElement("div");
  h.className = "flex items-center justify-between gap-2";

  const tSpan = document.createElement("span");
  tSpan.className = "text-xs font-medium text-white font-mono";
  tSpan.textContent = title;
  h.appendChild(tSpan);

  if (badgeText) {
    const bSpan = document.createElement("span");
    bSpan.textContent = badgeText;
    bSpan.className = "text-[10px] font-mono px-2 py-0.5 rounded text-white";
    bSpan.style.background = badgeColor || "#27272a";
    h.appendChild(bSpan);
  }

  const pre = document.createElement("pre");
  pre.className = "text-xs font-mono text-slate-300 whitespace-pre-wrap overflow-x-auto leading-relaxed bg-[#0c0c0e] p-2.5 squircle-sm border border-white/[0.05]";
  pre.textContent = typeof body === "string" ? body : JSON.stringify(body, null, 2);
  card.appendChild(h);
  card.appendChild(pre);
  container.appendChild(card);
  if (scroller) scroller.scrollTop = scroller.scrollHeight;
}

/* ============================================================================
 * 6. RIGHT ARTIFACT / DIFF RENDERER (Per-File Diff Table, Structured Evidence)
 * ============================================================================ */
function parseUnifiedDiffByFile(diffText) {
  const sections = [];
  if (!diffText || !diffText.trim()) return sections;
  const lines = diffText.split("\n");
  let current = null;

  function ensureSection(fileName) {
    if (!current || (fileName && current.file !== fileName && current.lines.length > 0)) {
      current = { file: fileName || "changes.diff", added: 0, deleted: 0, lines: [] };
      sections.push(current);
    } else if (fileName && current.file === "changes.diff") {
      current.file = fileName;
    }
    return current;
  }

  lines.forEach(line => {
    if (line.startsWith("diff --git ")) {
      const m = line.match(/^diff --git a\/(.+?) b\/(.+)$/);
      const file = m ? m[2] : line.slice(11).trim();
      current = { file: file, added: 0, deleted: 0, lines: [line] };
      sections.push(current);
      return;
    }
    if (line.startsWith("+++ b/")) {
      const f = line.slice(6).trim();
      const sec = ensureSection(f);
      sec.file = f;
      sec.lines.push(line);
      return;
    }
    const sec = ensureSection("");
    if (line.startsWith("+") && !line.startsWith("+++")) sec.added++;
    else if (line.startsWith("-") && !line.startsWith("---")) sec.deleted++;
    sec.lines.push(line);
  });

  return sections;
}

function renderDiffTable(diffText, filterFile) {
  const tbody = byId("diff-tbody");
  const emptyState = byId("diff-empty-state");
  const tableContainer = byId("diff-table-container");
  tbody.replaceChildren();

  if (!diffText || !diffText.trim()) {
    emptyState.classList.remove("hidden");
    tableContainer.classList.add("hidden");
    return;
  }

  emptyState.classList.add("hidden");
  tableContainer.classList.remove("hidden");

  const sections = parseUnifiedDiffByFile(diffText);
  const visible = filterFile ? sections.filter(s => s.file === filterFile) : sections;
  const listToRender = visible.length > 0 ? visible : sections;

  listToRender.forEach(sec => {
    // Per-file header row
    const headerTr = mk("tr", "diff-file-row");
    const headerTd = document.createElement("td");
    headerTd.colSpan = 3;
    headerTd.className = "px-3 py-2";
    const bar = mk("div", "flex items-center justify-between gap-2");
    const left = mk("div", "flex items-center gap-2 min-w-0");
    left.append(
      mk("span", "text-xs font-mono font-semibold text-white truncate", sec.file),
      mk("span", "text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300", "+" + sec.added),
      mk("span", "text-[10px] font-mono px-1.5 py-0.5 rounded bg-rose-500/20 text-rose-300", "-" + sec.deleted)
    );
    const viewBtn = mk("button", "text-[11px] font-mono text-sky-400 hover:underline shrink-0", "ดูโค้ดเต็ม →");
    viewBtn.onclick = () => onSelectArtifactFile(sec.file);
    bar.append(left, viewBtn);
    headerTd.appendChild(bar);
    headerTr.appendChild(headerTd);
    tbody.appendChild(headerTr);

    let oldLine = 0, newLine = 0;
    sec.lines.forEach(line => {
      const tr = document.createElement("tr");
      const tdOld = document.createElement("td");
      const tdNew = document.createElement("td");
      const tdCode = document.createElement("td");
      tdOld.className = "diff-num";
      tdNew.className = "diff-num";
      tdCode.className = "diff-code";

      if (line.startsWith("@@")) {
        tr.className = "diff-line-hunk";
        const m = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
        if (m) {
          oldLine = parseInt(m[1], 10);
          newLine = parseInt(m[2], 10);
        }
        tdOld.textContent = "···";
        tdNew.textContent = "···";
      } else if (line.startsWith("+") && !line.startsWith("+++")) {
        tr.className = "diff-line-add";
        tdOld.textContent = "";
        tdNew.textContent = newLine > 0 ? String(newLine++) : "+";
      } else if (line.startsWith("-") && !line.startsWith("---")) {
        tr.className = "diff-line-del";
        tdOld.textContent = oldLine > 0 ? String(oldLine++) : "-";
        tdNew.textContent = "";
      } else if (line.startsWith("diff --git") || line.startsWith("index ") || line.startsWith("--- ") || line.startsWith("+++ ")) {
        tdOld.textContent = "";
        tdNew.textContent = "";
      } else {
        if (oldLine > 0) oldLine++;
        if (newLine > 0) newLine++;
        tdOld.textContent = oldLine > 0 ? String(oldLine - 1) : "";
        tdNew.textContent = newLine > 0 ? String(newLine - 1) : "";
      }

      tdCode.textContent = line;
      tr.append(tdOld, tdNew, tdCode);
      tbody.appendChild(tr);
    });
  });
}

function collectJobFiles(job) {
  const report = (job && job.report) || {};
  const plan = report.plan || {};
  const out = [];
  const seen = new Set();
  const add = (f) => {
    if (f && typeof f === "string" && !seen.has(f)) {
      seen.add(f);
      out.push(f);
    }
  };
  (plan.affected_files || []).forEach(add);
  (report.modified_files || []).forEach(add);
  return out;
}

function renderAcceptanceEvidence(job) {
  const report = (job && job.report) || {};
  const accBox = byId("acceptance-box");
  const accStructured = byId("acceptance-structured");
  const accPre = byId("acceptance-pre");
  const accBadge = byId("acceptance-badge");
  accStructured.replaceChildren();

  const hasAcc = !!report.acceptance;
  const hasAttempts = Array.isArray(report.reflex_attempts) && report.reflex_attempts.length > 0;
  const hasWarnings = Array.isArray(report.warnings) && report.warnings.length > 0;
  const hasDiag = !!report.diagnostic_comparison;

  if (!hasAcc && !hasAttempts && !hasWarnings && !hasDiag) {
    accBox.classList.add("hidden");
    accPre.textContent = "";
    return;
  }

  accBox.classList.remove("hidden");
  const unverified = job.mode === "unverified" || report.unverified;
  const passed = report.passed && !unverified;
  accBadge.textContent = unverified ? "UNVERIFIED" : (passed ? "PASSED" : "CHECKED");
  accBadge.className = "text-[10px] font-mono px-2 py-0.5 rounded-full border " +
    (unverified ? "bg-amber-500/20 text-amber-300 border-amber-500/30" :
     passed ? "bg-emerald-500/20 text-emerald-300 border-emerald-500/30" :
     "bg-rose-500/20 text-rose-300 border-rose-500/30");

  // 1. Reflex Verification Attempts
  if (hasAttempts) {
    const sec = mk("div", "space-y-1");
    sec.appendChild(mk("div", "text-[11px] font-mono font-semibold text-slate-300", "Reflex Verification Stages:"));
    report.reflex_attempts.forEach(a => {
      const ok = a.Result && a.Result.passed;
      const row = mk("div", "flex items-center justify-between text-[11px] font-mono px-2.5 py-1 rounded bg-[#0E1117] border border-white/[0.05]");
      row.append(
        mk("span", ok ? "text-emerald-300" : "text-rose-300",
          (ok ? "✓ " : "✗ ") + "รอบที่ " + a.Number + (ok ? " — ผ่านทุกขั้นตอน" : " — ไม่ผ่านที่ " + (a.Result ? a.Result.failed_stage : "unknown"))),
        mk("span", "text-slate-400", formatNs(a.Result ? a.Result.duration : 0))
      );
      sec.appendChild(row);
    });
    accStructured.appendChild(sec);
  }

  // 2. Acceptance Checks
  const accList = Array.isArray(report.acceptance) ? report.acceptance :
    (report.acceptance && Array.isArray(report.acceptance.checks) ? report.acceptance.checks : []);
  if (accList.length > 0) {
    const sec = mk("div", "space-y-1");
    sec.appendChild(mk("div", "text-[11px] font-mono font-semibold text-slate-300", "Acceptance Criteria Checks:"));
    accList.forEach(chk => {
      const ok = !!chk.passed;
      const label = chk.criterion || chk.name || chk.kind || "check";
      const detail = chk.evidence || chk.detail || "";
      const row = mk("div", "text-[11px] font-mono px-2.5 py-1 rounded bg-[#0E1117] border border-white/[0.05] " + (ok ? "text-emerald-300" : "text-rose-300"));
      row.textContent = (ok ? "✓ " : "✗ ") + label + (detail ? " — " + detail : "");
      sec.appendChild(row);
    });
    accStructured.appendChild(sec);
  }

  // 3. Warnings
  if (hasWarnings) {
    const sec = mk("div", "space-y-1");
    report.warnings.forEach(w => {
      sec.appendChild(mk("div", "text-[11px] font-mono text-amber-300 bg-amber-500/10 border border-amber-500/30 rounded px-2.5 py-1", "⚠ " + w));
    });
    accStructured.appendChild(sec);
  }

  if (report.acceptance) {
    accPre.textContent = typeof report.acceptance === "string" ? report.acceptance : JSON.stringify(report.acceptance, null, 2);
    accPre.classList.remove("hidden");
  } else {
    accPre.textContent = "";
    accPre.classList.add("hidden");
  }
}

function showConflictBanner(job, errText) {
  const box = byId("artifact-conflict-box");
  const msg = byId("artifact-conflict-msg");
  const btn = byId("btn-conflict-resume");
  if (!errText) {
    box.classList.add("hidden");
    msg.textContent = "";
    return;
  }
  box.classList.remove("hidden");
  msg.textContent = errText;
  btn.onclick = async () => {
    if (!job || !job.id) return;
    const out = await postAPI("/v1/tasks/" + encodeURIComponent(job.id) + "/resume", {}, true);
    if (!out.ok) {
      appendCard("Resume ไม่สำเร็จ", out.message, "ERROR", "#f85149");
      return;
    }
    box.classList.add("hidden");
    watchTask(out.data);
  };
}

function createTaskActionButtons(job) {
  const unverifiedJob = job.mode === "unverified";
  const actions = activeStates.has(job.status) ? ["cancel"] :
    job.status === "ready_to_apply" ? ["apply", "resume"] :
    ["preview_ready", "failed", "interrupted", "cancelled"].includes(job.status) ? ["resume"] : [];

  return actions.map(action => {
    const b = document.createElement("button");
    b.className = "px-3 py-1.5 rounded-xl text-xs font-semibold transition " +
      (action === "apply" ? "bg-emerald-600 hover:bg-emerald-500 text-white" :
       action === "resume" ? "bg-amber-600 hover:bg-amber-500 text-white" :
       "bg-rose-600/30 hover:bg-rose-600/50 text-rose-200");
    b.textContent = action === "apply" ? (unverifiedJob ? "✓ Apply (UNVERIFIED) ลง Workspace" : "✓ Apply ลง Workspace") :
      action === "resume" ? "↻ Resume & Verify" : "✕ Cancel";
    b.onclick = async (ev) => {
      ev.stopPropagation();
      if (action === "apply") { await applyTask(job); return; }
      const out = await postAPI("/v1/tasks/" + encodeURIComponent(job.id) + "/" + action, {}, action === "resume");
      if (!out.ok) { appendCard("Task action failed", out.message, "ERROR", "#f85149"); return; }
      watchTask(out.data);
    };
    return b;
  });
}

function populateArtifactFromJob(job) {
  if (!job) return;
  currentArtifactJob = job;
  selectedTaskId = job.id;
  lsSet("apw_desktop_task", job.id);

  const report = job.report || {};
  const files = collectJobFiles(job);
  const unverifiedJob = job.mode === "unverified";

  byId("artifact-title").textContent = "Task " + job.id + " (" + job.status + ")";
  byId("artifact-status-pill").textContent = job.status.toUpperCase() + (unverifiedJob ? " · UNVERIFIED" : "");
  byId("artifact-task-id").textContent = job.id + " · " + job.status + (unverifiedJob ? " · UNVERIFIED" : "");

  const select = byId("artifact-file-select");
  select.replaceChildren();
  const defaultOpt = document.createElement("option");
  defaultOpt.value = "";
  defaultOpt.textContent = files.length ? "ไฟล์ที่เกี่ยวข้อง (" + files.length + " ไฟล์)" : "(เลือกไฟล์)";
  select.appendChild(defaultOpt);
  files.forEach(f => {
    const opt = document.createElement("option");
    opt.value = f;
    opt.textContent = f;
    select.appendChild(opt);
  });

  // File filter chips for Diff view
  const chipsWrap = byId("diff-file-chips");
  chipsWrap.replaceChildren();
  if (files.length > 0 && report.diff) {
    chipsWrap.classList.remove("hidden");
    const allChip = mk("button", "text-[11px] font-mono px-2 py-0.5 rounded-lg border transition " +
      (!currentDiffFilterFile ? "bg-[#E07A5F]/20 text-[#E07A5F] border-[#E07A5F]/40" : "bg-[#11141B] text-slate-400 border-white/[0.07] hover:text-white"),
      "ทุกไฟล์ (" + files.length + ")");
    allChip.onclick = () => {
      currentDiffFilterFile = "";
      populateArtifactFromJob(currentArtifactJob);
    };
    chipsWrap.appendChild(allChip);

    files.forEach(f => {
      const active = currentDiffFilterFile === f;
      const chip = mk("button", "text-[11px] font-mono px-2 py-0.5 rounded-lg border transition truncate max-w-[200px] " +
        (active ? "bg-[#E07A5F]/20 text-[#E07A5F] border-[#E07A5F]/40" : "bg-[#11141B] text-slate-300 border-white/[0.07] hover:text-white"),
        f);
      chip.onclick = () => {
        currentDiffFilterFile = f;
        renderDiffTable(report.diff || "", f);
        populateArtifactFromJob(currentArtifactJob);
      };
      chipsWrap.appendChild(chip);
    });
  } else {
    chipsWrap.classList.add("hidden");
  }

  byId("artifact-count-badge").textContent = String(files.length || (report.diff ? 1 : 0));

  renderDiffTable(report.diff || "", currentDiffFilterFile);
  renderAcceptanceEvidence(job);

  // Detect workspace conflict / hash drift on failed apply
  if (job.error && /conflict|changed since|modified|drift|hash/i.test(job.error)) {
    showConflictBanner(job, job.error);
  } else {
    showConflictBanner(job, "");
  }

  // Populate action buttons in Right Pane footer
  const btnWrap = byId("artifact-buttons");
  btnWrap.replaceChildren();
  createTaskActionButtons(job).forEach(b => btnWrap.appendChild(b));

  // Auto-slide-out Right Pane when Diff or affected files exist
  if (report.diff || files.length > 0) {
    openRightPane("diff");
  }
}

function showCodeViewer(filename, content, meta) {
  byId("code-viewer-filename").textContent = filename;
  byId("code-viewer-meta").textContent = meta || "";
  byId("code-viewer-pre").textContent = content;
}

async function onSelectArtifactFile(relPath) {
  if (!relPath) return;
  let url = "/api/file?path=" + encodeURIComponent(relPath);
  if (currentArtifactJob && currentArtifactJob.id) url += "&task=" + encodeURIComponent(currentArtifactJob.id);
  const out = await getAPI(url);
  if (!out.ok) {
    let msg = out.message;
    if (out.status === 403 || out.error === "protected") msg = "ไฟล์นี้ถูกป้องกัน (protected) — APW ไม่แสดงไฟล์ระบบหรือไฟล์ลับ เช่น .git, .env, *.pem, *.key";
    else if (out.error === "worktree_gone") msg = "worktree ของงานนี้ถูกลบไปแล้ว (worktree_gone) — ไม่สามารถแสดงเวอร์ชันที่แก้ไขได้ ลองรันงานใหม่หรือเลือกดูไฟล์ใน Workspace";
    else if (out.status === 404) msg = "ไม่พบไฟล์นี้ — อาจเป็นไฟล์ใหม่ที่ยังไม่ถูกสร้าง หรือถูกลบแล้ว";
    showCodeViewer(relPath, msg, "ERROR");
    openRightPane("code");
    return;
  }
  const data = out.data || {};
  const notes = [];
  notes.push(data.source === "worktree" ? "worktree" : "workspace");
  if (data.truncated) notes.push("ตัดทอน (แสดงบางส่วน)");
  if (data.binary) notes.push("ไฟล์ binary");
  const content = data.binary ? "ไฟล์ binary — ไม่แสดงเนื้อหา" : (data.content || "");
  showCodeViewer(data.path || relPath, content, notes.join(" · "));
  if (!data.binary && (relPath.endsWith(".html") || relPath.endsWith(".htm"))) {
    byId("html-preview-frame").srcdoc = data.content || "";
    openRightPane("preview");
  } else {
    openRightPane("code");
  }
}

function renderCodeAsHTMLPreview() {
  byId("html-preview-frame").srcdoc = byId("code-viewer-pre").textContent;
  switchArtifactTab("preview");
}

function loadSampleHTMLPreview() {
  const sample = '<!DOCTYPE html><html><body style="background:#0f172a;color:#f8fafc;font-family:sans-serif;padding:24px;">' +
    '<div style="background:#1e293b;border:1px solid #334155;border-radius:16px;padding:20px;">' +
    '<h2 style="margin:0 0 8px;color:#38bdf8;">APW Live Artifact Preview</h2>' +
    '<p style="color:#94a3b8;font-size:14px;">เรนเดอร์ HTML Preview โดยตรงในแผงด้านขวา</p>' +
    '</div></body></html>';
  byId("html-preview-frame").srcdoc = sample;
  openRightPane("preview");
}

/* ============================================================================
 * 7. WORKSPACE, APW.MD (PROJECT INSTRUCTIONS), SESSIONS & RECENT TASKS SIDEBAR
 * ============================================================================ */
function setTrustBadge(trusted) {
  const trustEl = byId("ws-trust-badge");
  const dot = mk("span", "w-1.5 h-1.5 rounded-full " + (trusted ? "bg-emerald-400" : "bg-amber-400"));
  trustEl.className = "flex items-center gap-1.5 font-mono text-[10.5px] " + (trusted ? "text-emerald-400" : "text-amber-400");
  trustEl.replaceChildren(dot, document.createTextNode(trusted ? " Trusted" : " Safe Mode"));
  byId("btn-trust-toggle").textContent = trusted ? "Revoke" : "Trust...";
}

async function loadStatus() {
  const r = await apiFetch("/api/status");
  if (!r.ok) return;
  const s = await r.json();
  lastStatus = s;

  byId("hdr-version").textContent = "v" + s.version;
  byId("hdr-workspace").textContent = s.workspace;
  byId("hdr-workspace").title = s.workspace;
  byId("ws-stack-badge").textContent = s.stack || "Project";
  setTrustBadge(!!s.trusted);

  byId("pill-brain1").textContent = "Reflex · " + (s.brain1_model || s.brain1_name);
  byId("pill-brain2").textContent = "Architect · " + (s.brain2_model || s.brain2_name);
  byId("pill-daemon").textContent = s.daemon_running ? ("PID " + s.daemon_pid + " (" + s.brain1_latency_ms + "ms)") : "Embedded";

  // APW.md (project instructions) status in Left Sidebar
  const dot = byId("instr-dot");
  const badge = byId("instr-badge");
  const desc = byId("instr-desc");
  const createBtn = byId("btn-create-instr");

  if (s.instructions_loaded) {
    dot.className = "w-1.5 h-1.5 rounded-full bg-emerald-400";
    badge.className = "text-[10px] font-mono text-emerald-300";
    badge.textContent = s.instructions_lines + "L";
    desc.textContent = "โหลด " + (s.instructions_path || "APW.md") + " (" + s.instructions_size + " bytes) เข้าสู่บริบทของ Agent แล้ว";
    createBtn.classList.add("hidden");
  } else {
    dot.className = "w-1.5 h-1.5 rounded-full bg-amber-400";
    badge.className = "text-[10px] font-mono text-slate-400";
    badge.textContent = "None";
    desc.textContent = "ยังไม่มี APW.md ในโปรเจกต์ (APW อ่าน AGENTS.md ด้วยถ้ามี) — กดสร้างไฟล์เริ่มต้นได้";
    createBtn.classList.remove("hidden");
  }
}

async function openInstructionsInArtifactPane() {
  byId("artifact-title").textContent = "APW.md";
  const out = await getAPI("/api/instructions");
  if (out.status === 404) {
    showCodeViewer("APW.md", "ยังไม่มี APW.md — กด '+ สร้าง APW.md' ที่แถบซ้ายเพื่อสร้างไฟล์เริ่มต้น (APW อ่าน AGENTS.md ด้วยถ้ามี)", "ไม่พบไฟล์");
  } else if (!out.ok) {
    showCodeViewer("APW.md", out.message, "ERROR");
  } else {
    const data = out.data || {};
    byId("artifact-title").textContent = data.path || "APW.md";
    showCodeViewer(data.path || "APW.md", data.content || "", data.truncated ? "ตัดทอน (แสดงบางส่วน)" : "");
  }
  openRightPane("code");
}

async function initInstructions() {
  const out = await postAPI("/api/instructions/init", {}, false);
  if (!out.ok) {
    const msg = out.error === "busy" ? "ตอนนี้มีงานกำลังใช้ Workspace อยู่ (busy) — ลองใหม่หลังงานเสร็จ" : out.message;
    appendCard("ไม่สามารถสร้าง APW.md ได้", msg, "ERROR", "#f85149");
    return;
  }
  if (out.data && out.data.created === false) {
    appendCard("APW.md", "มี APW.md หรือ AGENTS.md อยู่แล้ว จึงไม่สร้างทับ", "SKIPPED", "#6b7280");
  }
  await loadStatus();
  await openInstructionsInArtifactPane();
}

/* Persistent client-side session registry so creating a new session (+ Session ใหม่)
 * adds to the list instead of overwriting existing sessions before a task finishes. */
const LOCAL_SESSIONS_KEY = "apw_desktop_sessions_v2";

function loadLocalSessionRegistry() {
  const raw = lsGet(LOCAL_SESSIONS_KEY, "[]");
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch (e) {
    return [];
  }
}

function saveLocalSessionRegistry(list) {
  lsSet(LOCAL_SESSIONS_KEY, JSON.stringify((list || []).slice(0, 60)));
}

function upsertLocalSession(id, patch) {
  if (!id) return;
  const list = loadLocalSessionRegistry();
  const now = new Date().toISOString();
  const idx = list.findIndex(s => s && s.id === id);
  if (idx >= 0) {
    const cur = list[idx];
    list[idx] = {
      id,
      created_at: cur.created_at || now,
      updated_at: (patch && patch.updated_at) || now,
      turn_count: (patch && typeof patch.turn_count === "number") ? Math.max(cur.turn_count || 0, patch.turn_count) : (cur.turn_count || 0),
      last_instruction: (patch && patch.last_instruction) ? patch.last_instruction : (cur.last_instruction || "Session พร้อมรับคำสั่ง")
    };
  } else {
    list.unshift({
      id,
      created_at: now,
      updated_at: (patch && patch.updated_at) || now,
      turn_count: (patch && typeof patch.turn_count === "number") ? patch.turn_count : 0,
      last_instruction: (patch && patch.last_instruction) ? patch.last_instruction : "Session ใหม่ (พร้อมรับคำสั่ง)"
    });
  }
  saveLocalSessionRegistry(list);
}

function removeLocalSession(id) {
  if (!id) return;
  const list = loadLocalSessionRegistry().filter(s => s && s.id !== id);
  saveLocalSessionRegistry(list);
}

function getLocalSessionTurns(id) {
  if (!id) return [];
  const raw = lsGet("apw_desktop_turns_" + id, "[]");
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch (e) {
    return [];
  }
}

function appendLocalSessionTurn(id, turn) {
  if (!id || !turn) return;
  const turns = getLocalSessionTurns(id);
  const nextIdx = turns.length + 1;
  turns.push({
    index: turn.index || nextIdx,
    task_id: turn.task_id || "",
    timestamp: turn.timestamp || new Date().toISOString(),
    instruction: turn.instruction || "",
    success: turn.success !== false,
    applied: !!turn.applied,
    summary: turn.summary || ""
  });
  lsSet("apw_desktop_turns_" + id, JSON.stringify(turns.slice(-30)));
  upsertLocalSession(id, {
    turn_count: turns.length,
    last_instruction: turn.instruction || "อัปเดตล่าสุด",
    updated_at: new Date().toISOString()
  });
}

function mergeTurnsForSession(id, serverTurns) {
  const srv = Array.isArray(serverTurns) ? serverTurns : [];
  const loc = getLocalSessionTurns(id);
  if (!loc.length) return srv;
  const seen = new Set(srv.map(t => (t.task_id || "") + "|" + (t.instruction || "")));
  const combined = srv.slice();
  loc.forEach(lt => {
    const key = (lt.task_id || "") + "|" + (lt.instruction || "");
    if (!seen.has(key)) {
      combined.push({
        index: combined.length + 1,
        task_id: lt.task_id || "",
        timestamp: lt.timestamp,
        instruction: lt.instruction,
        success: lt.success !== false,
        applied: !!lt.applied,
        summary: lt.summary || ""
      });
    }
  });
  return combined;
}

function humanSessionTitle(sess) {
  const raw = String((sess && sess.last_instruction) || "").trim();
  if (!raw || raw.startsWith("Session ")) return "New conversation";
  return raw;
}

function updateActiveSessionHeader(id, titleText) {
  const badge = byId("active-session-badge");
  if (badge) badge.textContent = id ? String(id).slice(0, 14) : "-";
  const titleEl = byId("active-session-title");
  if (titleEl) {
    if (titleText && !titleText.startsWith("Session ")) {
      titleEl.textContent = titleText;
    } else {
      const reg = loadLocalSessionRegistry().find(s => s && s.id === id);
      titleEl.textContent = humanSessionTitle(reg);
    }
  }
}

function sessionItem(sess, indexNum) {
  const id = sess.id;
  const active = id === currentSessionId;
  const item = mk("div", "ent-list-row " + (active ? "active" : ""));
  const titleText = humanSessionTitle(sess);
  item.title = id + " — " + titleText;

  const left = mk("div", "flex items-center gap-2 min-w-0 flex-1");
  left.append(
    mk("span", "w-1.5 h-1.5 rounded-full shrink-0 " + (active ? "bg-white" : "bg-slate-600")),
    mk("span", "text-xs truncate " + (active ? "font-medium text-white" : "text-slate-300"), titleText)
  );
  const right = mk("div", "flex items-center gap-1 shrink-0");
  if ((sess.turn_count || 0) > 0) {
    right.appendChild(
      mk("span", "text-[10px] font-mono text-slate-500", (sess.turn_count || 0) + "t")
    );
  }
  if (!sess.persisted && !active) {
    const delBtn = mk("button", "text-[10px] font-mono px-1 rounded hover:bg-rose-500/25 text-slate-500 hover:text-rose-300 transition", "×");
    delBtn.title = "Remove empty conversation";
    delBtn.onclick = (e) => {
      e.stopPropagation();
      removeLocalSession(id);
      loadSessions();
    };
    right.appendChild(delBtn);
  }
  item.append(left, right);
  item.onclick = () => selectSession(id);
  if (active) updateActiveSessionHeader(id, titleText);
  return item;
}

async function loadSessions() {
  const res = await apiFetch("/api/sessions");
  const serverSessions = res.ok ? (await res.json() || []) : [];
  const listEl = byId("session-list");
  if (!listEl) return;
  listEl.replaceChildren();

  if (currentSessionId) {
    upsertLocalSession(currentSessionId, {});
  }

  const byIdMap = new Map();

  // 1. Server-persisted sessions (.apw/sessions)
  serverSessions.forEach(s => {
    if (!s || !s.id) return;
    byIdMap.set(s.id, {
      id: s.id,
      updated_at: s.updated_at || new Date().toISOString(),
      turn_count: s.turn_count || 0,
      last_instruction: s.last_instruction || "Session บันทึกแล้ว",
      persisted: true
    });
    upsertLocalSession(s.id, {
      turn_count: s.turn_count || 0,
      last_instruction: s.last_instruction || "Session บันทึกแล้ว",
      updated_at: s.updated_at
    });
  });

  // 2. Sessions referenced by tasks in SQLite (including running/queued tasks)
  (allLoadedTasks || []).forEach(job => {
    const sid = job && job.session_id;
    if (!sid) return;
    const existing = byIdMap.get(sid);
    if (!existing) {
      byIdMap.set(sid, {
        id: sid,
        updated_at: job.updated_at || job.created_at || new Date().toISOString(),
        turn_count: 1,
        last_instruction: job.instruction || "กำลังประมวลผลงาน...",
        persisted: true
      });
    } else if (!existing.last_instruction && job.instruction) {
      existing.last_instruction = job.instruction;
    }
  });

  // 3. Client-created sessions in localStorage (never overwrite previous sessions!)
  loadLocalSessionRegistry().forEach(localSess => {
    if (!localSess || !localSess.id) return;
    const localTurns = getLocalSessionTurns(localSess.id);
    if (!byIdMap.has(localSess.id)) {
      byIdMap.set(localSess.id, {
        id: localSess.id,
        updated_at: localSess.updated_at || localSess.created_at || new Date().toISOString(),
        turn_count: Math.max(localSess.turn_count || 0, localTurns.length),
        last_instruction: localSess.last_instruction || "Session พร้อมรับคำสั่ง",
        persisted: false
      });
    } else {
      const entry = byIdMap.get(localSess.id);
      if (localTurns.length > (entry.turn_count || 0)) {
        entry.turn_count = localTurns.length;
      }
      if ((!entry.last_instruction || entry.last_instruction === "Session ว่าง") && localSess.last_instruction) {
        entry.last_instruction = localSess.last_instruction;
      }
    }
  });

  const merged = Array.from(byIdMap.values());
  merged.sort((a, b) => {
    if (a.id === currentSessionId) return -1;
    if (b.id === currentSessionId) return 1;
    return String(b.updated_at || "").localeCompare(String(a.updated_at || ""));
  });

  const countBadge = byId("session-count-badge");
  if (countBadge) countBadge.textContent = String(merged.length);

  const q = (currentSidebarFilter || "").toLowerCase();
  const visibleSessions = q
    ? merged.filter(s => String(s.id || "").toLowerCase().includes(q) || String(s.last_instruction || "").toLowerCase().includes(q))
    : merged;

  visibleSessions.forEach((sess, idx) => {
    listEl.appendChild(sessionItem(sess, idx + 1));
  });
}

let currentSidebarFilter = "";
function onSidebarSearch(val) {
  currentSidebarFilter = String(val || "").trim();
  loadSessions();
  renderTaskList(allLoadedTasks);
}

function statusBadgeStyle(status) {
  if (activeStates.has(status)) return "bg-amber-500/20 text-amber-300 border-amber-500/30";
  if (status === "succeeded" || status === "ready_to_apply" || status === "preview_ready") return "bg-emerald-500/20 text-emerald-300 border-emerald-500/30";
  return "bg-rose-500/20 text-rose-300 border-rose-500/30";
}

function renderTaskList(jobs) {
  allLoadedTasks = jobs || [];
  const listEl = byId("task-list");
  const countEl = byId("task-count-badge");
  if (countEl) countEl.textContent = String(allLoadedTasks.length);
  if (!listEl) return;
  listEl.replaceChildren();

  if (allLoadedTasks.length === 0) {
    listEl.appendChild(mk("div", "text-[11px] text-slate-500 px-2 py-1.5", "No background tasks yet"));
    return;
  }

  const q = (currentSidebarFilter || "").toLowerCase();
  const filteredTasks = q
    ? allLoadedTasks.filter(j => String(j.id || "").toLowerCase().includes(q) || String(j.instruction || "").toLowerCase().includes(q) || String(j.status || "").toLowerCase().includes(q))
    : allLoadedTasks;

  filteredTasks.slice(0, 20).forEach(job => {
    const active = job.id === selectedTaskId;
    const isRunning = activeStates.has(job.status);
    const isOk = job.status === "succeeded" || job.status === "ready_to_apply" || job.status === "preview_ready";
    const dotCls = isRunning ? "bg-amber-400 pulse-dot" : (isOk ? "bg-emerald-400" : "bg-rose-400");
    const statusShort = job.status === "ready_to_apply" ? "ready" : (job.status === "preview_ready" ? "preview" : job.status);

    const item = mk("div", "ent-list-row " + (active ? "active" : ""));
    item.title = job.id + " [" + job.status + "]\n" + (job.instruction || "");

    const left = mk("div", "flex items-center gap-2 min-w-0 flex-1");
    left.append(
      mk("span", "w-1.5 h-1.5 rounded-full shrink-0 " + dotCls),
      mk("span", "text-xs truncate " + (active ? "font-medium text-white" : "text-slate-300"), job.instruction || job.id)
    );
    const right = mk("span", "text-[10px] font-mono shrink-0 " + (isRunning ? "text-amber-300" : (isOk ? "text-emerald-400" : "text-slate-500")), statusShort);
    item.append(left, right);
    item.onclick = () => openTaskById(job.id);
    listEl.appendChild(item);
  });
}

async function openTaskById(taskId) {
  if (!taskId) return;
  const out = await getAPI("/v1/tasks/" + encodeURIComponent(taskId));
  if (!out.ok || !out.data) {
    appendCard("ไม่พบข้อมูลงาน " + taskId, out.message || "Task not found", "ERROR", "#f85149");
    return;
  }
  const job = out.data;
  selectedTaskId = job.id;
  lsSet("apw_desktop_task", job.id);
  currentDiffFilterFile = "";
  renderTaskList(allLoadedTasks);
  renderLiveTaskCard(job);
  populateArtifactFromJob(job);
  if (activeStates.has(job.status)) {
    watchTask(job);
  }
}

function renderSessionTurns(turns) {
  const box = byId("session-history");
  box.replaceChildren();
  turns.forEach(turn => {
    const card = mk("div", "ent-tool-row p-3.5 space-y-2 transition");
    const head = mk("div", "flex items-center justify-between gap-2");
    const when = turn.timestamp ? new Date(turn.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "";
    head.append(mk("span", "text-[11px] font-mono text-slate-400", "Turn #" + turn.index + (when ? " · " + when : "")));

    const rightBadges = mk("div", "flex items-center gap-1.5");
    if (turn.task_id) {
      const taskBtn = mk("button", "text-[10px] font-mono px-2 py-0.5 rounded bg-white/[0.06] text-slate-200 hover:bg-white/[0.12] transition",
        "Inspect " + String(turn.task_id).slice(0, 10) + " →");
      taskBtn.onclick = () => openTaskById(turn.task_id);
      rightBadges.appendChild(taskBtn);
    }
    const state = turn.success ? (turn.applied ? "APPLIED" : "READY") : "STOPPED";
    rightBadges.appendChild(mk("span", "text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.04] text-slate-400", state));
    head.appendChild(rightBadges);

    card.append(head, mk("div", "text-xs text-slate-100 whitespace-pre-wrap leading-relaxed", turn.instruction || ""));
    if (turn.summary) {
      const text = String(turn.summary);
      card.appendChild(mk("div", "text-[11px] text-slate-400 whitespace-pre-wrap leading-relaxed pt-1 border-t border-white/[0.05]",
        text.length > 600 ? text.slice(0, 600) + "…" : text));
    }
    box.appendChild(card);
  });
}

async function selectSession(id) {
  if (currentSessionId) {
    upsertLocalSession(currentSessionId, {});
  }
  currentSessionId = id;
  lsSet("apw_desktop_session", id);
  upsertLocalSession(id, {});
  updateActiveSessionHeader(id);
  clearLiveFeed();
  await loadSessions();

  const out = await getAPI("/api/sessions/" + encodeURIComponent(id));
  const srvTurns = (out.ok && out.data && out.data.turns) ? out.data.turns : [];
  renderSessionTurns(mergeTurnsForSession(id, srvTurns));

  // Also restore task cards belonging to this session
  (allLoadedTasks || []).filter(j => j && j.session_id === id).slice(0, 6).reverse().forEach(j => {
    renderLiveTaskCard(j);
  });
}

async function createNewSession() {
  if (currentSessionId) {
    upsertLocalSession(currentSessionId, {});
  }
  const out = await postAPI("/api/sessions", {}, false);
  if (!out.ok || !out.data || !out.data.id) {
    appendCard("สร้าง Session ใหม่ไม่สำเร็จ", out.message || "no session id returned", "ERROR", "#f85149");
    return false;
  }
  currentSessionId = out.data.id;
  lsSet("apw_desktop_session", currentSessionId);
  upsertLocalSession(currentSessionId, {
    turn_count: 0,
    last_instruction: "Session ใหม่ (พร้อมรับคำสั่ง)",
    updated_at: new Date().toISOString()
  });
  updateActiveSessionHeader(currentSessionId, "New conversation");
  clearLiveFeed();
  renderSessionTurns([]);
  await loadSessions();
  return true;
}

/* Makes sure a valid conversation id exists (the server hands out ids; nothing is written until a task uses one). */
async function ensureSession() {
  if (currentSessionId) {
    upsertLocalSession(currentSessionId, {});
    updateActiveSessionHeader(currentSessionId);
    const out = await getAPI("/api/sessions/" + encodeURIComponent(currentSessionId));
    if (out.ok) {
      renderSessionTurns(mergeTurnsForSession(currentSessionId, (out.data && out.data.turns) || []));
      return;
    }
  }
  const out = await postAPI("/api/sessions", {}, false);
  if (out.ok && out.data && out.data.id) {
    currentSessionId = out.data.id;
    lsSet("apw_desktop_session", currentSessionId);
    upsertLocalSession(currentSessionId, {
      turn_count: 0,
      last_instruction: "Session ปัจจุบัน (พร้อมรับคำสั่ง)"
    });
  }
  updateActiveSessionHeader(currentSessionId);
}

function triggerComposerPrefix(prefix) {
  const input = byId("task-input");
  if (!input) return;
  if (prefix === "/") {
    input.value = "/";
    input.focus();
    input.setSelectionRange(1, 1);
    onComposerInput();
    return;
  }
  const val = input.value || "";
  const sep = (val && !val.endsWith(" ")) ? " " : "";
  const next = val + sep + prefix;
  input.value = next;
  input.focus();
  input.setSelectionRange(next.length, next.length);
  onComposerInput();
}

function quickPrompt(text, mode) {
  const input = byId("task-input");
  if (!input) return;
  input.value = text;
  runTask(mode || "submit");
}

// Mention completion queries the server per token: files and symbols are matched across the
// whole workspace server-side (results are capped), so nothing is cached beyond the last query.
let mentionSearchSeq = 0;
let mentionSearchTimer = null;
let mentionCandidates = [];
let mentionSelectedIndex = 0;

const desktopSlashCommands = [
  { cmd: "/plan ", label: "/plan <คำสั่ง>", meta: "Spec-First Planning (สร้างแผนก่อนแก้จริง)" },
  { cmd: "/review ", label: "/review <คำสั่ง>", meta: "อ่านและวิเคราะห์โค้ดโดยไม่รันคำสั่ง" },
  { cmd: "/preview ", label: "/preview <คำสั่ง>", meta: "สร้าง Diff ใน Isolated Worktree" },
  { cmd: "/verify", label: "/verify", meta: "รันตรวจสอบโค้ดด้วย Reflex Pipeline", action: () => runVerify() },
  { cmd: "/symbols", label: "/symbols", meta: "เปิดดู AST/Polyglot Symbols ทั้งโปรเจกต์", action: () => openSymbolsArtifact() },
  { cmd: "/graph", label: "/graph", meta: "เปิดดู Cross-File Call Impact Graph", action: () => openGraphArtifact() },
  { cmd: "/autonomy", label: "/autonomy", meta: "เปิดหน้าต่าง 24/7 Autonomy & Skills", action: () => openAutonomyDialog() },
  { cmd: "/settings", label: "/settings", meta: "ตั้งค่า Brain 1 / Brain 2 Models & Sandbox", action: () => openSettingsDialog() },
  { cmd: "/new", label: "/new", meta: "เริ่มบทสนทนา (Session) ใหม่", action: () => createNewSession() }
];

function searchWorkspace(rawQuery) {
  if (mentionSearchTimer) clearTimeout(mentionSearchTimer);
  const seq = ++mentionSearchSeq;
  return new Promise(resolve => {
    mentionSearchTimer = setTimeout(async () => {
      let data = null;
      try {
        const r = await apiFetch("/api/search?q=" + encodeURIComponent(rawQuery || ""));
        if (r.ok) data = await r.json();
      } catch (_) {}
      // A newer keystroke superseded this query; its own request will render.
      resolve(seq === mentionSearchSeq ? (data || { files: [], symbols: [], skills: [] }) : null);
    }, 120);
  });
}

function activeMentionToken(textarea) {
  if (!textarea) return null;
  const pos = textarea.selectionStart || 0;
  const before = (textarea.value || "").slice(0, pos);
  if (/^\/[A-Za-z0-9_-]*$/.test(before)) {
    return { kind: "slash", query: before.slice(1), atStart: 0, atEnd: pos };
  }
  const m = /(?:^|\s)@(#?[A-Za-z0-9_:./\\-]*)$/.exec(before);
  if (!m) return null;
  const query = m[1] || "";
  const atStart = pos - query.length - 1;
  return { kind: "mention", query, atStart, atEnd: pos };
}

function cancelMentionSearch() {
  mentionSearchSeq++;
  if (mentionSearchTimer) clearTimeout(mentionSearchTimer);
  mentionSearchTimer = null;
}

function hideMentionMenu() {
  cancelMentionSearch();
  const menu = byId("mention-menu");
  if (menu) {
    menu.classList.add("hidden");
    menu.replaceChildren();
  }
  mentionCandidates = [];
  mentionSelectedIndex = 0;
}

function insertMentionCandidate(cand) {
  const input = byId("task-input");
  const tok = activeMentionToken(input);
  if (!input || !tok || !cand) return;
  if (cand.action) {
    input.value = "";
    hideMentionMenu();
    cand.action();
    return;
  }
  const val = input.value || "";
  const replacement = tok.kind === "slash" ? cand.insert : ("@" + cand.insert + " ");
  input.value = val.slice(0, tok.atStart) + replacement + val.slice(tok.atEnd);
  const nextCursor = tok.atStart + replacement.length;
  input.focus();
  input.setSelectionRange(nextCursor, nextCursor);
  hideMentionMenu();
}

function renderMentionMenu() {
  const menu = byId("mention-menu");
  if (!menu) return;
  menu.replaceChildren();
  if (!mentionCandidates.length) {
    menu.classList.add("hidden");
    return;
  }
  menu.classList.remove("hidden");
  mentionCandidates.forEach((cand, idx) => {
    const active = idx === mentionSelectedIndex;
    const row = mk("div", "flex items-center justify-between gap-2 px-2.5 py-1.5 rounded-lg cursor-pointer transition " +
      (active ? "bg-[#E07A5F]/20 text-white border border-[#E07A5F]/40" : "hover:bg-white/[0.06] text-slate-300"));
    const left = mk("span", "truncate font-mono", cand.display || ("@" + cand.insert));
    const badge = mk("span", "text-[10px] px-1.5 py-0.5 rounded bg-white/[0.06] text-slate-400 shrink-0", cand.meta);
    row.append(left, badge);
    row.onmousedown = (ev) => {
      ev.preventDefault();
      insertMentionCandidate(cand);
    };
    menu.appendChild(row);
  });
}

async function onComposerInput() {
  const input = byId("task-input");
  const tok = activeMentionToken(input);
  if (!tok) {
    hideMentionMenu();
    return;
  }
  if (tok.kind === "slash") {
    cancelMentionSearch();
    const q = (tok.query || "").toLowerCase();
    mentionCandidates = desktopSlashCommands
      .filter(c => c.cmd.toLowerCase().includes(q) || c.meta.toLowerCase().includes(q))
      .map(c => ({ insert: c.cmd, display: c.label, meta: c.meta, action: c.action }));
    mentionSelectedIndex = 0;
    renderMentionMenu();
    return;
  }
  const rawQ = tok.query;
  const idx = await searchWorkspace(rawQ);
  if (!idx) return;
  const symOnly = rawQ.startsWith("#");
  const skillOnly = rawQ.toLowerCase().startsWith("skill:");
  const q = (symOnly ? rawQ.slice(1) : (skillOnly ? rawQ.slice(6) : rawQ)).toLowerCase();
  const items = [];
  if (!symOnly) {
    (idx.skills || []).forEach(sk => {
      const slug = sk.slug || "";
      const desc = sk.description || "";
      if (!q || slug.toLowerCase().includes(q) || desc.toLowerCase().includes(q)) {
        items.push({ insert: "skill:" + slug, meta: "skill · " + (desc.slice(0, 42) || ".apw/skills") });
      }
    });
  }
  if (!symOnly && !skillOnly) {
    (idx.files || []).forEach(f => {
      items.push({ insert: f, meta: "file" });
    });
  }
  if (!skillOnly) {
    (idx.symbols || []).forEach(s => {
      const name = s.name || "";
      const file = s.file || "";
      items.push({ insert: "#" + name, meta: (s.kind || "sym") + " · " + file + ":L" + (s.start_line || 1) });
    });
  }
  mentionCandidates = items.slice(0, 8);
  mentionSelectedIndex = 0;
  renderMentionMenu();
}

function onComposerKeyDown(event) {
  const menu = byId("mention-menu");
  const menuOpen = menu && !menu.classList.contains("hidden") && mentionCandidates.length > 0;
  if (menuOpen) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      mentionSelectedIndex = (mentionSelectedIndex + 1) % mentionCandidates.length;
      renderMentionMenu();
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      mentionSelectedIndex = (mentionSelectedIndex - 1 + mentionCandidates.length) % mentionCandidates.length;
      renderMentionMenu();
      return;
    }
    if (event.key === "Tab" || (event.key === "Enter" && !event.shiftKey)) {
      event.preventDefault();
      insertMentionCandidate(mentionCandidates[mentionSelectedIndex]);
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      hideMentionMenu();
      return;
    }
  }
  if (event.key === "Enter" && !event.shiftKey) {
    event.preventDefault();
    runTask("submit");
  }
}

async function openSymbolsArtifact() {
  const r = await apiFetch("/api/symbols");
  if (!r.ok) return;
  const files = await r.json() || [];
  const lines = files.map(f => "=== " + f.file + " ===\n" + (f.symbols || []).map(s => "  [" + s.Kind + "] " + s.Name + " (L" + s.StartLine + "-L" + s.EndLine + ")").join("\n")).join("\n\n");
  byId("artifact-title").textContent = "Go AST Symbols (" + files.length + " files)";
  showCodeViewer("workspace.symbols", lines || "ไม่พบ Go AST Symbols", "");
  openRightPane("code");
}

async function openGraphArtifact() {
  const r = await apiFetch("/api/graph");
  if (!r.ok) return;
  const data = await r.json() || {};
  const symbols = data.definitions || [];
  const edges = data.calls || [];
  const summaryLines = [
    "=== Cross-File AST Symbol & Call Impact Graph ===",
    "Symbols Indexed : " + symbols.length,
    "Call Edges      : " + edges.length,
    ""
  ];
  const byFile = {};
  edges.forEach(e => {
    const key = e.from_file || "(unknown)";
    if (!byFile[key]) byFile[key] = [];
    byFile[key].push("  L" + e.line + ": " + e.from_symbol + "  ──calls──►  " + e.to_symbol);
  });
  Object.keys(byFile).sort().forEach(f => {
    summaryLines.push("▸ " + f);
    byFile[f].slice(0, 40).forEach(line => summaryLines.push(line));
    summaryLines.push("");
  });
  byId("artifact-title").textContent = "AST Call Impact Graph (" + edges.length + " edges)";
  showCodeViewer("workspace.callgraph", summaryLines.join("\n"), "");
  openRightPane("code");
}

/* ============================================================================
 * 8. TASK WATCHING, INLINE CONTROLS, APPLY & EXECUTION REPORT
 * ============================================================================ */
function renderLiveTaskCard(job) {
  const container = byId("live-feed") || byId("feed");
  const cardId = "live-task-card-" + job.id;
  let card = byId(cardId);
  const isNew = !card;
  if (isNew) {
    card = mk("div", "ent-tool-row p-3.5 space-y-2.5");
    card.id = cardId;
  } else {
    card.replaceChildren();
  }

  const head = mk("div", "flex flex-wrap items-center justify-between gap-2");
  const left = mk("div", "flex items-center gap-2 min-w-0");
  const isRunning = activeStates.has(job.status);
  left.append(
    mk("span", "w-2 h-2 rounded-full shrink-0 " + (isRunning ? "bg-amber-400 pulse-dot" : job.error ? "bg-rose-400" : "bg-emerald-400")),
    mk("span", "text-xs font-medium text-white truncate", job.instruction || job.id),
    mk("span", "text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.06] text-slate-400 shrink-0", (job.mode || "execute").toUpperCase())
  );
  head.append(
    left,
    mk("span", "text-[10px] font-mono px-2 py-0.5 rounded border shrink-0 " + statusBadgeStyle(job.status), job.status.toUpperCase())
  );
  card.appendChild(head);

  const files = collectJobFiles(job);
  if (files.length > 0) {
    const fileRow = mk("div", "flex flex-wrap items-center gap-1.5 pt-0.5");
    fileRow.appendChild(mk("span", "text-[10.5px] font-mono text-slate-400", files.length + " files changed:"));
    files.forEach(f => {
      const fBtn = mk("button", "text-[11px] font-mono px-2 py-0.5 rounded bg-[#0c0c0e] hover:bg-white/[0.08] text-slate-200 border border-white/[0.07] transition", f);
      fBtn.onclick = () => {
        populateArtifactFromJob(job);
        onSelectArtifactFile(f);
      };
      fileRow.appendChild(fBtn);
    });
    card.appendChild(fileRow);
  }

  if (job.error) {
    card.appendChild(mk("div", "text-xs font-mono text-rose-300 bg-rose-500/15 border border-rose-500/30 rounded-lg p-2.5 whitespace-pre-wrap", job.error));
  }

  const actionRow = mk("div", "flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-white/[0.06]");
  const inspectBtn = mk("button", "ent-chip-btn font-mono", "Review Diff & Evidence →");
  inspectBtn.onclick = () => {
    populateArtifactFromJob(job);
    openRightPane("diff");
  };
  const btnGroup = mk("div", "flex flex-wrap items-center gap-1.5");
  if ((job.status === "succeeded" || job.status === "ready_to_apply") && job.report && job.report.passed) {
    const skillBtn = mk("button", "ent-chip-btn font-mono", "+ Save as Skill");
    skillBtn.onclick = (e) => { e.stopPropagation(); extractSkillFromTask(job); };
    btnGroup.appendChild(skillBtn);
  }
  createTaskActionButtons(job).forEach(b => btnGroup.appendChild(b));
  actionRow.append(inspectBtn, btnGroup);
  card.appendChild(actionRow);

  if (isNew) {
    container.appendChild(card);
    const scroller = byId("feed");
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
  }
}

async function applyTask(job, how) {
  if (applyingTasks.has(job.id)) return;
  applyingTasks.add(job.id);
  startApprovalPolling();
  const approval = how || ((approvalMode === "auto" && job.mode !== "unverified") ? "auto" : "ask");
  try {
    const out = await postAPI("/api/tasks/" + encodeURIComponent(job.id) + "/apply", { approval }, false);
    if (!out.ok) {
      if (out.status === 403 || out.error === "denied") {
        appendCard("Apply ถูกปฏิเสธ", "ผู้ใช้ปฏิเสธการ Apply — Workspace ไม่ถูกแก้ไข", "DENIED", "#f85149");
      } else if (out.error === "not_ready") {
        appendCard("Apply ไม่ได้", "งานนี้ยังไม่อยู่ในสถานะพร้อม Apply (ready_to_apply)", "NOT READY", "#f59e0b");
      } else {
        appendCard("Apply ล้มเหลว", out.message, "ERROR", "#f85149");
        showConflictBanner(job, out.message);
        openRightPane("diff");
      }
      return;
    }
    const done = out.data || job;
    appendCollapsibleToolCard("Apply", "[" + job.id + "] Apply ลง Workspace: " + done.status,
      done.error ? ("Error: " + done.error) : (approval === "auto" ? "Apply อัตโนมัติ (auto)" : "Apply หลังได้รับอนุญาต"),
      done.error ? "ERROR" : "DONE");
    renderLiveTaskCard(done);
    populateArtifactFromJob(done);
    await loadSessions();
    if (currentSessionId) {
      const sOut = await getAPI("/api/sessions/" + encodeURIComponent(currentSessionId));
      if (sOut.ok) renderSessionTurns(mergeTurnsForSession(currentSessionId, (sOut.data && sOut.data.turns) || []));
    }
    loadTasks();
  } finally {
    applyingTasks.delete(job.id);
  }
}

async function watchTask(initial) {
  if (watchedTasks.has(initial.id)) return;
  watchedTasks.add(initial.id);
  startApprovalPolling();
  let job = initial, cursor = 0;

  renderLiveTaskCard(job);
  const toolCardId = "task-tool-" + job.id;
  appendCollapsibleToolCard("TriadExecutor", "[" + job.id + "] " + job.instruction, "กำลังเริ่มต้นงานในโหมด " + (job.mode || "execute") + "...", "RUNNING", toolCardId);

  try {
    while (true) {
      const eventResponse = await apiFetch("/v1/tasks/" + encodeURIComponent(job.id) + "/events?after=" + cursor);
      if (!eventResponse.ok) throw new Error(await eventResponse.text());
      const events = await eventResponse.json() || [];
      for (const event of events) {
        cursor = event.seq;
        appendCollapsibleToolCard(
          event.stage.toUpperCase(),
          event.message,
          "Task: " + job.id + " · Stage: " + event.stage + " · Seq #" + event.seq,
          "DONE"
        );
      }
      const response = await apiFetch("/v1/tasks/" + encodeURIComponent(job.id));
      if (!response.ok) throw new Error(await response.text());
      job = await response.json();
      renderLiveTaskCard(job);
      if (!activeStates.has(job.status)) {
        appendCollapsibleToolCard(
          "TriadExecutor",
          "[" + job.id + "] เสร็จสิ้นด้วยสถานะ: " + job.status,
          job.error ? ("Error: " + job.error) : ("สมบูรณ์ในโหมด " + job.mode),
          job.error ? "ERROR" : "DONE",
          toolCardId
        );
        if (job.report) {
          appendExecutionReport(job.report);
          if (job.report.summary) {
            appendStreamingBubble("agent", "Apiwat Agent Summary", job.report.summary, job.status.toUpperCase());
          }
        }
        if (job.error) {
          appendCard("Task stopped", job.error, job.status, "#f85149");
        }
        populateArtifactFromJob(job);
        await loadSessions();
        if (currentSessionId) {
          const sOut = await getAPI("/api/sessions/" + encodeURIComponent(currentSessionId));
          if (sOut.ok) renderSessionTurns(mergeTurnsForSession(currentSessionId, (sOut.data && sOut.data.turns) || []));
        }
        loadTasks();
        // auto mode: apply once when verified work is ready (never for unverified tasks)
        if (job.status === "ready_to_apply" && approvalMode === "auto" && job.mode !== "unverified" && !autoAppliedTasks.has(job.id)) {
          autoAppliedTasks.add(job.id);
          await applyTask(job, "auto");
        }
        break;
      }
      await new Promise(resolve => setTimeout(resolve, 450));
    }
  } catch (error) {
    appendCard("Connection interrupted", error.message + " — งานยังคงถูกติดตามโดย Daemon", "ERROR", "#f85149");
  } finally {
    watchedTasks.delete(initial.id);
  }
}

function formatNs(ns) {
  if (!ns) return "0ms";
  const ms = Math.round(ns / 1e6);
  return ms >= 1000 ? (ms / 1000).toFixed(2) + "s" : ms + "ms";
}

function appendExecutionReport(data) {
  const plan = data.plan || {};
  const steps = (data.steps || []).map(s =>
    "  " + (s.success ? "✓" : "✗") + " [" + s.action.toUpperCase() + "] " + s.subtask_id + (s.target_file ? " (" + s.target_file + ")" : "") + " — " + s.message
  ).join("\n");

  const attempts = (data.reflex_attempts || []).map(a =>
    "  • รอบที่ " + a.Number + ": " + (a.Result && a.Result.passed ? "PASS (ผ่านทุก Stage)" : "FAIL ที่ขั้นตอน " + (a.Result ? a.Result.failed_stage : "unknown")) + " [" + formatNs(a.Result ? a.Result.duration : 0) + "]"
  ).join("\n");

  const summary =
    "เป้าหมาย (Objective): " + (plan.objective || "-") + "\n" +
    "ไฟล์ที่เกี่ยวข้อง     : " + ((plan.affected_files || []).join(", ") || "-") + "\n\n" +
    "ขั้นตอนการทำงาน (Surgeon Steps):\n" + (steps || "  (ไม่มีขั้นตอน)") + "\n\n" +
    "ผลการตรวจสอบ (Reflex Verification):\n" + (attempts || "  (ไม่ได้รัน)");

  appendCollapsibleToolCard(
    "ExecutionReport",
    "Triad Report — " + (plan.plan_id || data.task_id || "completed") + " (" + formatNs(data.duration) + ")",
    summary,
    data.passed || data.mode === "preview" || data.mode === "review" ? "DONE" : "ERROR"
  );
}

function appendPlanCard(plan) {
  const subtasks = (plan.subtasks || []).map(st =>
    "  • [" + st.action.toUpperCase() + "] " + st.id + (st.target_file ? " (" + st.target_file + ")" : "") + ": " + st.description
  ).join("\n");

  const summary =
    "เป้าหมาย (Objective): " + (plan.objective || "-") + "\n" +
    "ไฟล์ที่เกี่ยวข้อง     : " + ((plan.affected_files || []).join(", ") || "-") + "\n\n" +
    "แผนงานย่อย (Subtasks):\n" + subtasks + "\n\n" +
    "เกณฑ์ผ่าน (Acceptance Criteria):\n  - " + ((plan.acceptance_criteria || []).join("\n  - ") || "-");

  appendCollapsibleToolCard("ArchitectPlan", "แผนงาน: " + (plan.objective || plan.plan_id), summary, "DONE");
  byId("artifact-title").textContent = "ExecutionPlan: " + (plan.plan_id || "");
  showCodeViewer((plan.plan_id || "plan") + ".json", JSON.stringify(plan, null, 2), "");
  openRightPane("code");
}

async function runVerify() {
  const out = await postAPI("/api/verify", {}, true);
  if (!out.ok) {
    appendCard("Verification request failed", out.message, "ERROR", "#f85149");
    return;
  }
  watchTask(out.data);
}

/* kind: "submit" (follows the selected mode), "review" or "preview" (explicit actions). */
async function runTask(kind) {
  const input = byId("task-input");
  let instruction = input.value.trim();
  if (!instruction) return;
  input.value = "";

  let effective = kind === "submit" ? approvalMode : kind;
  if (instruction.startsWith("/plan ")) {
    effective = "plan";
    instruction = instruction.slice(6).trim();
  } else if (instruction.startsWith("/review ")) {
    effective = "review";
    instruction = instruction.slice(8).trim();
  } else if (instruction.startsWith("/preview ")) {
    effective = "preview";
    instruction = instruction.slice(9).trim();
  }
  if (!instruction) return;
  if (currentSessionId) {
    upsertLocalSession(currentSessionId, {
      last_instruction: instruction,
      updated_at: new Date().toISOString()
    });
    updateActiveSessionHeader(currentSessionId, instruction);
    loadSessions();
  }
  appendStreamingBubble("user", "คุณ (User)", instruction, MODE_LABELS[effective] || String(effective).toUpperCase());

  try {
    if (effective === "plan") {
      const cardId = "plan-" + Date.now();
      appendCollapsibleToolCard("ArchitectPlanner", "กำลังวิเคราะห์โครงสร้างโปรเจกต์และสร้าง ExecutionPlan...", instruction, "RUNNING", cardId);
      const out = await postAPI("/api/plan", { instruction }, false);
      if (!out.ok) {
        appendCollapsibleToolCard("ArchitectPlanner", "วางแผนไม่สำเร็จ", out.message, "ERROR", cardId);
        throw new Error(out.message);
      }
      appendCollapsibleToolCard("ArchitectPlanner", "สร้าง ExecutionPlan เรียบร้อย", instruction, "DONE", cardId);
      appendPlanCard(out.data);
      if (currentSessionId && out.data) {
        appendLocalSessionTurn(currentSessionId, {
          instruction,
          summary: "ExecutionPlan: " + (out.data.objective || out.data.plan_id || "วางแผนสำเร็จ"),
          success: true,
          applied: false
        });
        loadSessions();
      }
      return;
    }
    const runMode = effective === "unverified" ? "unverified" :
      (effective === "review" || effective === "preview") ? effective : "execute";
    const out = await postAPI("/api/run", {
      request_id: crypto.randomUUID(),
      session_id: currentSessionId,
      instruction,
      mode: runMode
    }, true);
    if (!out.ok) {
      if (out.error === "untrusted") {
        appendCard("ยังไม่ได้ trust โปรเจกต์", "โหมดนี้ต้อง trust โปรเจกต์ก่อน — ใช้ plan, review, preview หรือ unverified ได้โดยไม่ต้อง trust", "UNTRUSTED", "#f59e0b");
        return;
      }
      throw new Error(out.message);
    }
    if (currentSessionId && out.data && out.data.id) {
      appendLocalSessionTurn(currentSessionId, {
        task_id: out.data.id,
        instruction,
        summary: "Task " + out.data.id + " (" + runMode + ")",
        success: true,
        applied: false
      });
      loadSessions();
    }
    watchTask(out.data);
  } catch (error) {
    appendCard("Task request failed", error.message, "ERROR", "#f85149");
  }
}

async function loadTasks() {
  const response = await apiFetch("/v1/tasks");
  if (response.ok) {
    const jobs = await response.json() || [];
    renderTaskList(jobs);
    loadSessions();

    // If user had a selected task persisted in localStorage, restore it first
    if (!currentArtifactJob && selectedTaskId) {
      const persisted = jobs.find(j => j.id === selectedTaskId);
      if (persisted) {
        populateArtifactFromJob(persisted);
      }
    }

    for (const job of jobs.slice(0, 6)) {
      if (activeStates.has(job.status)) {
        watchTask(job);
      } else if (!currentArtifactJob && job.report && (job.report.diff || (job.report.plan && job.report.plan.affected_files))) {
        populateArtifactFromJob(job);
      }
    }
  }
}

/* ============================================================================
 * 9. 24/7 AUTONOMOUS RUNTIME, GOAL BACKLOG, SCHEDULES & PROCEDURAL MEMORY
 * ============================================================================ */
let currentGovernorPaused = false;

async function openAutonomyDialog() {
  byId("autonomy-dialog").classList.remove("hidden");
  await loadAutonomyState();
}

function closeAutonomyDialog() {
  byId("autonomy-dialog").classList.add("hidden");
}

async function loadAutonomyState() {
  const out = await getAPI("/api/autonomy");
  if (!out.ok || !out.data) return;
  const gov = out.data.governor || {};
  const goals = out.data.goals || [];
  const schedules = out.data.schedules || [];
  const episodes = out.data.episodes || [];
  const skills = out.data.skills || [];

  currentGovernorPaused = !!gov.paused;
  const govBadge = byId("gov-status-badge");
  const govReason = byId("gov-reason-text");
  const govBtn = byId("btn-gov-toggle");
  const sidePill = byId("sidebar-autonomy-pill");
  const countBadge = byId("autonomy-count-badge");

  if (govBadge) {
    govBadge.textContent = currentGovernorPaused ? "PAUSED (CIRCUIT OPEN)" : "ACTIVE (24/7 READY)";
    govBadge.className = "font-mono text-[11px] px-2 py-0.5 rounded " + (currentGovernorPaused ? "bg-amber-500/20 text-amber-300" : "bg-emerald-500/20 text-emerald-300");
  }
  if (govReason) {
    govReason.textContent = gov.pause_reason ? ("สาเหตุที่หยุด: " + gov.pause_reason) : ("รันใน Isolated Git Worktree เสมอ · โควตาสูงสุด " + (gov.max_tasks_per_hour || 6) + " งาน/ชม. · ล้มเหลวติดกัน " + (gov.consecutive_failures || 0) + "/" + (gov.max_consecutive_failures || 3));
  }
  if (govBtn) {
    govBtn.textContent = currentGovernorPaused ? "▶️ เปิดทำงาน 24/7 ต่อ (Resume)" : "⏸️ หยุดชั่วคราว (Pause)";
  }
  if (sidePill) {
    sidePill.textContent = currentGovernorPaused ? "PAUSED" : "ON";
    sidePill.className = "text-[10px] px-1.5 py-0.5 rounded " + (currentGovernorPaused ? "bg-amber-500/20 text-amber-300" : "bg-emerald-500/20 text-emerald-300");
  }
  const activeCount = goals.filter(g => g.status === "pending" || g.status === "running").length + schedules.filter(s => s.enabled).length;
  if (countBadge) countBadge.textContent = String(activeCount);
  if (byId("autonomy-skill-count")) byId("autonomy-skill-count").textContent = skills.length + " skills · " + episodes.length + " episodes";

  renderAutonomyGoals(goals);
  renderAutonomySchedules(schedules);
  renderAutonomyEpisodes(episodes, skills);
}

function renderAutonomyGoals(goals) {
  const list = byId("autonomy-goal-list");
  if (!list) return;
  list.replaceChildren();
  if (!goals.length) {
    list.appendChild(mk("div", "text-[11px] text-slate-500 font-mono py-2", "ยังไม่มี Goal ในคิว — เพิ่มเป้าหมายเพื่อให้ Agent ทยอยทำเองเมื่อเครื่องว่าง"));
    return;
  }
  goals.forEach(g => {
    const row = mk("div", "flex items-center justify-between gap-2 px-3 py-2 rounded-xl bg-[#11141B] border border-white/[0.06]");
    const left = mk("div", "min-w-0 flex-1");
    left.append(
      mk("div", "text-xs font-semibold text-slate-200 truncate", g.title + " [" + g.status.toUpperCase() + "]"),
      mk("div", "text-[11px] text-slate-400 font-mono truncate", g.instruction)
    );
    const delBtn = mk("button", "px-2 py-1 rounded bg-rose-500/15 hover:bg-rose-500/25 text-rose-300 text-[10px] font-mono shrink-0", "ลบ");
    delBtn.onclick = () => deleteAutonomyGoal(g.id);
    row.append(left, delBtn);
    list.appendChild(row);
  });
}

function renderAutonomySchedules(schedules) {
  const list = byId("autonomy-schedule-list");
  if (!list) return;
  list.replaceChildren();
  if (!schedules.length) {
    list.appendChild(mk("div", "text-[11px] text-slate-500 font-mono py-2", "ยังไม่มีตารางงานอัตโนมัติ (Recurring Schedule)"));
    return;
  }
  schedules.forEach(s => {
    const row = mk("div", "flex items-center justify-between gap-2 px-3 py-2 rounded-xl bg-[#11141B] border border-white/[0.06]");
    const left = mk("div", "min-w-0 flex-1");
    const mins = Math.round((s.interval_sec || 3600) / 60);
    left.append(
      mk("div", "text-xs font-semibold text-slate-200 truncate", s.name + " (ทุก " + mins + " นาที)"),
      mk("div", "text-[11px] text-slate-400 font-mono truncate", s.instruction)
    );
    const right = mk("div", "flex items-center gap-1.5 shrink-0");
    const toggleBtn = mk("button", "px-2 py-1 rounded text-[10px] font-mono " + (s.enabled ? "bg-emerald-500/20 text-emerald-300" : "bg-white/[0.06] text-slate-400"), s.enabled ? "ON" : "OFF");
    toggleBtn.onclick = () => toggleAutonomySchedule(s.id, !s.enabled);
    const delBtn = mk("button", "px-2 py-1 rounded bg-rose-500/15 hover:bg-rose-500/25 text-rose-300 text-[10px] font-mono", "ลบ");
    delBtn.onclick = () => deleteAutonomySchedule(s.id);
    right.append(toggleBtn, delBtn);
    row.append(left, right);
    list.appendChild(row);
  });
}

function renderAutonomyEpisodes(episodes, skills) {
  const list = byId("autonomy-episode-list");
  if (!list) return;
  list.replaceChildren();
  if (skills && skills.length > 0) {
    const skillRow = mk("div", "px-3 py-2 rounded-xl bg-amber-500/10 border border-amber-500/25 text-[11px] font-mono text-amber-200", "✨ Learned Procedural Skills (.apw/skills): " + skills.join(", "));
    list.appendChild(skillRow);
  }
  if (!episodes || !episodes.length) {
    list.appendChild(mk("div", "text-[11px] text-slate-500 font-mono py-2", "ยังไม่มี Episodic Memory — เมื่อ Agent ทำงานเสร็จจะสกัดบทเรียนเก็บไว้โดยอัตโนมัติ"));
    return;
  }
  episodes.slice(0, 8).forEach(ep => {
    const row = mk("div", "px-3 py-2 rounded-xl bg-[#11141B] border border-white/[0.06] space-y-0.5");
    const st = ep.status || ep.outcome || "done";
    const ok = st === "succeeded" || st === "ready_to_apply";
    row.append(
      mk("div", "text-[11px] font-mono font-semibold " + (ok ? "text-emerald-300" : "text-rose-300"), "[" + st.toUpperCase() + "] " + ep.instruction),
      mk("div", "text-[11px] text-slate-300", ep.lesson || "")
    );
    list.appendChild(row);
  });
}

async function toggleGovernorPause() {
  await postAPI("/api/autonomy/governor", {
    paused: !currentGovernorPaused,
    pause_reason: !currentGovernorPaused ? "หยุดชั่วคราวโดยผู้ใช้ผ่าน Desktop UI" : ""
  }, false);
  await loadAutonomyState();
}

async function addAutonomyGoal() {
  const titleEl = byId("goal-title-input");
  const instrEl = byId("goal-instr-input");
  const title = (titleEl.value || "").trim();
  const instruction = (instrEl.value || "").trim();
  if (!title || !instruction) return;
  const out = await postAPI("/api/autonomy/goals", { title, instruction, priority: 10 }, false);
  if (out.ok) {
    titleEl.value = "";
    instrEl.value = "";
    await loadAutonomyState();
  }
}

async function deleteAutonomyGoal(id) {
  await postAPI("/api/autonomy/goals", { action: "delete", id }, false);
  await loadAutonomyState();
}

async function addAutonomySchedule() {
  const nameEl = byId("sched-name-input");
  const instrEl = byId("sched-instr-input");
  const intEl = byId("sched-interval-input");
  const name = (nameEl.value || "").trim();
  const instruction = (instrEl.value || "").trim();
  const interval_sec = parseInt(intEl.value || "3600", 10);
  if (!name || !instruction) return;
  const out = await postAPI("/api/autonomy/schedules", { name, instruction, interval_sec, mode: "execute" }, false);
  if (out.ok) {
    nameEl.value = "";
    instrEl.value = "";
    await loadAutonomyState();
  }
}

async function toggleAutonomySchedule(id, enabled) {
  await postAPI("/api/autonomy/schedules", { action: "toggle", id, enabled }, false);
  await loadAutonomyState();
}

async function deleteAutonomySchedule(id) {
  await postAPI("/api/autonomy/schedules", { action: "delete", id }, false);
  await loadAutonomyState();
}

async function extractSkillFromTask(job) {
  const out = await postAPI("/api/autonomy/skills", {
    task_id: job.id,
    slug: "skill-" + job.id,
    title: job.instruction,
    instruction: job.instruction
  }, false);
  if (!out.ok) {
    appendCard("สกัด Skill ไม่สำเร็จ", out.message, "ERROR", "#f85149");
    return;
  }
  appendCollapsibleToolCard("ProceduralSkill", "บันทึกทักษะใหม่ลง " + (out.data && out.data.skill_path ? out.data.skill_path : ".apw/skills"), "งานต้นแบบ: " + job.instruction, "DONE");
  await loadAutonomyState();
}

initResizablePanels();
applyModeUI();
(async () => {
  await loadStatus();
  await ensureSession();
  loadSessions();
  loadTasks();
  loadAutonomyState();
  // One look at approvals on load; polling continues only while a task or Apply is active.
  if (await loadApprovals() > 0) startApprovalPolling();
})();
</script>
</body>
</html>`
