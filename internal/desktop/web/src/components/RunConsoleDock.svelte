<script lang="ts">
  import { onMount } from "svelte";
  import {
    Play,
    Square,
    Copy,
    Check,
    ExternalLink,
    Terminal,
    ChevronUp,
    ChevronDown,
    Globe,
    Trash2
  } from "lucide-svelte";
  import {
    terminalSessions,
    selectedTerminalSessionId,
    terminalDockOpen,
    terminalError,
    loadTerminalState,
    runCommandInAPW,
    openCommandInExternalTerminal,
    stopTerminalSession,
    clearFinishedTerminalSessions,
    copyTextToClipboard
  } from "../lib/stores/terminal";
  import { openExternalURL } from "../lib/shell";

  let customCommand = $state("");
  let customPortInput = $state("");
  let copiedKey = $state("");
  let logEl: HTMLPreElement | undefined = $state();
  let lastOutputLen = 0;

  const runningSessions = $derived($terminalSessions.filter((s) => s.status === "running"));
  const selectedSession = $derived(
    $terminalSessions.find((s) => s.id === $selectedTerminalSessionId) ||
      runningSessions[0] ||
      $terminalSessions[$terminalSessions.length - 1] ||
      null
  );

  function parsedCustomPort(): number | undefined {
    const n = parseInt(customPortInput.trim(), 10);
    if (!Number.isNaN(n) && n >= 1024 && n <= 65535) {
      return n;
    }
    return undefined;
  }

  async function flashCopied(key: string, text: string) {
    const ok = await copyTextToClipboard(text);
    if (ok) {
      copiedKey = key;
      setTimeout(() => {
        if (copiedKey === key) copiedKey = "";
      }, 1600);
    }
  }

  async function onRunCustom() {
    const cmd = customCommand.trim();
    if (!cmd) return;
    const p = parsedCustomPort();
    customCommand = "";
    await runCommandInAPW(cmd, p ? `http://localhost:${p}` : undefined, p);
  }

  async function onOpenExternalCustom() {
    const cmd = customCommand.trim();
    if (!cmd) return;
    await openCommandInExternalTerminal(cmd, parsedCustomPort());
  }

  function shortUrlLabel(u: string): string {
    return u.replace(/^https?:\/\//i, "");
  }

  $effect(() => {
    const outLen = selectedSession?.output?.length || 0;
    if (outLen !== lastOutputLen) {
      lastOutputLen = outLen;
      queueMicrotask(() => {
        if (logEl) {
          logEl.scrollTop = logEl.scrollHeight;
        }
      });
    }
  });

  onMount(() => {
    void loadTerminalState();
    const timer = setInterval(() => {
      void loadTerminalState();
    }, 2500);
    return () => clearInterval(timer);
  });
</script>

<!-- Project commands live in each response's launch block (TaskCard). This dock only shows the
     console for commands started there, and a reopen control while something is still running. -->
{#if $terminalDockOpen || runningSessions.length > 0}
<section class="apw-run-dock" aria-label="APW Console" data-testid="apw-run-dock">
  <div class="console-head">
    <button
      type="button"
      class="dock-toggle-btn"
      class:active={$terminalDockOpen || runningSessions.length > 0}
      onclick={() => terminalDockOpen.update((v) => !v)}
      data-testid="toggle-run-dock-btn"
      aria-expanded={$terminalDockOpen}
    >
      <Terminal size={13} />
      <span class="dock-title">Console</span>
      {#if runningSessions.length > 0}
        <span class="running-pill mono">
          <span class="pulse-dot"></span>
          กำลังรัน {runningSessions.length}
        </span>
      {/if}
      {#if $terminalDockOpen}
        <ChevronDown size={13} />
      {:else}
        <ChevronUp size={13} />
      {/if}
    </button>
  </div>

  {#if $terminalDockOpen}
    <div class="console-drawer" data-testid="apw-console-drawer">
      {#if $terminalError}
        <div class="term-err-banner">{$terminalError}</div>
      {/if}

      <div class="custom-cmd-bar">
        <span class="cmd-prompt mono">PS&gt;</span>
        <input
          type="text"
          class="cmd-input mono"
          bind:value={customCommand}
          placeholder="พิมพ์คำสั่งรันเอง เช่น npm run dev"
          onkeydown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void onRunCustom();
            }
          }}
          data-testid="custom-terminal-input"
        />
        <label class="port-override-box mono" title="กำหนดพอร์ตเองเพื่อไม่ให้ชนกับระบบอื่น (เว้นว่างให้ APW หาพอร์ตว่างให้อัตโนมัติ)">
          <span>PORT:</span>
          <input
            type="text"
            class="port-override-input mono"
            bind:value={customPortInput}
            placeholder="อัตโนมัติ"
            maxlength="5"
          />
        </label>
        <button
          type="button"
          class="cmd-act-btn primary"
          disabled={!customCommand.trim()}
          onclick={onRunCustom}
          title="รันคำสั่งภายใน APW Console พร้อมกำหนดพอร์ตไม่ให้ชนกับระบบอื่น"
        >
          <Play size={11} />
          <span>รันใน APW</span>
        </button>
        <button
          type="button"
          class="cmd-act-btn"
          disabled={!customCommand.trim()}
          onclick={onOpenExternalCustom}
          title="เปิดหน้าต่าง PowerShell แยกเพื่อรันคำสั่งนี้"
        >
          <ExternalLink size={11} />
          <span>เปิด Terminal</span>
        </button>
        <button
          type="button"
          class="cmd-act-btn"
          disabled={!customCommand.trim()}
          onclick={() => flashCopied("custom-cmd", customCommand.trim())}
          title="คัดลอกคำสั่ง"
        >
          {#if copiedKey === "custom-cmd"}
            <Check size={11} />
            <span>คัดลอกแล้ว</span>
          {:else}
            <Copy size={11} />
            <span>คัดลอก</span>
          {/if}
        </button>
      </div>

      {#if $terminalSessions.length > 0}
        <div class="session-tabs-bar">
          <div class="session-tabs">
            {#each $terminalSessions as sess (sess.id)}
              <button
                type="button"
                class="sess-tab mono"
                class:active={selectedSession?.id === sess.id}
                onclick={() => selectedTerminalSessionId.set(sess.id)}
              >
                <span class="sess-dot {sess.status}"></span>
                <span class="sess-cmd">{sess.command}</span>
                {#if sess.urls?.length}
                  <span class="sess-port-tag">
                    {sess.urls[0].replace(/^https?:\/\/localhost/i, "")}
                  </span>
                {:else if sess.port}
                  <span class="sess-port-tag">:{sess.port}</span>
                {/if}
              </button>
            {/each}
          </div>

          <button
            type="button"
            class="clear-sess-btn"
            onclick={clearFinishedTerminalSessions}
            title="ล้างรายการที่รันเสร็จแล้ว"
          >
            <Trash2 size={11} />
            <span>ล้างที่จบแล้ว</span>
          </button>
        </div>

        {#if selectedSession}
          <div class="session-viewer">
            <div class="session-meta-bar">
              <div class="meta-left mono">
                <span class="status-chip {selectedSession.status}">
                  {#if selectedSession.status === "running"}
                    ● กำลังรัน {selectedSession.pid ? `(PID ${selectedSession.pid})` : ""}
                  {:else if selectedSession.status === "exited"}
                    ✓ เสร็จสิ้น (Exit 0)
                  {:else if selectedSession.status === "stopped"}
                    ■ หยุดแล้ว
                  {:else}
                    ✗ ล้มเหลว (Exit {selectedSession.exit_code})
                  {/if}
                </span>
                <code class="meta-cmd-txt">{selectedSession.command}</code>
              </div>

              <div class="meta-actions">
                {#each selectedSession.urls || [] as u (u)}
                  <button
                    type="button"
                    class="open-url-cta mono"
                    onclick={() => openExternalURL(u)}
                  >
                    <Globe size={11} />
                    <span>เปิด {shortUrlLabel(u)}</span>
                    <ExternalLink size={10} />
                  </button>
                {/each}

                <button
                  type="button"
                  class="meta-btn"
                  onclick={() => flashCopied("sess-cmd-" + selectedSession.id, selectedSession.command)}
                >
                  {copiedKey === "sess-cmd-" + selectedSession.id ? "✓ คัดลอกคำสั่งแล้ว" : "คัดลอกคำสั่ง"}
                </button>

                <button
                  type="button"
                  class="meta-btn"
                  onclick={() => flashCopied("sess-out-" + selectedSession.id, selectedSession.output || "")}
                >
                  {copiedKey === "sess-out-" + selectedSession.id ? "✓ คัดลอก Log แล้ว" : "คัดลอก Log"}
                </button>

                <button
                  type="button"
                  class="meta-btn"
                  onclick={() => openCommandInExternalTerminal(selectedSession.command, selectedSession.port)}
                >
                  ↗ เปิดใน Terminal
                </button>

                {#if selectedSession.status === "running"}
                  <button
                    type="button"
                    class="meta-btn stop"
                    onclick={() => stopTerminalSession(selectedSession.id)}
                  >
                    <Square size={10} />
                    <span>หยุด</span>
                  </button>
                {:else}
                  <button
                    type="button"
                    class="meta-btn rerun"
                    onclick={() =>
                      runCommandInAPW(
                        selectedSession.command,
                        selectedSession.port ? `http://localhost:${selectedSession.port}` : undefined,
                        selectedSession.port
                      )}
                  >
                    <Play size={10} />
                    <span>รันใหม่</span>
                  </button>
                {/if}
              </div>
            </div>

            <pre class="session-output mono selectable" bind:this={logEl}>{selectedSession.output || "กำลังรอผลลัพธ์จากโปรเซส..."}</pre>
          </div>
        {/if}
      {:else}
        <div class="empty-console-hint">
          ยังไม่มีโปรเซสของโปรเจกต์นี้ที่กำลังรัน — หากพอร์ตเดิม (เช่น :5173 หรือ :8080) มีระบบอื่นใช้อยู่ APW จะสลับหาพอร์ตว่างให้อัตโนมัติ หรือพิมพ์เลขพอร์ตที่ต้องการในช่อง <b>PORT</b> ด้านบนได้เลยครับ
        </div>
      {/if}
    </div>
  {/if}
</section>
{/if}

<style>
  .apw-run-dock {
    max-width: 760px;
    width: 100%;
    margin: 10px auto 12px;
    padding: 0 20px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .console-head {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--panel);
    box-shadow: var(--shadow-sm);
  }

  .dock-toggle-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 9px;
    border-radius: 7px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11.5px;
    font-weight: 600;
  }
  .dock-toggle-btn.active {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent);
  }
  .dock-toggle-btn:hover {
    background: var(--hover);
  }

  .running-pill {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--ok-soft);
    color: var(--ok);
    font-size: 10.5px;
    font-weight: 600;
  }
  .pulse-dot,

  .console-drawer {
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    box-shadow: var(--shadow-sm);
  }

  .term-err-banner {
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 11.5px;
  }

  .custom-cmd-bar {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    padding: 5px 8px;
    border-radius: 8px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
  }
  .cmd-prompt {
    font-size: 11.5px;
    font-weight: 700;
    color: var(--accent);
    flex: none;
  }
  .cmd-input {
    flex: 1;
    min-width: 160px;
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 12px;
    outline: none;
  }
  .port-override-box {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 6px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    font-size: 10.5px;
    color: var(--muted);
  }
  .port-override-input {
    width: 58px;
    border: none;
    background: transparent;
    color: var(--accent);
    font-size: 11px;
    font-weight: 600;
    outline: none;
  }
  .cmd-act-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 11px;
    font-weight: 500;
    flex: none;
  }
  .cmd-act-btn.primary {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-fg);
  }
  .cmd-act-btn:disabled {
    opacity: 0.5;
  }

  .session-tabs-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    border-bottom: 1px solid var(--line);
    padding-bottom: 6px;
  }
  .session-tabs {
    display: flex;
    align-items: center;
    gap: 6px;
    overflow-x: auto;
  }
  .sess-tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 9px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--muted);
    font-size: 11px;
    max-width: 240px;
  }
  .sess-tab.active {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--text);
    font-weight: 600;
  }
  .sess-cmd {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sess-port-tag {
    padding: 0 5px;
    border-radius: 4px;
    background: var(--ok-soft);
    color: var(--ok);
    font-size: 10px;
    font-weight: 600;
  }
  .sess-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: none;
    background: var(--muted);
  }
  .sess-dot.running {
    background: var(--ok);
  }
  .sess-dot.exited {
    background: var(--accent);
  }
  .sess-dot.failed {
    background: var(--err);
  }
  .sess-dot.stopped {
    background: var(--warn);
  }

  .clear-sess-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 11px;
    padding: 3px 6px;
    border-radius: 5px;
  }
  .clear-sess-btn:hover {
    background: var(--hover);
    color: var(--text);
  }

  .session-viewer {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--line);
    border-radius: 8px;
    overflow: hidden;
    background: var(--panel-subtle);
  }
  .session-meta-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
    padding: 6px 10px;
    border-bottom: 1px solid var(--line);
    background: var(--panel);
  }
  .meta-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    font-size: 11.5px;
  }
  .status-chip {
    padding: 2px 7px;
    border-radius: 999px;
    font-size: 10.5px;
    font-weight: 600;
    flex: none;
  }
  .status-chip.running {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .status-chip.exited {
    background: var(--accent-soft);
    color: var(--accent);
  }
  .status-chip.stopped {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .status-chip.failed {
    background: var(--err-soft);
    color: var(--err);
  }
  .meta-cmd-txt {
    color: var(--text);
    font-size: 11.5px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta-actions {
    display: flex;
    align-items: center;
    gap: 5px;
    flex-wrap: wrap;
  }
  .open-url-cta {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 6px;
    border: 1px solid var(--ok);
    background: var(--ok-soft);
    color: var(--ok);
    font-size: 11px;
    font-weight: 600;
  }
  .meta-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11px;
  }
  .meta-btn:hover {
    background: var(--hover);
  }
  .meta-btn.stop {
    border-color: var(--err);
    background: var(--err-soft);
    color: var(--err);
    font-weight: 600;
  }
  .meta-btn.rerun {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
  }

  .session-output {
    margin: 0;
    padding: 10px 12px;
    max-height: 210px;
    overflow-y: auto;
    font-size: 11.5px;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--text);
  }

  .empty-console-hint {
    padding: 10px;
    text-align: center;
    font-size: 12px;
    color: var(--muted);
  }
</style>
