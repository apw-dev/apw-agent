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
  }
  /* Superellipse / Squircle smoothing & custom scrollbars */
  .squircle {
    border-radius: 18px;
  }
  .squircle-sm {
    border-radius: 13px;
  }
  .squircle-lg {
    border-radius: 22px;
  }
  ::-webkit-scrollbar {
    width: 6px;
    height: 6px;
  }
  ::-webkit-scrollbar-track {
    background: transparent;
  }
  ::-webkit-scrollbar-thumb {
    background: rgba(255, 255, 255, 0.12);
    border-radius: 9999px;
  }
  ::-webkit-scrollbar-thumb:hover {
    background: rgba(255, 255, 255, 0.24);
  }
  .diff-table {
    width: 100%;
    border-collapse: collapse;
    font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
    font-size: 12.5px;
    line-height: 1.55;
  }
  .diff-line-add {
    background: rgba(16, 185, 129, 0.12);
    color: #A7F3D0;
  }
  .diff-line-del {
    background: rgba(244, 63, 94, 0.13);
    color: #FECDD3;
  }
  .diff-line-hunk {
    background: rgba(59, 130, 246, 0.12);
    color: #93C5FD;
  }
  .diff-num {
    width: 44px;
    min-width: 44px;
    padding: 1px 8px;
    text-align: right;
    color: rgba(148, 163, 184, 0.45);
    user-select: none;
    border-right: 1px solid rgba(255, 255, 255, 0.06);
  }
  .diff-code {
    padding: 1px 12px;
    white-space: pre-wrap;
    word-break: break-all;
  }
  @keyframes pulseGlow {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.45; }
  }
  .pulse-dot {
    animation: pulseGlow 1.6s infinite ease-in-out;
  }
</style>
</head>
<body class="bg-[#0B0D11] text-[#E6EDF3] font-sans h-screen w-screen overflow-hidden select-none flex flex-col">

  <!-- MAIN 3-PANE SPLIT VIEW CONTAINER -->
  <div class="flex-1 flex min-h-0 relative overflow-hidden">

    <!-- =================================================================== -->
    <!-- 1. LEFT SIDEBAR (Collapsible: Sessions, Workspace, APW.md Status) -->
    <!-- =================================================================== -->
    <aside id="left-sidebar" class="w-80 bg-[#11141B] border-r border-white/[0.08] flex flex-col transition-all duration-300 ease-out shrink-0 select-text">
      <!-- Brand Header -->
      <div class="p-4 border-b border-white/[0.07] flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-9 h-9 squircle-sm bg-gradient-to-br from-[#E07A5F] to-[#D97706] flex items-center justify-center font-mono font-bold text-xs text-white shadow-lg shadow-[#E07A5F]/20">
            APW
          </div>
          <div>
            <div class="text-sm font-semibold tracking-tight text-white flex items-center gap-2">
              Apiwat Agent
              <span id="hdr-version" class="text-[10px] font-mono px-2 py-0.5 rounded-full bg-white/[0.06] text-slate-400">v0.1.0</span>
            </div>
            <div class="text-[11px] text-slate-400 font-mono">Dual-Brain Developer OS</div>
          </div>
        </div>
        <button onclick="toggleLeftSidebar()" title="ซ่อนแถบด้านซ้าย (Ctrl+B)" class="p-1.5 rounded-xl text-slate-400 hover:text-white hover:bg-white/[0.06] transition">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M11 19l-7-7 7-7m8 14l-7-7 7-7"/></svg>
        </button>
      </div>

      <!-- Workspace & APW.md Status Section -->
      <div class="p-3.5 space-y-2.5 border-b border-white/[0.07]">
        <!-- Workspace Card -->
        <div class="squircle-sm bg-[#161A23] border border-white/[0.07] p-3 space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-[11px] uppercase tracking-wider font-semibold text-slate-400">Workspace ปัจจุบัน</span>
            <span id="ws-stack-badge" class="text-[10px] font-mono px-2 py-0.5 rounded-md bg-sky-500/15 text-sky-300 border border-sky-500/25">Go</span>
          </div>
          <div id="hdr-workspace" class="text-xs font-mono text-slate-200 truncate" title="">กำลังโหลด...</div>
          <div class="flex items-center justify-between pt-1 border-t border-white/[0.05] text-[11px]">
            <span id="ws-trust-badge" class="flex items-center gap-1.5 text-emerald-400 font-mono">
              <span class="w-2 h-2 rounded-full bg-emerald-400"></span> Trusted
            </span>
            <button onclick="onTrustControl()" id="btn-trust-toggle" class="text-[11px] text-slate-400 hover:text-white underline decoration-dotted">Trust...</button>
          </div>
        </div>

        <!-- APW.md (Project Instructions) Status Card -->
        <div id="instr-card" class="squircle-sm bg-[#161A23] border border-white/[0.07] p-3 transition hover:border-white/[0.15]">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span id="instr-dot" class="w-2 h-2 rounded-full bg-slate-500"></span>
              <span class="text-xs font-mono font-semibold text-slate-200">APW.md</span>
            </div>
            <span id="instr-badge" class="text-[10px] font-mono px-2 py-0.5 rounded-md bg-slate-700/50 text-slate-300">Checking...</span>
          </div>
          <p id="instr-desc" class="text-[11px] text-slate-400 mt-1.5 leading-relaxed">ตรวจสอบกฎและบริบทของโปรเจกต์ (APW.md; ถ้าไม่มีจะอ่าน AGENTS.md)</p>
          <div class="mt-2 flex items-center gap-2">
            <button onclick="openInstructionsInArtifactPane()" class="flex-1 text-[11px] font-medium py-1.5 px-2.5 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] text-slate-200 transition text-center">
              เปิดดู / ตรวจสอบ
            </button>
            <button id="btn-create-instr" onclick="initInstructions()" class="hidden text-[11px] font-medium py-1.5 px-2.5 rounded-lg bg-[#E07A5F]/20 hover:bg-[#E07A5F]/30 text-[#E07A5F] border border-[#E07A5F]/30 transition">
              + สร้าง APW.md
            </button>
          </div>
        </div>
      </div>

      <!-- New Session Button -->
      <div class="px-3.5 pt-3 pb-2 flex items-center justify-between">
        <span class="text-[11px] uppercase tracking-wider font-semibold text-slate-400">Sessions & งานย้อนหลัง</span>
        <button onclick="createNewSession()" class="text-xs font-medium px-2.5 py-1 rounded-lg bg-[#E07A5F] hover:bg-[#EB8C73] text-white shadow-sm transition flex items-center gap-1">
          <span>+</span> Session ใหม่
        </button>
      </div>

      <!-- Session & Task History List -->
      <div id="session-list" class="flex-1 overflow-y-auto px-3 py-1 space-y-1.5">
        <!-- Populated dynamically -->
      </div>

      <!-- Sidebar Footer: Daemon & AST Symbols Quick Access -->
      <div class="p-3 border-t border-white/[0.07] bg-[#0E1117] space-y-2 text-[11px] font-mono">
        <div class="flex items-center justify-between text-slate-400">
          <span id="pill-daemon" class="truncate">Daemon: Checking...</span>
          <button onclick="openSymbolsArtifact()" class="text-sky-400 hover:text-sky-300 shrink-0">AST Symbols</button>
        </div>
        <div class="flex items-center justify-between text-slate-400">
          <span>Apply อัตโนมัติ:</span>
          <span id="apply-auto-status" class="text-amber-300">ปิด</span>
        </div>
      </div>
    </aside>

    <!-- =================================================================== -->
    <!-- 2. CENTER CHAT PANE (Streaming Chat, Collapsible Tool Cards, HITL)  -->
    <!-- =================================================================== -->
    <main class="flex-1 flex flex-col min-w-0 bg-[#0B0D11] select-text">
      <!-- Top Navigation Bar -->
      <header class="h-14 px-4 border-b border-white/[0.07] bg-[#11141B]/80 backdrop-blur flex items-center justify-between shrink-0">
        <div class="flex items-center gap-3 min-w-0">
          <button id="btn-show-left" onclick="toggleLeftSidebar()" title="แสดงแถบด้านซ้าย (Ctrl+B)" class="hidden p-2 rounded-xl bg-white/[0.05] text-slate-300 hover:text-white hover:bg-white/[0.1] transition">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16"/></svg>
          </button>
          <div class="flex items-center gap-2 truncate">
            <span class="text-xs font-semibold text-white">Session:</span>
            <span id="active-session-badge" class="text-xs font-mono px-2.5 py-1 rounded-lg bg-[#161A23] border border-white/[0.07] text-amber-300">local-default</span>
          </div>
          <div class="hidden md:flex items-center gap-2 text-[11px] font-mono">
            <span id="pill-brain1" class="px-2.5 py-1 rounded-lg bg-[#161A23] border border-white/[0.07] text-emerald-300">⚡ Reflex</span>
            <span id="pill-brain2" class="px-2.5 py-1 rounded-lg bg-[#161A23] border border-white/[0.07] text-sky-300">🧠 Architect</span>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <!-- Toggle Right Artifact/Diff Pane -->
          <button onclick="toggleRightPane()" id="btn-toggle-right" class="px-3 py-1.5 rounded-xl bg-[#161A23] hover:bg-[#1C212D] text-slate-200 border border-white/[0.08] text-xs font-medium flex items-center gap-1.5 transition">
            <span>Artifact / Diff</span>
            <span id="artifact-count-badge" class="px-1.5 py-0.2 rounded-full bg-[#E07A5F]/25 text-[#E07A5F] font-mono text-[10px]">0</span>
          </button>
        </div>
      </header>

      <!-- Hidden Legacy Compatibility Elements (keeps existing status hooks intact) -->
      <pre id="banner-box" class="hidden"></pre>
      <div id="symbol-list" class="hidden"></div>

      <!-- Streaming Chat & Tool Execution Feed -->
      <div id="feed" class="flex-1 overflow-y-auto p-6 space-y-4 max-w-4xl w-full mx-auto">
        <!-- Welcome Squircle Card -->
        <div class="squircle bg-[#161A23] border border-white/[0.08] p-5 shadow-xl">
          <div class="flex items-start justify-between gap-4">
            <div class="space-y-1.5">
              <div class="inline-flex items-center gap-2 text-xs font-mono text-[#E07A5F] bg-[#E07A5F]/10 px-2.5 py-1 rounded-full border border-[#E07A5F]/20">
                <span>⚡ System 1 (Reflex) + 🧠 System 2 (Architect) + 🔬 Surgeon</span>
              </div>
              <h2 class="text-base font-semibold text-white">พร้อมทำงานในโหมด 3-Pane Split View & Human-in-the-Loop</h2>
              <p class="text-xs text-slate-400 leading-relaxed">
                สั่งงานด้วยภาษาธรรมชาติเพื่อตรวจโค้ด (Review), เตรียม Diff ใน Worktree แยก (Preview), หรือสั่งผ่าตัดโค้ดและรันทดสอบอัตโนมัติ (Execute)
                เมื่อมีการเรียกใช้ Tool หรือแก้ไขไฟล์ แผง <strong>Artifact / Diff</strong> ด้านขวาจะสไลด์ออกมาอัตโนมัติ
              </p>
            </div>
          </div>
        </div>

        <!-- Turns of the selected session (filled with textContent only) -->
        <div id="session-history" class="space-y-3"></div>

        <!-- Pending Approvals Container (Human-in-the-loop Cards) -->
        <div id="approval-feed" class="space-y-3"></div>

        <!-- Live Task & Tool Execution Stream for the current session -->
        <div id="live-feed" class="space-y-4"></div>
      </div>

      <!-- Bottom Composer (Superellipse / Squircle) -->
      <div class="p-4 pt-2 bg-gradient-to-t from-[#0B0D11] via-[#0B0D11] to-transparent shrink-0">
        <div class="max-w-4xl mx-auto squircle bg-[#161A23] border border-white/[0.1] p-3 shadow-2xl focus-within:border-[#E07A5F]/60 transition">
          <textarea
            id="task-input"
            rows="2"
            placeholder="พิมพ์คำสั่งให้ Agent ทำงาน เช่น ตรวจสอบโค้ดในโปรเจกต์, เพิ่มฟังก์ชันใหม่, หรือวางแผนสถาปัตยกรรม... (กด Enter เพื่อส่ง, Shift+Enter ขึ้นบรรทัดใหม่)"
            class="w-full bg-transparent text-sm text-white placeholder-slate-500 focus:outline-none resize-none font-sans leading-relaxed px-1"
            onkeydown="if(event.key==='Enter' && !event.shiftKey){ event.preventDefault(); runTask('submit'); }"
          ></textarea>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-2 mt-1 border-t border-white/[0.06]">
            <!-- Left Mode Selector & Quick Buttons -->
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="text-[11px] font-mono text-slate-400 mr-1">โหมด:</span>
              <select id="approval-mode" onchange="onApprovalModeChange(this.value)" class="text-xs font-mono bg-[#11141B] text-slate-200 border border-white/[0.1] rounded-xl px-2.5 py-1.5 focus:outline-none focus:border-[#E07A5F]">
                <option value="ask">ask — verify แล้วถามก่อน apply</option>
                <option value="auto">auto — verify แล้ว apply อัตโนมัติ</option>
                <option value="plan">plan — วางแผนอย่างเดียว</option>
                <option value="unverified">unverified — แก้โดยไม่รัน build/test แล้วถามก่อน apply</option>
              </select>

              <button onclick="runTask('review')" class="text-xs px-2.5 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] text-slate-300 transition">
                Review
              </button>
              <button onclick="runTask('preview')" class="text-xs px-2.5 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] text-slate-300 transition">
                Preview Diff
              </button>
              <button onclick="runVerify()" class="text-xs px-2.5 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] text-emerald-300 transition">
                Verify (Reflex)
              </button>
            </div>

            <!-- Submit Button -->
            <button onclick="runTask('submit')" class="px-4 py-1.5 rounded-xl bg-[#E07A5F] hover:bg-[#EB8C73] text-white text-xs font-semibold shadow-lg shadow-[#E07A5F]/25 transition flex items-center gap-1.5">
              <span>ส่งคำสั่ง</span>
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M5 12h14M12 5l7 7-7 7"/></svg>
            </button>
          </div>
          <div id="mode-desc" class="pt-1.5 text-[11px] text-slate-400 leading-relaxed"></div>
        </div>
      </div>
    </main>

    <!-- =================================================================== -->
    <!-- 3. RIGHT ARTIFACT / DIFF PANE (Auto-Slide-Out Drawer)               -->
    <!-- =================================================================== -->
    <aside id="right-pane" class="w-0 overflow-hidden bg-[#11141B] border-l border-white/[0.08] flex flex-col transition-all duration-300 ease-out shrink-0 select-text">
      <!-- Artifact Header -->
      <div class="h-14 px-4 border-b border-white/[0.07] flex items-center justify-between shrink-0">
        <div class="flex items-center gap-2.5 min-w-0">
          <span class="w-2.5 h-2.5 rounded-full bg-[#E07A5F]"></span>
          <h3 id="artifact-title" class="text-xs font-mono font-semibold text-white truncate">Artifact & Diff Inspector</h3>
        </div>
        <div class="flex items-center gap-1.5">
          <!-- View Mode Tabs -->
          <div class="flex bg-[#161A23] p-0.5 rounded-xl border border-white/[0.07] text-[11px] font-medium">
            <button id="tab-btn-diff" onclick="switchArtifactTab('diff')" class="px-2.5 py-1 rounded-lg bg-[#E07A5F] text-white transition">Diff View</button>
            <button id="tab-btn-code" onclick="switchArtifactTab('code')" class="px-2.5 py-1 rounded-lg text-slate-400 hover:text-white transition">Code</button>
            <button id="tab-btn-preview" onclick="switchArtifactTab('preview')" class="px-2.5 py-1 rounded-lg text-slate-400 hover:text-white transition">HTML Preview</button>
          </div>
          <button onclick="closeRightPane()" title="ซ่อนแผง Artifact" class="p-1.5 rounded-xl text-slate-400 hover:text-white hover:bg-white/[0.06] transition">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
      </div>

      <!-- Affected Files Bar -->
      <div id="artifact-files-bar" class="px-4 py-2 bg-[#161A23]/70 border-b border-white/[0.06] flex items-center justify-between gap-2 text-xs">
        <div class="flex items-center gap-2 min-w-0 flex-1">
          <span class="text-[11px] text-slate-400 shrink-0">ไฟล์:</span>
          <select id="artifact-file-select" onchange="onSelectArtifactFile(this.value)" class="bg-[#11141B] text-slate-200 font-mono text-xs border border-white/[0.08] rounded-lg px-2 py-1 truncate flex-1 focus:outline-none">
            <option value="">(เลือกไฟล์เพื่อตรวจสอบ)</option>
          </select>
        </div>
        <span id="artifact-status-pill" class="text-[10px] font-mono px-2 py-0.5 rounded-md bg-white/[0.06] text-slate-300 shrink-0">READY</span>
      </div>

      <!-- TAB 1: DIFF VIEW (Line-by-Line Comparison Table) -->
      <div id="artifact-tab-diff" class="flex-1 overflow-auto p-3 space-y-3">
        <div id="diff-empty-state" class="squircle bg-[#161A23] border border-white/[0.06] p-6 text-center text-xs text-slate-400">
          ยังไม่มีตารางเปรียบเทียบโค้ด (Diff) — สั่ง <code class="text-amber-300">Preview Diff</code> หรือ <code class="text-emerald-300">Execute</code> เพื่อดูการเปลี่ยนแปลงแบบบรรทัดต่อบรรทัด
        </div>
        <div id="diff-table-container" class="squircle-sm overflow-hidden border border-white/[0.08] bg-[#0D1017] hidden">
          <table class="diff-table">
            <tbody id="diff-tbody"></tbody>
          </table>
        </div>
        <div id="acceptance-box" class="hidden squircle-sm bg-[#161A23] border border-white/[0.07] p-3.5 space-y-1.5">
          <div class="text-xs font-semibold text-emerald-300 font-mono">✓ หลักฐานการตรวจสอบ (Acceptance & Reflex Evidence)</div>
          <pre id="acceptance-pre" class="text-[11px] font-mono text-slate-300 whitespace-pre-wrap leading-relaxed"></pre>
        </div>
      </div>

      <!-- TAB 2: CODE / FILE VIEWER -->
      <div id="artifact-tab-code" class="flex-1 overflow-auto p-3 hidden">
        <div class="squircle-sm border border-white/[0.08] bg-[#0D1017] overflow-hidden">
          <div class="px-3.5 py-2 bg-[#161A23] border-b border-white/[0.06] flex items-center justify-between">
            <span class="flex items-center gap-2 min-w-0">
              <span id="code-viewer-filename" class="text-xs font-mono text-slate-300 truncate">code.go</span>
              <span id="code-viewer-meta" class="text-[10px] font-mono text-slate-500 shrink-0"></span>
            </span>
            <button onclick="renderCodeAsHTMLPreview()" class="text-[11px] text-[#E07A5F] hover:underline">ส่งไปเรนเดอร์ใน HTML Preview →</button>
          </div>
          <pre id="code-viewer-pre" class="p-4 text-xs font-mono text-slate-200 overflow-x-auto leading-relaxed whitespace-pre">เลือกไฟล์จากด้านบนหรือคลิกการ์ด APW.md เพื่อดูเนื้อหาโค้ด</pre>
        </div>
      </div>

      <!-- TAB 3: HTML PREVIEW (Sandboxed Live Renderer) -->
      <div id="artifact-tab-preview" class="flex-1 flex flex-col p-3 hidden">
        <div class="squircle-sm border border-white/[0.08] bg-white flex-1 overflow-hidden flex flex-col">
          <div class="px-3 py-1.5 bg-[#161A23] border-b border-white/[0.08] flex items-center justify-between text-[11px] font-mono text-slate-300">
            <span>Sandboxed HTML Preview</span>
            <button onclick="loadSampleHTMLPreview()" class="text-sky-400 hover:underline">โหลดตัวอย่าง UI Artifact</button>
          </div>
          <iframe id="html-preview-frame" sandbox="allow-scripts" class="w-full flex-1 border-0 bg-white"></iframe>
        </div>
      </div>

      <!-- Right Pane Footer: Explicit Apply / Resume / Cancel Bar -->
      <div id="artifact-action-bar" class="p-3 border-t border-white/[0.07] bg-[#161A23] flex items-center justify-between gap-2 shrink-0">
        <div class="text-[11px] text-slate-400 font-mono truncate" id="artifact-task-id">ไม่มีงานที่รอ Apply</div>
        <div id="artifact-buttons" class="flex items-center gap-2"></div>
      </div>
    </aside>

  </div>

  <!-- TRUST CONFIRMATION DIALOG (in-page; opened by the trust control or an "untrusted" 409) -->
  <div id="trust-dialog" class="hidden fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4 select-text" role="dialog" aria-modal="true" aria-labelledby="trust-title">
    <div class="squircle bg-[#161A23] border border-amber-500/40 shadow-2xl w-full max-w-lg p-5 space-y-3">
      <h3 id="trust-title" class="text-sm font-semibold text-amber-300">ให้ Trust โปรเจกต์นี้หรือไม่?</h3>
      <div id="trust-workspace" class="text-[11px] font-mono text-slate-300 truncate"></div>
      <ul class="text-xs text-slate-200 leading-relaxed space-y-1.5 list-disc pl-5">
        <li>APW จะรันคำสั่ง install / build / test ของโปรเจกต์นี้เอง ใน isolated worktree ด้วยสิทธิ์ของผู้ใช้ปัจจุบัน</li>
        <li>ให้ trust เฉพาะโค้ดที่คุณเชื่อถือเท่านั้น เพราะสคริปต์ของโปรเจกต์ทำอะไรก็ได้ภายใต้สิทธิ์ของคุณ</li>
        <li>ถ้าไม่ trust คุณยังใช้ plan, review, preview และโหมด unverified (แก้โดยไม่รัน build/test) ได้</li>
      </ul>
      <p id="trust-sandbox-hint" class="hidden text-[11px] text-sky-300 leading-relaxed">
        ตอนนี้ sandbox ปิดอยู่ (sandbox mode: off) แนะนำให้ตั้ง <code class="text-amber-200">"sandbox": {"mode": "restricted"}</code> ใน apw.json ก่อน trust
      </p>
      <div class="flex items-center justify-end gap-2 pt-1">
        <button id="trust-cancel" onclick="closeTrustDialog(false)" class="px-3.5 py-1.5 rounded-xl text-xs font-semibold bg-white/[0.07] hover:bg-white/[0.12] text-slate-200 transition">ไม่ trust</button>
        <button id="trust-confirm" onclick="closeTrustDialog(true)" class="px-3.5 py-1.5 rounded-xl text-xs font-semibold bg-amber-600 hover:bg-amber-500 text-white transition">Trust โปรเจกต์นี้</button>
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
let leftSidebarOpen = true;
let rightPaneOpen = false;
let currentArtifactJob = null;
let lastStatus = {};
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
 * 1. 3-PANE SPLIT VIEW CONTROLS (Left Sidebar & Right Auto-Slide Artifact Pane)
 * ============================================================================ */
function toggleLeftSidebar() {
  const sb = byId("left-sidebar");
  const showBtn = byId("btn-show-left");
  leftSidebarOpen = !leftSidebarOpen;
  if (leftSidebarOpen) {
    sb.classList.remove("w-0", "opacity-0", "overflow-hidden", "border-r-0");
    sb.classList.add("w-80");
    showBtn.classList.add("hidden");
  } else {
    sb.classList.remove("w-80");
    sb.classList.add("w-0", "opacity-0", "overflow-hidden", "border-r-0");
    showBtn.classList.remove("hidden");
  }
}

function openRightPane(defaultTab) {
  const rp = byId("right-pane");
  rightPaneOpen = true;
  rp.classList.remove("w-0");
  rp.classList.add("w-[480px]", "xl:w-[560px]");
  if (defaultTab) switchArtifactTab(defaultTab);
}

function closeRightPane() {
  const rp = byId("right-pane");
  rightPaneOpen = false;
  rp.classList.remove("w-[480px]", "xl:w-[560px]");
  rp.classList.add("w-0");
}

function toggleRightPane() {
  if (rightPaneOpen) closeRightPane();
  else openRightPane();
}

function switchArtifactTab(tab) {
  ["diff", "code", "preview"].forEach(t => {
    const panel = byId("artifact-tab-" + t);
    const btn = byId("tab-btn-" + t);
    if (t === tab) {
      panel.classList.remove("hidden");
      btn.className = "px-2.5 py-1 rounded-lg bg-[#E07A5F] text-white transition";
    } else {
      panel.classList.add("hidden");
      btn.className = "px-2.5 py-1 rounded-lg text-slate-400 hover:text-white transition";
    }
  });
}

document.addEventListener("keydown", (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "b") {
    e.preventDefault();
    toggleLeftSidebar();
  }
  if (e.key === "Escape" && !byId("trust-dialog").classList.contains("hidden")) {
    closeTrustDialog(false);
  }
});

/* ============================================================================
 * 2. TRUST (explicit confirmation; never a blind toggle)
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
 * 5. COLLAPSIBLE TOOL CARDS & STREAMING MESSAGE RENDERER
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
    detailsEl.className = "squircle-sm bg-[#141821] border border-white/[0.07] overflow-hidden group transition";
  }

  const badgeColors = {
    RUNNING: "bg-amber-500/20 text-amber-300 border-amber-500/30",
    DONE: "bg-emerald-500/20 text-emerald-300 border-emerald-500/30",
    DENIED: "bg-rose-500/20 text-rose-300 border-rose-500/30",
    ERROR: "bg-rose-500/20 text-rose-300 border-rose-500/30"
  };
  const badgeCls = badgeColors[status] || "bg-sky-500/20 text-sky-300 border-sky-500/30";
  const dotCls = status === "RUNNING" ? "bg-amber-400 pulse-dot" : status === "DONE" ? "bg-emerald-400" : "bg-rose-400";

  const summary = mk("summary", "px-4 py-2.5 cursor-pointer flex items-center justify-between gap-3 hover:bg-white/[0.03] select-none list-none");
  const left = mk("div", "flex items-center gap-2.5 min-w-0");
  left.append(
    mk("span", "w-2 h-2 rounded-full shrink-0 " + dotCls),
    mk("span", "text-[11px] font-mono font-semibold px-2 py-0.5 rounded-md bg-white/[0.06] text-sky-300 shrink-0", toolName),
    mk("span", "text-xs text-slate-200 truncate tool-summary", summaryText)
  );
  const right = mk("div", "flex items-center gap-2 shrink-0");
  right.append(
    mk("span", "text-[10px] font-mono px-2 py-0.5 rounded-full border " + badgeCls, status),
    mk("span", "text-slate-400 text-xs group-open:rotate-180 transition-transform", "▾")
  );
  summary.append(left, right);
  const detail = mk("div", "px-4 py-3 border-t border-white/[0.06] bg-[#0D1017] font-mono text-xs text-slate-300 whitespace-pre-wrap leading-relaxed tool-detail",
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
  card.className = "squircle p-4 border " + (isUser
    ? "bg-[#1A202C]/70 border-sky-500/25 ml-8"
    : "bg-[#161A23] border-white/[0.08]");

  const header = document.createElement("div");
  header.className = "flex items-center justify-between mb-2";
  const left = document.createElement("span");
  left.className = "text-xs font-semibold " + (isUser ? "text-sky-300" : "text-[#E07A5F]");
  left.textContent = title;
  header.appendChild(left);

  if (badgeText) {
    const b = document.createElement("span");
    b.className = "text-[10px] font-mono px-2 py-0.5 rounded-full bg-white/[0.06] text-slate-300";
    b.textContent = badgeText;
    header.appendChild(b);
  }

  const bodyEl = document.createElement("div");
  bodyEl.className = "text-xs text-slate-200 whitespace-pre-wrap leading-relaxed font-sans";
  card.appendChild(header);
  card.appendChild(bodyEl);
  container.appendChild(card);

  const full = typeof text === "string" ? text : JSON.stringify(text, null, 2);
  if (isUser || full.length > 900) {
    bodyEl.textContent = full;
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
    return;
  }
  // Streaming typewriter effect for Agent replies
  let i = 0;
  const step = Math.max(4, Math.ceil(full.length / 45));
  const timer = setInterval(() => {
    i += step;
    if (i >= full.length) {
      bodyEl.textContent = full;
      clearInterval(timer);
    } else {
      bodyEl.textContent = full.slice(0, i) + "▋";
    }
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
  }, 16);
}

function appendCard(title, body, badgeText, badgeColor) {
  const scroller = byId("feed");
  const container = byId("live-feed") || scroller;
  const card = document.createElement("div");
  card.className = "squircle bg-[#161A23] border border-white/[0.08] p-4 space-y-2 shadow-lg";
  const h = document.createElement("div");
  h.className = "flex items-center justify-between gap-2";

  const tSpan = document.createElement("span");
  tSpan.className = "text-xs font-semibold text-white font-mono";
  tSpan.textContent = title;
  h.appendChild(tSpan);

  if (badgeText) {
    const bSpan = document.createElement("span");
    bSpan.textContent = badgeText;
    bSpan.className = "text-[10px] font-mono px-2.5 py-0.5 rounded-full text-white";
    bSpan.style.background = badgeColor || "#262c36";
    h.appendChild(bSpan);
  }

  const pre = document.createElement("pre");
  pre.className = "text-xs font-mono text-slate-300 whitespace-pre-wrap overflow-x-auto leading-relaxed bg-[#0E1117] p-3 squircle-sm border border-white/[0.05]";
  pre.textContent = typeof body === "string" ? body : JSON.stringify(body, null, 2);
  card.appendChild(h);
  card.appendChild(pre);
  container.appendChild(card);
  if (scroller) scroller.scrollTop = scroller.scrollHeight;
}

/* ============================================================================
 * 6. RIGHT ARTIFACT / DIFF RENDERER (Diff Table, Code Viewer, HTML Preview)
 * ============================================================================ */
function renderDiffTable(diffText) {
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

  const lines = diffText.split("\n");
  let oldLine = 0, newLine = 0;

  lines.forEach(line => {
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
}

function populateArtifactFromJob(job) {
  currentArtifactJob = job;
  const report = job.report || {};
  const plan = report.plan || {};
  const files = plan.affected_files || [];
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

  byId("artifact-count-badge").textContent = String(files.length || (report.diff ? 1 : 0));

  if (report.diff) {
    renderDiffTable(report.diff);
  }

  const accBox = byId("acceptance-box");
  const accPre = byId("acceptance-pre");
  if (report.acceptance) {
    accBox.classList.remove("hidden");
    accPre.textContent = typeof report.acceptance === "string" ? report.acceptance : JSON.stringify(report.acceptance, null, 2);
  } else {
    accBox.classList.add("hidden");
  }

  // Populate action buttons in Right Pane footer
  const btnWrap = byId("artifact-buttons");
  btnWrap.replaceChildren();
  const actions = activeStates.has(job.status) ? ["cancel"] :
    job.status === "ready_to_apply" ? ["apply"] :
    ["preview_ready", "failed", "interrupted", "cancelled"].includes(job.status) ? ["resume"] : [];

  actions.forEach(action => {
    const b = document.createElement("button");
    b.className = "px-3 py-1.5 rounded-xl text-xs font-semibold transition " +
      (action === "apply" ? "bg-emerald-600 hover:bg-emerald-500 text-white" :
       action === "resume" ? "bg-amber-600 hover:bg-amber-500 text-white" :
       "bg-rose-600/30 hover:bg-rose-600/50 text-rose-200");
    b.textContent = action === "apply" ? (unverifiedJob ? "✓ Apply (UNVERIFIED) ลง Workspace" : "✓ Apply ลง Workspace") :
      action === "resume" ? "↻ Resume & Verify" : "✕ Cancel";
    b.onclick = async () => {
      if (action === "apply") { await applyTask(job); return; }
      const out = await postAPI("/v1/tasks/" + encodeURIComponent(job.id) + "/" + action, {}, action === "resume");
      if (!out.ok) { appendCard("Task action failed", out.message, "ERROR", "#f85149"); return; }
      watchTask(out.data);
    };
    btnWrap.appendChild(b);
  });

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
 * 7. WORKSPACE, APW.MD (PROJECT INSTRUCTIONS) & SESSION SIDEBAR MANAGEMENT
 * ============================================================================ */
function setTrustBadge(trusted) {
  const trustEl = byId("ws-trust-badge");
  const dot = mk("span", "w-2 h-2 rounded-full " + (trusted ? "bg-emerald-400" : "bg-amber-400"));
  trustEl.className = "flex items-center gap-1.5 font-mono " + (trusted ? "text-emerald-400" : "text-amber-400");
  trustEl.replaceChildren(dot, document.createTextNode(trusted ? " Trusted Workspace" : " Untrusted (Safe Mode)"));
  byId("btn-trust-toggle").textContent = trusted ? "ยกเลิก trust" : "Trust...";
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

  byId("pill-brain1").textContent = "⚡ Brain 1: " + (s.brain1_model || s.brain1_name);
  byId("pill-brain2").textContent = "🧠 Brain 2: " + (s.brain2_model || s.brain2_name);
  byId("pill-daemon").textContent = "apwd: " + (s.daemon_running ? "PID " + s.daemon_pid + " (" + s.brain1_latency_ms + "ms)" : "Embedded");

  // APW.md (project instructions) status in Left Sidebar
  const dot = byId("instr-dot");
  const badge = byId("instr-badge");
  const desc = byId("instr-desc");
  const createBtn = byId("btn-create-instr");

  if (s.instructions_loaded) {
    dot.className = "w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_#34d399]";
    badge.className = "text-[10px] font-mono px-2 py-0.5 rounded-md bg-emerald-500/15 text-emerald-300 border border-emerald-500/30";
    badge.textContent = "Loaded (" + s.instructions_lines + " lines)";
    desc.textContent = "โหลด " + (s.instructions_path || "APW.md") + " (" + s.instructions_size + " bytes) เข้าสู่บริบทของ Agent แล้ว";
    createBtn.classList.add("hidden");
  } else {
    dot.className = "w-2 h-2 rounded-full bg-amber-400";
    badge.className = "text-[10px] font-mono px-2 py-0.5 rounded-md bg-amber-500/15 text-amber-300 border border-amber-500/30";
    badge.textContent = "Not Found";
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

function sessionItem(id, active, badgeText, subtitle) {
  const item = mk("div", "squircle-sm p-3 border transition cursor-pointer " +
    (active ? "bg-[#1C212D] border-[#E07A5F]/50" : "bg-[#161A23] border-white/[0.06] hover:border-white/[0.15]"));
  const top = mk("div", "flex items-center justify-between gap-2");
  top.append(
    mk("span", "text-xs font-mono font-semibold text-white truncate", id),
    mk("span", "text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.06] text-slate-300 shrink-0", badgeText)
  );
  item.append(top, mk("div", "text-[11px] text-slate-400 truncate mt-1", subtitle));
  item.onclick = () => selectSession(id);
  return item;
}

async function loadSessions() {
  const res = await apiFetch("/api/sessions");
  if (!res.ok) return;
  const sessions = await res.json() || [];
  const listEl = byId("session-list");
  listEl.replaceChildren();

  if (currentSessionId && !sessions.some(sess => sess.id === currentSessionId)) {
    listEl.appendChild(sessionItem(currentSessionId, true, "0 turns", "Session ปัจจุบัน (พร้อมรับคำสั่ง)"));
  }
  sessions.forEach(sess => {
    listEl.appendChild(sessionItem(sess.id, sess.id === currentSessionId,
      (sess.turn_count || 0) + " turns", sess.last_instruction || "Session ว่าง"));
  });
}

function renderSessionTurns(turns) {
  const box = byId("session-history");
  box.replaceChildren();
  turns.forEach(turn => {
    const card = mk("div", "squircle bg-[#161A23] border border-white/[0.08] p-4 space-y-2");
    const head = mk("div", "flex items-center justify-between gap-2");
    const when = turn.timestamp ? new Date(turn.timestamp).toLocaleString() : "";
    head.append(mk("span", "text-xs font-semibold text-sky-300", "Turn #" + turn.index + (when ? " · " + when : "")));
    const state = turn.success ? (turn.applied ? "SUCCESS · APPLIED" : "SUCCESS · NOT APPLIED") : "STOPPED";
    head.append(mk("span", "text-[10px] font-mono px-2 py-0.5 rounded-full bg-white/[0.06] text-slate-300", state));
    card.append(head, mk("div", "text-xs text-slate-200 whitespace-pre-wrap leading-relaxed", turn.instruction || ""));
    if (turn.summary) {
      const text = String(turn.summary);
      card.appendChild(mk("div", "text-[11px] text-slate-400 whitespace-pre-wrap leading-relaxed",
        text.length > 600 ? text.slice(0, 600) + "…" : text));
    }
    box.appendChild(card);
  });
}

async function selectSession(id) {
  currentSessionId = id;
  lsSet("apw_desktop_session", id);
  byId("active-session-badge").textContent = id;
  clearLiveFeed();
  await loadSessions();

  const out = await getAPI("/api/sessions/" + encodeURIComponent(id));
  if (!out.ok) { renderSessionTurns([]); return; }
  renderSessionTurns((out.data && out.data.turns) || []);
}

async function createNewSession() {
  const out = await postAPI("/api/sessions", {}, false);
  if (!out.ok || !out.data || !out.data.id) {
    appendCard("สร้าง Session ใหม่ไม่สำเร็จ", out.message || "no session id returned", "ERROR", "#f85149");
    return false;
  }
  currentSessionId = out.data.id;
  lsSet("apw_desktop_session", currentSessionId);
  byId("active-session-badge").textContent = currentSessionId;
  clearLiveFeed();
  renderSessionTurns([]);
  appendCollapsibleToolCard("SessionManager", "เริ่ม Session ใหม่: " + currentSessionId, "แยกหน่วยความจำการสนทนาใหม่เรียบร้อยแล้ว", "DONE");
  await loadSessions();
  return true;
}

/* Makes sure a valid conversation id exists (the server hands out ids; nothing is written until a task uses one). */
async function ensureSession() {
  if (currentSessionId) {
    const out = await getAPI("/api/sessions/" + encodeURIComponent(currentSessionId));
    if (out.ok) { renderSessionTurns((out.data && out.data.turns) || []); return; }
  }
  const out = await postAPI("/api/sessions", {}, false);
  if (out.ok && out.data && out.data.id) {
    currentSessionId = out.data.id;
    lsSet("apw_desktop_session", currentSessionId);
  }
  byId("active-session-badge").textContent = currentSessionId || "-";
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

/* ============================================================================
 * 8. TASK WATCHING, APPLY, EXECUTION REPORT
 * ============================================================================ */
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
      }
      return;
    }
    const done = out.data || job;
    appendCollapsibleToolCard("Apply", "[" + job.id + "] Apply ลง Workspace: " + done.status,
      done.error ? ("Error: " + done.error) : (approval === "auto" ? "Apply อัตโนมัติ (auto)" : "Apply หลังได้รับอนุญาต"),
      done.error ? "ERROR" : "DONE");
    populateArtifactFromJob(done);
    loadSessions();
  } finally {
    applyingTasks.delete(job.id);
  }
}

async function watchTask(initial) {
  if (watchedTasks.has(initial.id)) return;
  watchedTasks.add(initial.id);
  startApprovalPolling();
  let job = initial, cursor = 0;

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
        loadSessions();
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
  const instruction = input.value.trim();
  if (!instruction) return;
  input.value = "";

  const effective = kind === "submit" ? approvalMode : kind;
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
    watchTask(out.data);
  } catch (error) {
    appendCard("Task request failed", error.message, "ERROR", "#f85149");
  }
}

async function loadTasks() {
  const response = await apiFetch("/v1/tasks");
  if (response.ok) {
    const jobs = await response.json() || [];
    for (const job of jobs.slice(0, 6)) {
      if (activeStates.has(job.status)) {
        watchTask(job);
      } else if (!currentArtifactJob && job.report && (job.report.diff || (job.report.plan && job.report.plan.affected_files))) {
        populateArtifactFromJob(job);
      }
    }
  }
}

applyModeUI();
(async () => {
  await loadStatus();
  await ensureSession();
  loadSessions();
  loadTasks();
  // One look at approvals on load; polling continues only while a task or Apply is active.
  if (await loadApprovals() > 0) startApprovalPolling();
})();
</script>
</body>
</html>`
