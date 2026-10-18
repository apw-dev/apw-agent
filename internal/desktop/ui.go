package desktop

const desktopHTML = `<!DOCTYPE html>
<html lang="th" class="dark">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>APW Desktop — Apiwat Agent (3-Pane AI Developer OS)</title>
<script src="https://cdn.tailwindcss.com"></script>
<script>
tailwind.config = {
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        obsidian: '#0B0D11',
        slatepanel: '#11141B',
        cardbg: '#161A23',
        cardhover: '#1C212D',
        borderdim: 'rgba(255, 255, 255, 0.08)',
        accent: '#E07A5F',
        accenthover: '#EB8C73',
      },
      fontFamily: {
        sans: ['Inter', 'Noto Sans Thai', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'sans-serif'],
        mono: ['JetBrains Mono', 'Cascadia Code', 'Consolas', 'monospace'],
      }
    }
  }
}
</script>
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
    <!-- 1. LEFT SIDEBAR (Collapsible: Sessions, Workspace, AGENT.md Status) -->
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

      <!-- Workspace & AGENT.md Status Section -->
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
            <button onclick="toggleTrustWorkspace()" id="btn-trust-toggle" class="text-[11px] text-slate-400 hover:text-white underline decoration-dotted">ตั้งค่าสิทธิ์</button>
          </div>
        </div>

        <!-- AGENT.md Status Card -->
        <div id="agent-md-card" class="squircle-sm bg-[#161A23] border border-white/[0.07] p-3 transition hover:border-white/[0.15]">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span id="agent-md-dot" class="w-2 h-2 rounded-full bg-slate-500"></span>
              <span class="text-xs font-mono font-semibold text-slate-200">AGENT.md</span>
            </div>
            <span id="agent-md-badge" class="text-[10px] font-mono px-2 py-0.5 rounded-md bg-slate-700/50 text-slate-300">Checking...</span>
          </div>
          <p id="agent-md-desc" class="text-[11px] text-slate-400 mt-1.5 leading-relaxed">ตรวจสอบกฎและบริบทของโปรเจกต์</p>
          <div class="mt-2 flex items-center gap-2">
            <button onclick="openAgentMDInArtifactPane()" class="flex-1 text-[11px] font-medium py-1.5 px-2.5 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] text-slate-200 transition text-center">
              เปิดดู / ตรวจสอบ
            </button>
            <button id="btn-create-agent-md" onclick="initAgentMD()" class="hidden text-[11px] font-medium py-1.5 px-2.5 rounded-lg bg-[#E07A5F]/20 hover:bg-[#E07A5F]/30 text-[#E07A5F] border border-[#E07A5F]/30 transition">
              + สร้างไฟล์
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
          <span>HITL Always-Allow:</span>
          <span id="hitl-allow-count" class="text-amber-300">0 คำสั่ง</span>
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
          <!-- Human-in-the-loop Gate Toggle -->
          <label class="flex items-center gap-2 px-3 py-1.5 rounded-xl bg-[#161A23] border border-white/[0.08] cursor-pointer text-xs" title="ถามคำยืนยันผ่าน chan bool ก่อนรันคำสั่งระบบหรือแก้ไขโค้ด">
            <input type="checkbox" id="hitl-toggle" checked class="accent-[#E07A5F] rounded" />
            <span class="text-slate-300 font-medium">Tool Approval (HITL)</span>
          </label>

          <!-- Test Approval Button -->
          <button onclick="triggerDemoApproval()" class="px-3 py-1.5 rounded-xl bg-amber-500/15 hover:bg-amber-500/25 text-amber-300 border border-amber-500/30 text-xs font-medium transition">
            ทดสอบขออนุมัติ Tool
          </button>

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

        <!-- Pending Approvals Container (Human-in-the-loop Cards) -->
        <div id="approval-feed" class="space-y-3"></div>
      </div>

      <!-- Bottom Composer (Superellipse / Squircle) -->
      <div class="p-4 pt-2 bg-gradient-to-t from-[#0B0D11] via-[#0B0D11] to-transparent shrink-0">
        <div class="max-w-4xl mx-auto squircle bg-[#161A23] border border-white/[0.1] p-3 shadow-2xl focus-within:border-[#E07A5F]/60 transition">
          <textarea
            id="task-input"
            rows="2"
            placeholder="พิมพ์คำสั่งให้ Agent ทำงาน เช่น ตรวจสอบโค้ดในโปรเจกต์, เพิ่มฟังก์ชันใหม่, หรือวางแผนสถาปัตยกรรม... (กด Enter เพื่อส่ง, Shift+Enter ขึ้นบรรทัดใหม่)"
            class="w-full bg-transparent text-sm text-white placeholder-slate-500 focus:outline-none resize-none font-sans leading-relaxed px-1"
            onkeydown="if(event.key==='Enter' && !event.shiftKey){ event.preventDefault(); runTask(document.getElementById('exec-mode').value); }"
          ></textarea>

          <div class="flex flex-wrap items-center justify-between gap-2 pt-2 mt-1 border-t border-white/[0.06]">
            <!-- Left Mode Selector & Quick Buttons -->
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="text-[11px] font-mono text-slate-400 mr-1">โหมด:</span>
              <select id="exec-mode" class="text-xs font-mono bg-[#11141B] text-slate-200 border border-white/[0.1] rounded-xl px-2.5 py-1.5 focus:outline-none focus:border-[#E07A5F]">
                <option value="execute">Execute (Architect → Surgeon → Reflex)</option>
                <option value="preview">Preview Diff (Isolated Worktree)</option>
                <option value="review">Review (Read-only Audit)</option>
                <option value="plan">Plan Only (ExecutionPlan JSON)</option>
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
            <button onclick="runTask(document.getElementById('exec-mode').value)" class="px-4 py-1.5 rounded-xl bg-[#E07A5F] hover:bg-[#EB8C73] text-white text-xs font-semibold shadow-lg shadow-[#E07A5F]/25 transition flex items-center gap-1.5">
              <span>ส่งคำสั่ง</span>
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M5 12h14M12 5l7 7-7 7"/></svg>
            </button>
          </div>
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
            <span id="code-viewer-filename" class="text-xs font-mono text-slate-300">code.go</span>
            <button onclick="renderCodeAsHTMLPreview()" class="text-[11px] text-[#E07A5F] hover:underline">ส่งไปเรนเดอร์ใน HTML Preview →</button>
          </div>
          <pre id="code-viewer-pre" class="p-4 text-xs font-mono text-slate-200 overflow-x-auto leading-relaxed whitespace-pre">เลือกไฟล์จากด้านบนหรือคลิกการ์ด AGENT.md เพื่อดูเนื้อหาโค้ด</pre>
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

<script>
const apiToken = "__APW_TOKEN__";
let currentSessionId = localStorage.getItem("apw_desktop_session") || "local-default";
let leftSidebarOpen = true;
let rightPaneOpen = false;
let currentArtifactJob = null;
const watchedTasks = new Set();
const renderedApprovalIds = new Set();
const activeStates = new Set(["queued", "planning", "analyzing", "patching", "verifying", "applying"]);

document.getElementById("active-session-badge").textContent = currentSessionId;

async function apiFetch(path, options = {}) {
  if (!path.startsWith("/")) throw new Error("Local API path required");
  const headers = { ...(options.headers || {}), Authorization: "Bearer " + apiToken };
  if (options.method === "POST") headers["Content-Type"] = "application/json";
  const request = { ...options, headers };
  try { return await window.fetch(path, request); }
  catch (error) { return window.fetch(path, request); }
}

/* ============================================================================
 * 1. 3-PANE SPLIT VIEW CONTROLS (Left Sidebar & Right Auto-Slide Artifact Pane)
 * ============================================================================ */
function toggleLeftSidebar() {
  const sb = document.getElementById("left-sidebar");
  const showBtn = document.getElementById("btn-show-left");
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
  const rp = document.getElementById("right-pane");
  rightPaneOpen = true;
  rp.classList.remove("w-0");
  rp.classList.add("w-[480px]", "xl:w-[560px]");
  if (defaultTab) switchArtifactTab(defaultTab);
}

function closeRightPane() {
  const rp = document.getElementById("right-pane");
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
    const panel = document.getElementById("artifact-tab-" + t);
    const btn = document.getElementById("tab-btn-" + t);
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
});

/* ============================================================================
 * 2. HUMAN-IN-THE-LOOP TOOL APPROVAL (Go chan bool <-> 3-Button Approval Card)
 * ============================================================================ */
function renderApprovalCard(req) {
  const existing = document.getElementById("appr-card-" + req.id);
  if (req.status !== "pending") {
    if (existing) {
      const statusBadge = existing.querySelector(".appr-status-badge");
      const btnRow = existing.querySelector(".appr-btn-row");
      if (statusBadge) {
        const label = req.status === "approved_always" ? "✓ อนุญาตเสมอสำหรับคำสั่งนี้" :
                      req.status === "approved_once" ? "✓ อนุญาตครั้งเดียวแล้ว" : "✗ ปฏิเสธแล้ว";
        statusBadge.textContent = label;
        statusBadge.className = "appr-status-badge text-[11px] font-mono px-2.5 py-0.5 rounded-full " +
          (req.status.startsWith("approved") ? "bg-emerald-500/20 text-emerald-300 border border-emerald-500/30" : "bg-rose-500/20 text-rose-300 border border-rose-500/30");
      }
      if (btnRow) btnRow.remove();
    }
    return;
  }
  if (renderedApprovalIds.has(req.id)) return;
  renderedApprovalIds.add(req.id);

  const container = document.getElementById("approval-feed");
  const card = document.createElement("div");
  card.id = "appr-card-" + req.id;
  card.className = "squircle bg-[#1A1714] border-2 border-amber-500/40 p-4 shadow-2xl space-y-3";

  const header = document.createElement("div");
  header.className = "flex items-center justify-between gap-2";
  header.innerHTML =
    '<div class="flex items-center gap-2">' +
      '<span class="w-2.5 h-2.5 rounded-full bg-amber-400 pulse-dot"></span>' +
      '<span class="text-xs font-semibold text-amber-300 uppercase tracking-wider">ขออนุมัติรันคำสั่ง / Tool Approval (Human-in-the-Loop)</span>' +
    '</div>' +
    '<span class="appr-status-badge text-[11px] font-mono px-2.5 py-0.5 rounded-full bg-amber-500/20 text-amber-300 border border-amber-500/30">รอการยืนยัน (chan bool)</span>';

  const details = document.createElement("div");
  details.className = "squircle-sm bg-[#0E1015] border border-white/[0.07] p-3 font-mono text-xs space-y-1.5";
  const toolLine = document.createElement("div");
  toolLine.innerHTML = '<span class="text-slate-400">Tool:</span> <span class="text-sky-300 font-semibold"></span>';
  toolLine.querySelector("span:last-child").textContent = req.tool_name || "SystemCommand";

  const cmdLine = document.createElement("div");
  cmdLine.innerHTML = '<span class="text-slate-400">Command:</span> <code class="text-amber-200 bg-white/[0.04] px-1.5 py-0.5 rounded"></code>';
  cmdLine.querySelector("code").textContent = req.command || "-";

  details.appendChild(toolLine);
  details.appendChild(cmdLine);

  if (req.target_path) {
    const pathLine = document.createElement("div");
    pathLine.innerHTML = '<span class="text-slate-400">Target:</span> <span class="text-slate-200"></span>';
    pathLine.querySelector("span:last-child").textContent = req.target_path;
    details.appendChild(pathLine);
  }
  if (req.description) {
    const descLine = document.createElement("div");
    descLine.className = "text-slate-400 font-sans text-[11px] pt-1";
    descLine.textContent = req.description;
    details.appendChild(descLine);
  }

  const btnRow = document.createElement("div");
  btnRow.className = "appr-btn-row flex flex-wrap items-center gap-2 pt-1";

  const choices = [
    { key: "once", label: "อนุญาตครั้งเดียว", cls: "bg-emerald-600 hover:bg-emerald-500 text-white" },
    { key: "always", label: "อนุญาตเสมอสำหรับคำสั่งนี้", cls: "bg-sky-600 hover:bg-sky-500 text-white" },
    { key: "deny", label: "ปฏิเสธ", cls: "bg-rose-600/25 hover:bg-rose-600/40 text-rose-200 border border-rose-500/40" }
  ];

  choices.forEach(c => {
    const btn = document.createElement("button");
    btn.className = "px-3.5 py-1.5 rounded-xl text-xs font-semibold transition " + c.cls;
    btn.textContent = c.label;
    btn.onclick = () => submitApprovalDecision(req.id, c.key);
    btnRow.appendChild(btn);
  });

  card.appendChild(header);
  card.appendChild(details);
  card.appendChild(btnRow);
  container.appendChild(card);

  const feed = document.getElementById("feed");
  feed.scrollTop = feed.scrollHeight;
}

async function submitApprovalDecision(id, decision) {
  const res = await apiFetch("/api/approvals/" + encodeURIComponent(id) + "/decide", {
    method: "POST",
    body: JSON.stringify({ decision })
  });
  if (!res.ok) {
    appendCard("การส่งคำยืนยันล้มเหลว", await res.text(), "ERROR", "#f85149");
    return;
  }
  const updated = await res.json();
  renderApprovalCard(updated);
  loadApprovals();
}

async function loadApprovals() {
  const res = await apiFetch("/api/approvals");
  if (!res.ok) return;
  const data = await res.json();
  (data.pending || []).forEach(renderApprovalCard);
  const countEl = document.getElementById("hitl-allow-count");
  if (countEl && data.always_allowed) {
    countEl.textContent = data.always_allowed.length + " คำสั่ง";
  }
}

async function triggerDemoApproval() {
  const cmd = "go test -mod=readonly -race ./...";
  appendCollapsibleToolCard("SystemCommand", "กำลังขออนุมัติรันคำสั่ง: " + cmd, "รอคำยืนยันจากผู้ใช้ผ่าน chan bool ในฝั่ง Go...", "RUNNING");
  const res = await apiFetch("/api/approvals/request", {
    method: "POST",
    body: JSON.stringify({
      tool_name: "SystemCommand",
      command: cmd,
      target_path: ".",
      description: "รันชุดทดสอบ Go Race Detector ใน Isolated Worktree ก่อนอนุญาตให้แก้ไขโค้ด"
    })
  });
  if (!res.ok) {
    appendCard("คำสั่งถูกปฏิเสธหรือยกเลิก", await res.text(), "DENIED", "#f85149");
    return;
  }
  const result = await res.json();
  if (result.approved) {
    appendCollapsibleToolCard("SystemCommand", "อนุญาตแล้ว: " + cmd, "ผลลัพธ์จาก chan bool = true (อนุมัติให้รันคำสั่งระบบได้)", "DONE");
  } else {
    appendCollapsibleToolCard("SystemCommand", "ปฏิเสธคำสั่ง: " + cmd, "ผลลัพธ์จาก chan bool = false (ยกเลิกการรันคำสั่ง)", "DENIED");
  }
}

/* ============================================================================
 * 3. COLLAPSIBLE TOOL CARDS & STREAMING MESSAGE RENDERER
 * ============================================================================ */
function appendCollapsibleToolCard(toolName, summaryText, detailBody, status, CardID) {
  const feed = document.getElementById("feed");
  let detailsEl = CardID ? document.getElementById(CardID) : null;
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

  detailsEl.innerHTML =
    '<summary class="px-4 py-2.5 cursor-pointer flex items-center justify-between gap-3 hover:bg-white/[0.03] select-none list-none">' +
      '<div class="flex items-center gap-2.5 min-w-0">' +
        '<span class="w-2 h-2 rounded-full shrink-0 ' + dotCls + '"></span>' +
        '<span class="text-[11px] font-mono font-semibold px-2 py-0.5 rounded-md bg-white/[0.06] text-sky-300 shrink-0"></span>' +
        '<span class="text-xs text-slate-200 truncate tool-summary"></span>' +
      '</div>' +
      '<div class="flex items-center gap-2 shrink-0">' +
        '<span class="text-[10px] font-mono px-2 py-0.5 rounded-full border ' + badgeCls + '"></span>' +
        '<svg class="w-3.5 h-3.5 text-slate-400 group-open:rotate-180 transition-transform" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7"/></svg>' +
      '</div>' +
    '</summary>' +
    '<div class="px-4 py-3 border-t border-white/[0.06] bg-[#0D1017] font-mono text-xs text-slate-300 whitespace-pre-wrap leading-relaxed tool-detail"></div>';

  detailsEl.querySelector("summary span:nth-child(2)").textContent = toolName;
  detailsEl.querySelector(".tool-summary").textContent = summaryText;
  detailsEl.querySelector("summary div:last-child span").textContent = status;
  detailsEl.querySelector(".tool-detail").textContent = typeof detailBody === "string" ? detailBody : JSON.stringify(detailBody, null, 2);

  if (isNew) {
    feed.appendChild(detailsEl);
    feed.scrollTop = feed.scrollHeight;
  }
  return detailsEl;
}

function appendStreamingBubble(role, title, text, badgeText) {
  const feed = document.getElementById("feed");
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
  feed.appendChild(card);

  const full = typeof text === "string" ? text : JSON.stringify(text, null, 2);
  if (isUser || full.length > 900) {
    bodyEl.textContent = full;
    feed.scrollTop = feed.scrollHeight;
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
    feed.scrollTop = feed.scrollHeight;
  }, 16);
}

function appendCard(title, body, badgeText, badgeColor) {
  const feed = document.getElementById("feed");
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
  feed.appendChild(card);
  feed.scrollTop = feed.scrollHeight;
}

/* ============================================================================
 * 4. RIGHT ARTIFACT / DIFF RENDERER (Diff Table, Code Viewer, HTML Preview)
 * ============================================================================ */
function renderDiffTable(diffText) {
  const tbody = document.getElementById("diff-tbody");
  const emptyState = document.getElementById("diff-empty-state");
  const tableContainer = document.getElementById("diff-table-container");
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

  document.getElementById("artifact-title").textContent = "Task " + job.id + " (" + job.status + ")";
  document.getElementById("artifact-status-pill").textContent = job.status.toUpperCase();
  document.getElementById("artifact-task-id").textContent = job.id + " · " + job.status;

  const select = document.getElementById("artifact-file-select");
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

  document.getElementById("artifact-count-badge").textContent = String(files.length || (report.diff ? 1 : 0));

  if (report.diff) {
    renderDiffTable(report.diff);
  }

  const accBox = document.getElementById("acceptance-box");
  const accPre = document.getElementById("acceptance-pre");
  if (report.acceptance) {
    accBox.classList.remove("hidden");
    accPre.textContent = typeof report.acceptance === "string" ? report.acceptance : JSON.stringify(report.acceptance, null, 2);
  } else {
    accBox.classList.add("hidden");
  }

  // Populate action buttons in Right Pane footer
  const btnWrap = document.getElementById("artifact-buttons");
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
    b.textContent = action === "apply" ? "✓ Apply ลง Workspace" : action === "resume" ? "↻ Resume & Verify" : "✕ Cancel";
    b.onclick = async () => {
      const res = await apiFetch("/v1/tasks/" + encodeURIComponent(job.id) + "/" + action, { method: "POST", body: "{}" });
      if (!res.ok) { appendCard("Task action failed", await res.text(), "ERROR", "#f85149"); return; }
      watchTask(await res.json());
    };
    btnWrap.appendChild(b);
  });

  // Auto-slide-out Right Pane when Diff or affected files exist
  if (report.diff || files.length > 0) {
    openRightPane("diff");
  }
}

async function onSelectArtifactFile(relPath) {
  if (!relPath) return;
  const res = await apiFetch("/api/file?path=" + encodeURIComponent(relPath));
  if (!res.ok) return;
  const data = await res.json();
  document.getElementById("code-viewer-filename").textContent = data.path;
  document.getElementById("code-viewer-pre").textContent = data.content || "";
  if (relPath.endsWith(".html") || relPath.endsWith(".htm")) {
    document.getElementById("html-preview-frame").srcdoc = data.content || "";
    openRightPane("preview");
  } else {
    openRightPane("code");
  }
}

function renderCodeAsHTMLPreview() {
  const content = document.getElementById("code-viewer-pre").textContent;
  document.getElementById("html-preview-frame").srcdoc = content;
  switchArtifactTab("preview");
}

function loadSampleHTMLPreview() {
  const sample = '<!DOCTYPE html><html><body style="background:#0f172a;color:#f8fafc;font-family:sans-serif;padding:24px;">' +
    '<div style="background:#1e293b;border:1px solid #334155;border-radius:16px;padding:20px;">' +
    '<h2 style="margin:0 0 8px;color:#38bdf8;">APW Live Artifact Preview</h2>' +
    '<p style="color:#94a3b8;font-size:14px;">เรนเดอร์ HTML/Tailwind Preview โดยตรงในแผงด้านขวา</p>' +
    '</div></body></html>';
  document.getElementById("html-preview-frame").srcdoc = sample;
  openRightPane("preview");
}

/* ============================================================================
 * 5. WORKSPACE, AGENT.MD & SESSION SIDEBAR MANAGEMENT
 * ============================================================================ */
let cachedAgentMDContent = "";
let cachedAgentMDPath = "AGENT.md";

async function loadStatus() {
  const r = await apiFetch("/api/status");
  if (!r.ok) return;
  const s = await r.json();

  document.getElementById("hdr-version").textContent = "v" + s.version;
  document.getElementById("hdr-workspace").textContent = s.workspace;
  document.getElementById("hdr-workspace").title = s.workspace;
  document.getElementById("ws-stack-badge").textContent = s.stack || "Project";

  const trustEl = document.getElementById("ws-trust-badge");
  if (s.trusted) {
    trustEl.className = "flex items-center gap-1.5 text-emerald-400 font-mono";
    trustEl.innerHTML = '<span class="w-2 h-2 rounded-full bg-emerald-400"></span> Trusted Workspace';
  } else {
    trustEl.className = "flex items-center gap-1.5 text-amber-400 font-mono";
    trustEl.innerHTML = '<span class="w-2 h-2 rounded-full bg-amber-400"></span> Untrusted (Safe Mode)';
  }

  document.getElementById("pill-brain1").textContent = "⚡ Brain 1: " + (s.brain1_model || s.brain1_name);
  document.getElementById("pill-brain2").textContent = "🧠 Brain 2: " + (s.brain2_model || s.brain2_name);
  document.getElementById("pill-daemon").textContent = "apwd: " + (s.daemon_running ? "PID " + s.daemon_pid + " (" + s.brain1_latency_ms + "ms)" : "Embedded");

  // AGENT.md status in Left Sidebar
  const dot = document.getElementById("agent-md-dot");
  const badge = document.getElementById("agent-md-badge");
  const desc = document.getElementById("agent-md-desc");
  const createBtn = document.getElementById("btn-create-agent-md");

  cachedAgentMDContent = s.agent_md_content || "";
  cachedAgentMDPath = s.agent_md_path || "AGENT.md";

  if (s.agent_md_loaded) {
    dot.className = "w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_#34d399]";
    badge.className = "text-[10px] font-mono px-2 py-0.5 rounded-md bg-emerald-500/15 text-emerald-300 border border-emerald-500/30";
    badge.textContent = "Loaded (" + s.agent_md_lines + " lines)";
    desc.textContent = "โหลด " + s.agent_md_path + " (" + s.agent_md_size + " bytes) เข้าสู่บริบทของ Agent แล้ว";
    createBtn.classList.add("hidden");
  } else {
    dot.className = "w-2 h-2 rounded-full bg-amber-400";
    badge.className = "text-[10px] font-mono px-2 py-0.5 rounded-md bg-amber-500/15 text-amber-300 border border-amber-500/30";
    badge.textContent = "Not Found";
    desc.textContent = "ยังไม่พบไฟล์ AGENT.md ในโปรเจกต์ สามารถกดสร้างไฟล์เริ่มต้นได้";
    createBtn.classList.remove("hidden");
  }
}

function openAgentMDInArtifactPane() {
  document.getElementById("artifact-title").textContent = cachedAgentMDPath;
  document.getElementById("code-viewer-filename").textContent = cachedAgentMDPath;
  document.getElementById("code-viewer-pre").textContent = cachedAgentMDContent || "# AGENT.md (ยังไม่มีไฟล์ใน Workspace — กด '+ สร้างไฟล์' ที่แถบซ้ายเพื่อสร้าง)";
  openRightPane("code");
}

async function initAgentMD() {
  const res = await apiFetch("/api/agent-md/init", { method: "POST", body: "{}" });
  if (!res.ok) {
    appendCard("ไม่สามารถสร้าง AGENT.md ได้", await res.text(), "ERROR", "#f85149");
    return;
  }
  await loadStatus();
  openAgentMDInArtifactPane();
}

async function toggleTrustWorkspace() {
  const res = await apiFetch("/api/trust", { method: "POST", body: "{}" });
  if (res.ok) {
    await loadStatus();
  }
}

async function loadSessions() {
  const res = await apiFetch("/api/sessions");
  if (!res.ok) return;
  const sessions = await res.json() || [];
  const listEl = document.getElementById("session-list");
  listEl.replaceChildren();

  if (sessions.length === 0) {
    const item = document.createElement("div");
    item.className = "squircle-sm p-3 bg-[#161A23] border border-[#E07A5F]/40 text-xs cursor-pointer";
    item.innerHTML = '<div class="font-mono font-semibold text-white">' + currentSessionId + '</div>' +
                     '<div class="text-[11px] text-slate-400 mt-0.5">Session ปัจจุบัน (พร้อมรับคำสั่ง)</div>';
    listEl.appendChild(item);
    return;
  }

  sessions.forEach(sess => {
    const active = sess.id === currentSessionId;
    const item = document.createElement("div");
    item.className = "squircle-sm p-3 border transition cursor-pointer " +
      (active ? "bg-[#1C212D] border-[#E07A5F]/50" : "bg-[#161A23] border-white/[0.06] hover:border-white/[0.15]");
    const lastPrompt = sess.last_instruction || "Session ว่าง";
    item.innerHTML =
      '<div class="flex items-center justify-between gap-2">' +
        '<span class="text-xs font-mono font-semibold text-white truncate"></span>' +
        '<span class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-white/[0.06] text-slate-300 shrink-0">' + (sess.turn_count || 0) + ' turns</span>' +
      '</div>' +
      '<div class="text-[11px] text-slate-400 truncate mt-1"></div>';
    item.querySelector("span:first-child").textContent = sess.id;
    item.querySelector("div:last-child").textContent = lastPrompt;
    item.onclick = () => selectSession(sess.id);
    listEl.appendChild(item);
  });
}

async function selectSession(id) {
  currentSessionId = id;
  localStorage.setItem("apw_desktop_session", id);
  document.getElementById("active-session-badge").textContent = id;
  await loadSessions();

  const res = await apiFetch("/api/sessions/" + encodeURIComponent(id));
  if (!res.ok) return;
  const sess = await res.json();
  (sess.history || []).forEach(turn => {
    appendStreamingBubble("user", "คำสั่ง (Turn #" + turn.index + ")", turn.instruction, turn.plan_id || "");
    if (turn.summary || turn.error) {
      appendStreamingBubble("agent", "สรุปผลการทำงาน", turn.summary || turn.error, turn.success ? "SUCCESS" : "STOPPED");
    }
  });
}

async function createNewSession() {
  const res = await apiFetch("/api/sessions", { method: "POST", body: "{}" });
  if (!res.ok) return;
  const sess = await res.json();
  currentSessionId = sess.id;
  localStorage.setItem("apw_desktop_session", sess.id);
  document.getElementById("active-session-badge").textContent = sess.id;
  appendCollapsibleToolCard("SessionManager", "เริ่ม Session ใหม่: " + sess.id, "แยกหน่วยความจำการสนทนาใหม่เรียบร้อยแล้ว", "DONE");
  await loadSessions();
}

async function openSymbolsArtifact() {
  const r = await apiFetch("/api/symbols");
  if (!r.ok) return;
  const files = await r.json() || [];
  const lines = files.map(f => "=== " + f.file + " ===\n" + (f.symbols || []).map(s => "  [" + s.Kind + "] " + s.Name + " (L" + s.StartLine + "-L" + s.EndLine + ")").join("\n")).join("\n\n");
  document.getElementById("artifact-title").textContent = "Go AST Symbols (" + files.length + " files)";
  document.getElementById("code-viewer-filename").textContent = "workspace.symbols";
  document.getElementById("code-viewer-pre").textContent = lines || "ไม่พบ Go AST Symbols";
  openRightPane("code");
}

/* ============================================================================
 * 6. TASK WATCHING, EXECUTION REPORT & SSE STREAMING
 * ============================================================================ */
function jobActions(job) {
  populateArtifactFromJob(job);
}

async function watchTask(initial) {
  if (watchedTasks.has(initial.id)) return;
  watchedTasks.add(initial.id);
  let job = initial, cursor = 0;

  const toolCardId = "task-tool-" + job.id;
  appendCollapsibleToolCard("TriadExecutor", "[" + job.id + "] " + job.instruction, "กำลังเริ่มต้นงานในโหมด " + (job.mode || "execute") + "...", "RUNNING", toolCardId);

  try {
    while (true) {
      await loadApprovals();
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
  document.getElementById("artifact-title").textContent = "ExecutionPlan: " + (plan.plan_id || "");
  document.getElementById("code-viewer-filename").textContent = (plan.plan_id || "plan") + ".json";
  document.getElementById("code-viewer-pre").textContent = JSON.stringify(plan, null, 2);
  openRightPane("code");
}

async function runVerify() {
  const requireApproval = document.getElementById("hitl-toggle").checked;
  const response = await apiFetch("/api/verify", {
    method: "POST",
    body: JSON.stringify({ require_approval: requireApproval })
  });
  if (!response.ok) {
    appendCard("Verification request failed", await response.text(), "ERROR", "#f85149");
    return;
  }
  watchTask(await response.json());
}

async function runTask(mode) {
  const input = document.getElementById("task-input");
  const instruction = input.value.trim();
  if (!instruction) return;
  input.value = "";

  appendStreamingBubble("user", "คุณ (User)", instruction, mode.toUpperCase());
  const requireApproval = document.getElementById("hitl-toggle").checked && (mode === "execute" || mode === "run");

  try {
    if (mode === "plan") {
      const cardId = "plan-" + Date.now();
      appendCollapsibleToolCard("ArchitectPlanner", "กำลังวิเคราะห์โครงสร้างโปรเจกต์และสร้าง ExecutionPlan...", instruction, "RUNNING", cardId);
      const response = await apiFetch("/api/plan", { method: "POST", body: JSON.stringify({ instruction }) });
      if (!response.ok) throw new Error(await response.text());
      const plan = await response.json();
      appendPlanCard(plan);
    } else {
      const normalizedMode = mode === "run" ? "execute" : mode;
      const response = await apiFetch("/api/run", {
        method: "POST",
        body: JSON.stringify({
          request_id: crypto.randomUUID(),
          session_id: currentSessionId,
          instruction,
          mode: normalizedMode,
          require_approval: requireApproval
        })
      });
      if (!response.ok) throw new Error(await response.text());
      watchTask(await response.json());
    }
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

// Poll for pending Human-in-the-loop approvals so approval cards appear immediately when Go blocks on chan bool
setInterval(loadApprovals, 600);

loadStatus();
loadSessions();
loadApprovals();
loadTasks();
</script>
</body>
</html>`
