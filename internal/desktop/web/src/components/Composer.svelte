<script lang="ts">
  import { ArrowUp, Cpu } from "lucide-svelte";
  import MentionMenu from "./MentionMenu.svelte";
  import {
    composerMode,
    submitComposerInstruction,
    runProjectVerify,
    type ComposerMode
  } from "../lib/stores/tasks";
  import {
    workspaceStatus,
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
    composerDraftText
  } from "../lib/stores/settings";
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
              aria-label={`ลบ Skill ${sk}`}
              title={`ลบ Skill ${sk}`}
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
      placeholder="สั่งงาน… ใช้ @ อ้างไฟล์/Skill (@skill:), / สำหรับคำสั่ง (Ctrl+K)"
      data-testid="composer-input"
    ></textarea>

    <div class="toolbar">
      <select
        class="chip"
        aria-label="โหมดการทำงาน"
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
        title="เลือกแนบ Skill เข้าคำสั่ง (@skill:)"
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
        aria-label="ส่งคำสั่ง"
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
    background: transparent;
    color: var(--text);
    font: inherit;
    resize: none;
    line-height: 1.55;
    overflow-y: hidden;
  }
  textarea::placeholder {
    color: var(--faint);
  }
  .toolbar {
    display: flex;
    align-items: center;
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
