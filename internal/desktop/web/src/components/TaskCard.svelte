<script lang="ts">
  import {
    ChevronDown,
    ChevronRight,
    BookmarkPlus,
    RefreshCw,
    Check,
    AlertCircle,
    Play,
    Copy,
    ExternalLink,
    Globe
  } from "lucide-svelte";
  import MarkdownView from "./MarkdownView.svelte";
  import ModelFlowRunner from "./ModelFlowRunner.svelte";
  import PromptInspectorModal from "./PromptInspectorModal.svelte";
  import type { TaskJob, TaskEvent, PromptTrace } from "../lib/api";
  import { parseUnifiedDiff } from "../lib/diff";
  import {
    ACTIVE_TASK_STATUSES,
    RESUMABLE_TASK_STATUSES,
    selectedTaskId,
    mapTaskSteps,
    extractTaskSubagents,
    extractTaskPromptTraces,
    selectTask,
    applyTaskChanges,
    cancelTaskById,
    resumeTaskById,
    taskActionError
  } from "../lib/stores/tasks";
  import { createProceduralSkill, fetchWorkspaceFile } from "../lib/stores/workspace";
  import { rightCollapsed, rightPanelView, selectedDiffFile } from "../lib/stores/settings";
  import { parseInstructionSkillPills } from "../lib/stores/search";
  import { openExternalURL } from "../lib/shell";
  import { t, tNow, locale } from "../lib/i18n";
  import {
    terminalScripts,
    activeWorkspacePorts,
    runCommandInAPW,
    openCommandInExternalTerminal,
    copyTextToClipboard
  } from "../lib/stores/terminal";

  let { job, events = [] }: { job: TaskJob; events?: TaskEvent[] } = $props();

  let logExpanded = $state(false);
  let logFilter = $state<"all" | "steps" | "subagents" | "alerts">("all");
  let logSearch = $state("");
  let expandedEventSeqs = $state<Record<number, boolean>>({});
  let subagentsExpanded = $state(false);
  let thinkingExpanded = $state(false);
  let commandsExpanded = $state(false);
  let allAcceptanceExpanded = $state(false);
  let busy = $state(false);
  let skillNotice = $state("");
  let copiedCmdKey = $state("");
  let nowMs = $state(Date.now());
  let inspectedTrace = $state<PromptTrace | null>(null);
  const promptTraces = $derived(extractTaskPromptTraces(events));

  async function flashTaskCopy(key: string, text: string) {
    const ok = await copyTextToClipboard(text);
    if (ok) {
      copiedCmdKey = key;
      setTimeout(() => {
        if (copiedCmdKey === key) copiedCmdKey = "";
      }, 1500);
    }
  }

  const parsedInstruction = $derived(parseInstructionSkillPills(job.instruction || ""));
  const subagents = $derived(extractTaskSubagents(job, events));
  const runningSubagentsCount = $derived(subagents.filter((s) => s.status === "running").length);
  const stepBar = $derived(mapTaskSteps(job, events));
  const isRunning = $derived(ACTIVE_TASK_STATUSES.has(job.status));

  $effect(() => {
    if (!isRunning) return;
    nowMs = Date.now();
    const timer = setInterval(() => {
      nowMs = Date.now();
    }, 1000);
    return () => clearInterval(timer);
  });

  function formatLiveClock(sec: number): string {
    const s = Math.max(0, Math.floor(sec));
    const hrs = Math.floor(s / 3600);
    const mins = Math.floor((s % 3600) / 60);
    const rem = s % 60;
    const mm = String(mins).padStart(2, "0");
    const ss = String(rem).padStart(2, "0");
    if (hrs > 0) {
      return `${hrs}:${mm}:${ss}`;
    }
    return `${mm}:${ss}`;
  }

  const liveElapsedSec = $derived.by((): number => {
    if (!isRunning) return 0;
    const startStr = job.created_at || job.updated_at;
    if (!startStr) return 0;
    const startMs = new Date(startStr).getTime();
    if (Number.isNaN(startMs)) return 0;
    return Math.max(0, Math.floor((nowMs - startMs) / 1000));
  });

  const liveStageSec = $derived.by((): number => {
    if (!isRunning) return 0;
    const lastEvTime = events.length > 0 ? events[events.length - 1]?.timestamp : undefined;
    const refStr = lastEvTime || job.updated_at || job.created_at;
    if (!refStr) return liveElapsedSec;
    const refMs = new Date(refStr).getTime();
    if (Number.isNaN(refMs)) return liveElapsedSec;
    return Math.max(0, Math.floor((nowMs - refMs) / 1000));
  });
  const canApply = $derived(job.status === "ready_to_apply");
  const canResume = $derived(RESUMABLE_TASK_STATUSES.has(job.status));
  const changedFiles = $derived.by(() => {
    const mod = job.report?.modified_files || [];
    if (mod.length > 0) return mod;
    return job.report?.plan?.affected_files || [];
  });
  const fileDiffs = $derived(parseUnifiedDiff(job.report?.diff || "", changedFiles));
  const diffStatsByFile = $derived.by(() => {
    const m = new Map<string, { added: number; deleted: number }>();
    for (const fd of fileDiffs) {
      m.set(fd.file, { added: fd.added, deleted: fd.deleted });
    }
    return m;
  });
  const totalAdded = $derived(fileDiffs.reduce((acc, f) => acc + f.added, 0));
  const totalDeleted = $derived(fileDiffs.reduce((acc, f) => acc + f.deleted, 0));
  const actionErr = $derived($taskActionError[job.id] || "");
  const rawError = $derived((actionErr || job.error || "").trim());
  const isInterruptedRestart = $derived(
    job.status === "interrupted" || rawError === "recovered after service restart"
  );
  const unmetAcceptanceItems = $derived.by((): string[] => {
    const prefix = "acceptance not satisfied:";
    if (rawError.toLowerCase().startsWith(prefix)) {
      return rawError
        .slice(prefix.length)
        .split(";")
        .map((s) => s.trim())
        .filter(Boolean);
    }
    return [];
  });

  interface ChecklistItem {
    id: string;
    label: string;
    file?: string;
    status: "done" | "active" | "failed" | "pending";
  }

  const checklistItems = $derived.by((): ChecklistItem[] => {
    const subtasks = job.report?.plan?.subtasks || [];
    if (subtasks.length === 0) return [];
    const stepsById = new Map((job.report?.steps || []).map((st) => [st.subtask_id, st]));
    let activeFound = false;
    return subtasks.map((st) => {
      const stepRes = stepsById.get(st.id);
      let status: ChecklistItem["status"] = "pending";
      if (stepRes) {
        status = stepRes.success ? "done" : "failed";
      } else if (job.status === "succeeded" || job.status === "ready_to_apply" || job.status === "preview_ready") {
        status = "done";
      } else if (isRunning && !activeFound) {
        activeFound = true;
        status = "active";
      }
      return {
        id: st.id,
        label: st.description || `${st.action} ${st.target_file || st.id}`,
        file: st.target_file || undefined,
        status
      };
    });
  });

  const commandEntries = $derived.by((): Array<{ key: string; label: string; output?: string; passed: boolean }> => {
    const out: Array<{ key: string; label: string; output?: string; passed: boolean }> = [];
    const attempts = job.report?.reflex_attempts || [];
    for (const att of attempts) {
      const r = att.Result;
      if (!r) continue;
      out.push({
        key: "reflex-" + att.Number,
        label: r.failed_stage ? `Verification stage: ${r.failed_stage} (round #${att.Number})` : `Verification pipeline (round #${att.Number})`,
        output: r.raw_output || undefined,
        passed: r.passed
      });
    }
    if (out.length === 0) {
      for (const ev of events) {
        const stg = (ev.stage || "").toLowerCase();
        if (stg === "baseline" || stg === "verifying" || stg === "verified" || stg === "worktree_ready") {
          out.push({
            key: "ev-cmd-" + ev.seq,
            label: `[${ev.stage}] ${ev.message}`,
            passed: true
          });
        }
      }
    }
    return out;
  });

  const elapsedText = $derived.by((): string => {
    const durNs = job.report?.duration;
    if (typeof durNs === "number" && durNs > 0) {
      const sec = Math.max(1, Math.round(durNs / 1e9));
      if (sec < 60) return `${sec}s`;
      const m = Math.floor(sec / 60);
      const s = sec % 60;
      return `${m}m ${s}s`;
    }
    if (job.created_at && job.updated_at) {
      const diffMs = new Date(job.updated_at).getTime() - new Date(job.created_at).getTime();
      if (diffMs > 0) {
        const sec = Math.max(1, Math.round(diffMs / 1000));
        if (sec < 60) return `${sec}s`;
        const m = Math.floor(sec / 60);
        const s = sec % 60;
        return `${m}m ${s}s`;
      }
    }
    return "";
  });

  interface ActivityItem {
    key: string;
    verb: string;
    file?: string;
    added?: number;
    deleted?: number;
    detail?: string;
    status: "done" | "active" | "failed";
  }

  const activityItems = $derived.by((): ActivityItem[] => {
    const items: ActivityItem[] = [];
    const subtasksById = new Map(
      (job.report?.plan?.subtasks || []).map((st) => [st.id, st.description])
    );
    const steps = job.report?.steps || [];
    const seenFiles = new Set<string>();

    if (steps.length > 0) {
      for (const st of steps) {
        const act = (st.action || "").toLowerCase();
        const file = st.target_file || "";
        const stats = file ? diffStatsByFile.get(file) : undefined;
        let verb = "Executed step";
        if (act === "read" || act === "inspect") {
          verb = "Read";
        } else if (act === "patch" || act === "modify" || act === "edit" || act === "write" || act === "create") {
          verb = "Edited";
          if (file) seenFiles.add(file);
        } else if (act === "verify" || act === "test") {
          verb = "Ran verification";
        } else if (act) {
          verb = act.charAt(0).toUpperCase() + act.slice(1);
        }
        const planDesc = subtasksById.get(st.subtask_id) || "";
        const detail =
          planDesc && st.message && planDesc !== st.message
            ? `${planDesc} (${st.message})`
            : planDesc || st.message || "";
        items.push({
          key: st.subtask_id + "-" + file,
          verb,
          file: file || undefined,
          added: verb === "Edited" ? stats?.added : undefined,
          deleted: verb === "Edited" ? stats?.deleted : undefined,
          detail,
          status: st.success ? "done" : "failed"
        });
      }
    } else if (events.length > 0) {
      for (const ev of events) {
        const stg = (ev.stage || "").toLowerCase();
        if (stg === "planning" || stg === "step_start" || stg === "step_done" || stg === "verifying") {
          items.push({
            key: "ev-" + ev.seq,
            verb: stg === "planning" ? "Planned changes" : stg === "verifying" ? "Running verification" : "Step",
            detail: ev.message,
            status: isRunning && ev.seq === events[events.length - 1]?.seq ? "active" : "done"
          });
        }
      }
    }

    for (const fd of fileDiffs) {
      if (!seenFiles.has(fd.file)) {
        items.push({
          key: "diff-" + fd.file,
          verb: "Edited",
          file: fd.file,
          added: fd.added,
          deleted: fd.deleted,
          status: "done"
        });
      }
    }
    return items;
  });

  const latestLiveMessage = $derived.by(() => {
    void $locale;
    if (!isRunning) return "";
    if (events.length > 0) {
      const last = events[events.length - 1];
      return `[${last.stage}] ${last.message}`;
    }
    switch (job.status) {
      case "queued":
        return tNow("Queued (preparing workspace)…", "อยู่ในคิวรอประมวลผล (กำลังเตรียมเริ่มงาน)…");
      case "analyzing":
        return tNow("Analyzing project & preparing environment…", "กำลังวิเคราะห์โปรเจกต์และเตรียมสภาพแวดล้อม…");
      case "planning":
        return tNow("Planning execution steps (Architect)…", "กำลังวางแผนขั้นตอนการแก้ไขโค้ด (Architect)…");
      case "patching":
        return tNow("Reading & editing files in Isolated Worktree…", "กำลังอ่านและแก้ไขไฟล์ใน Isolated Worktree…");
      case "verifying":
        return tNow("Running automated verification & tests (Reflex)…", "กำลังรันตรวจสอบโค้ดและเทสต์อัตโนมัติ (Reflex)…");
      case "applying":
        return tNow("Applying changes to project…", "กำลังนำการเปลี่ยนแปลงไปใช้ในโปรเจกต์…");
      default:
        return tNow("Working…", "กำลังทำงาน…");
    }
  });

  function statusLabel(st: string): string {
    switch (st) {
      case "succeeded":
        return tNow("✓ Succeeded (Applied)", "✓ เสร็จสมบูรณ์ (Applied)");
      case "ready_to_apply":
        return tNow("✓ Verified · Ready to Apply", "✓ ตรวจสอบผ่านแล้ว · พร้อม Apply");
      case "preview_ready":
        return tNow("✓ Preview Ready", "✓ สร้าง Preview เสร็จแล้ว");
      case "interrupted":
        return tNow("⏸ Paused (Ready to Resume)", "⏸ พักงานชั่วคราว (พร้อมทำต่อ)");
      case "failed":
        return tNow("✗ Failed", "✗ ไม่สำเร็จ (Failed)");
      case "cancelled":
        return tNow("⊘ Cancelled", "⊘ ยกเลิกแล้ว");
      case "queued":
        return tNow("Queued…", "รอคิวทำงาน…");
      case "analyzing":
        return tNow("Analyzing…", "กำลังวิเคราะห์…");
      case "planning":
        return tNow("Planning…", "กำลังวางแผน…");
      case "patching":
        return tNow("Editing code…", "กำลังแก้ไขโค้ด…");
      case "verifying":
        return tNow("Running verification…", "กำลังรันทดสอบ…");
      case "applying":
        return tNow("Applying…", "กำลัง Apply…");
      default:
        return st;
    }
  }

  function scrollFeedToBottom() {
    if (typeof document === "undefined") return;
    const feed = document.getElementById("feed");
    if (feed) {
      feed.scrollTo({ top: feed.scrollHeight, behavior: "smooth" });
    }
  }

  async function onApply() {
    if (busy) return;
    busy = true;
    await applyTaskChanges(job.id, "ask");
    busy = false;
    setTimeout(scrollFeedToBottom, 50);
  }

  async function onCancel() {
    if (busy) return;
    busy = true;
    await cancelTaskById(job.id);
    busy = false;
  }

  async function onResume() {
    if (busy) return;
    busy = true;
    await resumeTaskById(job.id);
    busy = false;
    setTimeout(scrollFeedToBottom, 50);
  }

  async function onSaveSkill() {
    if (busy) return;
    busy = true;
    skillNotice = "";
    const res = await createProceduralSkill({
      task_id: job.id,
      slug: "skill-" + job.id,
      title: job.instruction,
      instruction: job.instruction
    });
    skillNotice = res.ok
      ? tNow(`Saved Skill (${res.path})`, `บันทึก Skill แล้ว (${res.path})`)
      : res.message;
    busy = false;
  }

  function openDiffFor(file?: string) {
    const alreadySelected = $selectedTaskId === job.id;
    void selectTask(job.id, false);
    rightCollapsed.set(false);
    const targetFile = file || changedFiles[0] || "";
    if (file) {
      selectedDiffFile.set(file);
      rightPanelView.set("diff");
    } else if (alreadySelected && !$rightCollapsed && $rightPanelView === "diff") {
      rightPanelView.set("cards");
    } else {
      if (targetFile) selectedDiffFile.set(targetFile);
      rightPanelView.set("diff");
    }
  }

  async function openActivityItem(item: ActivityItem) {
    if (!item.file) return;
    if (item.verb === "Read") {
      void selectTask(job.id, false);
      rightCollapsed.set(false);
      const res = await fetchWorkspaceFile(item.file, job.id);
      if (res.ok) {
        rightPanelView.set("file");
        return;
      }
    }
    openDiffFor(item.file);
  }

  interface FormattedLogEvent {
    seq: number;
    stage: string;
    badgeLabel: string;
    badgeTone: "plan" | "step" | "agent" | "verify" | "save" | "warn";
    category: "steps" | "subagents" | "alerts" | "other";
    summary: string;
    fullMessage: string;
    isLong: boolean;
    timeText: string;
  }

  function formatEventItem(ev: TaskEvent, idx: number): FormattedLogEvent {
    const rawStage = (ev.stage || "event").toLowerCase();
    const rawMsg = (ev.message || "").trim();
    let badgeLabel = rawStage.toUpperCase();
    let badgeTone: FormattedLogEvent["badgeTone"] = "step";
    let category: FormattedLogEvent["category"] = "steps";

    if (rawStage === "planning" || rawStage === "plan_ready" || rawStage.startsWith("research")) {
      badgeLabel = rawStage === "plan_ready" ? "PLAN READY" : rawStage.startsWith("research") ? "CONTEXT" : "PLAN";
      badgeTone = "plan";
      category = rawStage === "research_skipped" ? "alerts" : "steps";
    } else if (rawStage.startsWith("subagent")) {
      badgeLabel = rawStage === "subagent_wave" ? "WAVE" : rawStage === "subagent_start" ? "AGENT ▶" : "AGENT ✓";
      badgeTone = "agent";
      category = "subagents";
    } else if (rawStage === "step_start" || rawStage === "step_done") {
      badgeLabel = rawStage === "step_start" ? "STEP ▶" : "STEP ✓";
      badgeTone = "step";
      category = "steps";
    } else if (rawStage === "verifying" || rawStage === "verified" || rawStage === "baseline") {
      badgeLabel = rawStage === "baseline" ? "BASELINE" : rawStage === "verified" ? "VERIFIED" : "VERIFY";
      badgeTone = "verify";
      category = "steps";
    } else if (rawStage === "checkpoint_saved" || rawStage.startsWith("worktree")) {
      badgeLabel = rawStage === "checkpoint_saved" ? "SAVED" : "WORKTREE";
      badgeTone = "save";
      category = "other";
    } else if (rawStage === "patch_retry" || rawStage === "replanning" || rawStage.includes("fail") || rawStage.includes("err")) {
      badgeLabel = rawStage === "replanning" ? "REPLAN" : "RETRY";
      badgeTone = "warn";
      category = "alerts";
    } else if (rawStage === "applying" || rawStage === "applied") {
      badgeLabel = "APPLY";
      badgeTone = "verify";
      category = "steps";
    }

    let clean = rawMsg
      .replace(/^\[SUBAGENT\s*·\s*[^\]]+\]\s*/i, "")
      .replace(/^\s*\[(read|create|patch|delete|verify)\]\s*/i, (_, verb) => `${verb.toUpperCase()}: `);

    const promptIdx = clean.search(/\b(Workspace:|Codebase Overview:|User Task:|Raw:\s*\{)/);
    if (promptIdx > 0) {
      clean = clean.slice(0, promptIdx).trim();
    }
    const firstLine = clean.split(/\r?\n/)[0].trim();
    const summary = firstLine.length > 150 ? firstLine.slice(0, 150) + "…" : firstLine || rawMsg.slice(0, 150);
    const isLong = rawMsg.length > 160 || rawMsg.includes("\n") || promptIdx > 0;

    let timeText = "";
    if (ev.timestamp) {
      const d = new Date(ev.timestamp);
      if (!Number.isNaN(d.getTime())) {
        timeText = d.toLocaleTimeString("th-TH", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
      }
    }

    return {
      seq: ev.seq ?? idx + 1,
      stage: ev.stage,
      badgeLabel,
      badgeTone,
      category,
      summary,
      fullMessage: rawMsg,
      isLong,
      timeText
    };
  }

  const structuredEvents = $derived.by((): FormattedLogEvent[] => {
    if (events.length > 0) {
      return events.map((ev, idx) => formatEventItem(ev, idx));
    }
    if (job.report?.steps?.length) {
      return job.report.steps.map((st, idx) => ({
        seq: idx + 1,
        stage: st.action,
        badgeLabel: st.success ? "STEP ✓" : "STEP ✗",
        badgeTone: st.success ? "step" : "warn",
        category: st.success ? "steps" : "alerts",
        summary: `[${st.action}] ${st.target_file || st.subtask_id}: ${st.message}`,
        fullMessage: `[${st.action}] ${st.target_file || st.subtask_id}: ${st.message}`,
        isLong: (st.message || "").length > 160,
        timeText: ""
      }));
    }
    return [];
  });

  const eventCategoryCounts = $derived.by(() => {
    let steps = 0;
    let subagentsCount = 0;
    let alerts = 0;
    for (const item of structuredEvents) {
      if (item.category === "steps") steps++;
      else if (item.category === "subagents") subagentsCount++;
      else if (item.category === "alerts") alerts++;
    }
    return { all: structuredEvents.length, steps, subagents: subagentsCount, alerts };
  });

  const filteredLogEvents = $derived.by((): FormattedLogEvent[] => {
    const q = logSearch.trim().toLowerCase();
    return structuredEvents.filter((item) => {
      if (logFilter === "steps" && item.category !== "steps") return false;
      if (logFilter === "subagents" && item.category !== "subagents") return false;
      if (logFilter === "alerts" && item.category !== "alerts") return false;
      if (q) {
        return (
          item.summary.toLowerCase().includes(q) ||
          item.stage.toLowerCase().includes(q) ||
          item.badgeLabel.toLowerCase().includes(q)
        );
      }
      return true;
    });
  });

  function toggleEventExpand(seq: number) {
    expandedEventSeqs = { ...expandedEventSeqs, [seq]: !expandedEventSeqs[seq] };
  }
</script>

<div class="task-card" data-testid="task-card" data-task-id={job.id} data-task-status={job.status}>
  <div class="hdr">
    <div class="hdr-left">
      {#if isRunning}
        <span class="pulse-dot" aria-hidden="true"></span>
      {/if}
      <span class="mode-label">{job.report?.answered ? $t("Answered", "ตอบคำถาม") : (job.mode || "execute").toUpperCase()}</span>
      <span class="sep">·</span>
      <span>{changedFiles.length} {$t(changedFiles.length === 1 ? "file" : "files", "ไฟล์")}</span>
      {#if totalAdded > 0 || totalDeleted > 0}
        <span class="diff-stat add mono">+{totalAdded}</span>
        <span class="diff-stat del mono">−{totalDeleted}</span>
      {/if}
      <span class="sep">·</span>
      {#if parsedInstruction.skills.length > 0}
        {#each parsedInstruction.skills as sk (sk)}
          <span class="skill-pill mono" data-testid="task-skill-pill">
            <span class="skill-icon">&lt;&gt;</span>
            <span>{sk}</span>
          </span>
        {/each}
      {/if}
      <span class="instr">{parsedInstruction.cleanText || job.instruction || job.id}</span>
    </div>
    <div class="hdr-right">
      {#if isRunning}
        <span class="timer-pill running mono" data-testid="task-live-timer" title={$t("Total elapsed time for this task", "เวลาที่ใช้ไปทั้งหมดในงานนี้")}>
          ⏱ {formatLiveClock(liveElapsedSec)}
        </span>
      {:else if elapsedText}
        <span class="timer-pill done mono" data-testid="task-elapsed-pill" title={$t("Total elapsed time", "เวลาที่ใช้ทั้งหมด")}>
          ⏱ {elapsedText}
        </span>
      {/if}
      <span
        class="status-pill"
        class:running={isRunning}
        class:ok={job.status === "succeeded" || job.status === "ready_to_apply" || job.status === "preview_ready"}
        class:warn={job.status === "interrupted"}
        class:err={job.status === "failed" || job.status === "cancelled"}
      >
        {#key $locale}{statusLabel(job.status)}{/key}
      </span>
    </div>
  </div>

  {#if commandEntries.length > 0}
    <div class="ran-commands-wrap" data-testid="ran-commands-wrap">
      <button
        type="button"
        class="ran-commands-toggle"
        onclick={() => (commandsExpanded = !commandsExpanded)}
        data-testid="ran-commands-toggle"
      >
        <span>Ran {commandEntries.length} {commandEntries.length === 1 ? "command" : "commands"}</span>
        {#if commandsExpanded}
          <ChevronDown size={12} />
        {:else}
          <ChevronRight size={12} />
        {/if}
      </button>
      {#if commandsExpanded}
        <div class="ran-commands-box mono selectable" data-testid="ran-commands-output">
          {#each commandEntries as cmd (cmd.key)}
            <div class="cmd-entry">
              <div class="cmd-hdr-row">
                <div class="cmd-hdr" class:ok={cmd.passed} class:err={!cmd.passed}>
                  {cmd.passed ? "✓" : "✗"} {cmd.label}
                </div>
                <button
                  type="button"
                  class="cmd-copy-btn"
                  onclick={() => flashTaskCopy("ran-" + cmd.key, cmd.output || cmd.label)}
                  title={$t("Copy command output", "คัดลอกผลลัพธ์คำสั่ง")}
                >
                  {copiedCmdKey === "ran-" + cmd.key ? $t("✓ Copied", "✓ คัดลอกแล้ว") : $t("Copy", "คัดลอก")}
                </button>
              </div>
              {#if cmd.output}
                <pre class="cmd-out">{cmd.output}</pre>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  {#if job.report?.plan?.objective}
    <div class="thinking-block" data-testid="task-thinking-block">
      <button
        type="button"
        class="thinking-toggle"
        onclick={() => (thinkingExpanded = !thinkingExpanded)}
      >
        <span>Thinking..</span>
        {#if thinkingExpanded}
          <ChevronDown size={12} />
        {:else}
          <ChevronRight size={12} />
        {/if}
      </button>
      <div class="plan-objective selectable" class:collapsed={!thinkingExpanded && !isRunning}>
        {job.report.plan.objective}
      </div>
    </div>
  {/if}

  {#if subagents.length > 0}
    <div class="subagents-block" data-testid="task-subagents-block">
      <button
        type="button"
        class="subagents-toggle"
        onclick={() => (subagentsExpanded = !subagentsExpanded)}
        data-testid="task-subagents-toggle"
      >
        <span class="sub-toggle-icon" class:spin={runningSubagentsCount > 0} class:ok={runningSubagentsCount === 0}>
          {#if runningSubagentsCount > 0}
            <RefreshCw size={12} />
          {:else}
            <Check size={12} />
          {/if}
        </span>
        <span>
          {#if runningSubagentsCount > 0}
            {runningSubagentsCount} {runningSubagentsCount === 1 ? "subagent" : "subagents"} running
          {:else}
            {subagents.length} {subagents.length === 1 ? "subagent" : "subagents"} completed
          {/if}
        </span>
        {#if subagentsExpanded}
          <ChevronDown size={13} />
        {:else}
          <ChevronRight size={13} />
        {/if}
      </button>

      {#if subagentsExpanded || runningSubagentsCount > 0}
        <div class="subagents-list">
          {#each subagents as sub (sub.id)}
            <button
              type="button"
              class="subagent-row"
              class:clickable={Boolean(sub.target_file)}
              onclick={async () => {
                void selectTask(job.id, false);
                rightCollapsed.set(false);
                if (sub.target_file) {
                  const res = await fetchWorkspaceFile(sub.target_file, job.id);
                  if (res.ok) {
                    rightPanelView.set("file");
                    return;
                  }
                }
                rightPanelView.set("cards");
              }}
            >
              <span
                class="sub-status-icon"
                class:spin={sub.status === "running"}
                class:ok={sub.status === "done"}
                class:err={sub.status === "failed"}
              >
                {#if sub.status === "running"}
                  <RefreshCw size={11} />
                {:else if sub.status === "failed"}
                  <AlertCircle size={11} />
                {:else}
                  <Check size={11} />
                {/if}
              </span>
              <span class="sub-role">{sub.role}</span>
              {#if sub.target_file}
                <span class="sub-target mono">{sub.target_file}</span>
              {/if}
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  {#if activityItems.length > 0}
    <div class="activity-stream">
      {#each activityItems as item (item.key)}
        <div class="activity-block">
          {#if item.file}
            <button type="button" class="activity-row clickable" data-file-path={item.file} onclick={() => openActivityItem(item)}>
              <span class="act-verb">{item.verb}</span>
              <span class="act-file mono">{item.file}</span>
              {#if item.added !== undefined || item.deleted !== undefined}
                <span class="diff-stat add mono">+{item.added ?? 0}</span>
                <span class="diff-stat del mono">−{item.deleted ?? 0}</span>
              {/if}
              <span class="act-chevron">›</span>
            </button>
          {:else}
            <div class="activity-row">
              <span class="act-verb">{item.verb}</span>
            </div>
          {/if}
          {#if item.detail}
            <div class="activity-detail selectable">{item.detail}</div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if changedFiles.length > 0}
    <div class="file-chips">
      {#each changedFiles as f (f)}
        {@const st = diffStatsByFile.get(f)}
        <button type="button" class="file-chip mono" data-file-path={f} onclick={() => openDiffFor(f)}>
          <span>{f}</span>
          {#if st}
            <span class="diff-stat add">+{st.added}</span>
            <span class="diff-stat del">−{st.deleted}</span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}

  {#if structuredEvents.length > 0}
    <button type="button" class="log-toggle" onclick={() => (logExpanded = !logExpanded)}>
      {#if logExpanded}
        <ChevronDown size={13} />
      {:else}
        <ChevronRight size={13} />
      {/if}
      <span>{$t(`Detailed Event Log (${structuredEvents.length} events)`, `บันทึกเหตุการณ์ละเอียด (${structuredEvents.length} รายการ)`)}</span>
    </button>
    {#if logExpanded}
      <div class="log-panel" data-testid="task-event-log-panel">
        <div class="log-toolbar">
          <div class="log-filters">
            <button
              type="button"
              class="log-filter-pill"
              class:active={logFilter === "all"}
              onclick={() => (logFilter = "all")}
            >
              {$t(`All (${eventCategoryCounts.all})`, `ทั้งหมด (${eventCategoryCounts.all})`)}
            </button>
            <button
              type="button"
              class="log-filter-pill"
              class:active={logFilter === "steps"}
              onclick={() => (logFilter = "steps")}
            >
              {$t(`Core Steps (${eventCategoryCounts.steps})`, `ขั้นตอนหลัก (${eventCategoryCounts.steps})`)}
            </button>
            {#if eventCategoryCounts.subagents > 0}
              <button
                type="button"
                class="log-filter-pill"
                class:active={logFilter === "subagents"}
                onclick={() => (logFilter = "subagents")}
              >
                Subagents ({eventCategoryCounts.subagents})
              </button>
            {/if}
            {#if eventCategoryCounts.alerts > 0}
              <button
                type="button"
                class="log-filter-pill warn"
                class:active={logFilter === "alerts"}
                onclick={() => (logFilter = "alerts")}
              >
                {$t(`Alerts / Retries (${eventCategoryCounts.alerts})`, `แจ้งเตือน/แก้ไข (${eventCategoryCounts.alerts})`)}
              </button>
            {/if}
          </div>
          <input
            type="search"
            class="log-search"
            placeholder={$t("Filter events or file names…", "ค้นหาเหตุการณ์ หรือชื่อไฟล์…")}
            bind:value={logSearch}
          />
        </div>

        <div class="log-box mono selectable">
          {#if filteredLogEvents.length === 0}
            <div class="log-empty">{$t("No events match this filter", "ไม่พบเหตุการณ์ที่ตรงกับตัวกรอง")}</div>
          {:else}
            {#each filteredLogEvents as item (item.seq)}
              <div class="log-item">
                <div class="log-row">
                  <span class="log-seq">#{item.seq}</span>
                  {#if item.timeText}
                    <span class="log-time">{item.timeText}</span>
                  {/if}
                  <span class="log-badge {item.badgeTone}">{item.badgeLabel}</span>
                  <span class="log-msg">{item.summary}</span>
                  {#if item.isLong}
                    <button
                      type="button"
                      class="log-more-btn"
                      onclick={() => toggleEventExpand(item.seq)}
                    >
                      {expandedEventSeqs[item.seq] ? $t("Collapse", "ย่อ") : $t("Full", "ดูเต็ม")}
                    </button>
                  {/if}
                </div>
                {#if item.isLong && expandedEventSeqs[item.seq]}
                  <pre class="log-full-msg">{item.fullMessage}</pre>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>
    {/if}
  {/if}

  {#if checklistItems.length > 0}
    <div class="task-checklist-card" data-testid="task-checklist-card">
      {#each checklistItems as item (item.id)}
        <div class="chk-row" class:done={item.status === "done"} class:active={item.status === "active"} class:failed={item.status === "failed"}>
          <span
            class="chk-icon"
            class:done={item.status === "done"}
            class:active={item.status === "active"}
            class:failed={item.status === "failed"}
            aria-hidden="true"
          >
            {#if item.status === "done"}
              ✓
            {:else if item.status === "failed"}
              ✗
            {/if}
          </span>
          <span class="chk-label selectable">{item.label}</span>
          {#if item.file}
            <button
              type="button"
              class="chk-file mono"
              data-file-path={item.file}
              onclick={() => openDiffFor(item.file)}
            >
              {item.file}
            </button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if !job.report?.answered}
  <ModelFlowRunner
    {job}
    {events}
    onInspectPrompt={(tr) => (inspectedTrace = tr)}
  />

  <div class="steps" role="list" aria-label={$t("Task stages", "ขั้นตอนของงาน")}>
    {#each stepBar.steps as step (step.label)}
      <i
        class="step-bar"
        class:done={step.state === "done"}
        class:active={step.state === "active"}
        class:failed={step.state === "failed"}
        data-step={step.label}
        data-state={step.state}
      ></i>
    {/each}
  </div>
  <div class="step-labels">
    {#each stepBar.steps as step (step.label)}
      <span class:active={step.state === "active" || step.state === "done"}>{step.label}</span>
    {/each}
  </div>
  {/if}

  {#if isRunning}
    <div class="live-banner">
      <span class="pulse-dot" aria-hidden="true"></span>
      <div class="live-text">
        <b>{$t("Agent is currently working", "Agent กำลังทำงานอยู่ในขณะนี้")}</b>
        <div class="mono live-sub">{latestLiveMessage}</div>
      </div>
      <div class="live-timer-box mono" data-testid="live-banner-timer">
        <span class="live-timer-main">⏱ {formatLiveClock(liveElapsedSec)}</span>
        <span class="live-timer-stage">{$t(`Stage ${formatLiveClock(liveStageSec)}`, `ขั้นตอนนี้ ${formatLiveClock(liveStageSec)}`)}</span>
      </div>
    </div>
    <div class="thinking-status-line mono" data-testid="task-thinking-status">
      <span class="thinking-dots" aria-hidden="true"></span>
      <span>
        {events.length || 1} events · Thinking · ⏱ {$t(`Total ${formatLiveClock(liveElapsedSec)} (Stage ${formatLiveClock(liveStageSec)})`, `รวม ${formatLiveClock(liveElapsedSec)} (ขั้นตอนนี้ ${formatLiveClock(liveStageSec)})`)}
      </span>
    </div>
  {/if}

  {#if job.error || actionErr}
    {#if isInterruptedRestart && !actionErr}
      <div class="interrupted-banner selectable" data-testid="task-error">
        <div class="interrupted-title">{$t("⏸ Task paused after service restart (Recovered after service restart)", "⏸ งานนี้ถูกพักชั่วคราวเนื่องจากบริการรีสตาร์ท (Recovered after service restart)")}</div>
        <div class="interrupted-desc">
          {#if $locale === "th"}
            โค้ดและขั้นตอนที่ทำสำเร็จก่อนหน้านี้ถูกบันทึกไว้ใน Git Worktree และ Checkpoint เรียบร้อยแล้ว กดปุ่ม <b>ทำต่อ (Resume)</b> ด้านล่างเพื่อทำงานต่อจากจุดเดิมได้ทันที
          {:else}
            Completed steps and code changes are safely preserved in the Git Worktree and Checkpoint. Click <b>Resume</b> below to continue from where it left off.
          {/if}
        </div>
      </div>
    {:else if unmetAcceptanceItems.length > 1}
      <div class="error-box acceptance-err selectable" data-testid="task-error">
        <div class="acc-err-hdr">
          <span>{$t(`⚠️ Unmet Acceptance Criteria (${unmetAcceptanceItems.length} items):`, `⚠️ เงื่อนไข Acceptance ที่ยังไม่ผ่าน (${unmetAcceptanceItems.length} รายการ):`)}</span>
          {#if unmetAcceptanceItems.length > 5}
            <button
              type="button"
              class="acc-expand-btn"
              onclick={() => (allAcceptanceExpanded = !allAcceptanceExpanded)}
            >
              {allAcceptanceExpanded ? $t("Show less", "ย่อรายการ") : $t(`Show all (${unmetAcceptanceItems.length})`, `ดูทั้งหมด (${unmetAcceptanceItems.length})`)}
            </button>
          {/if}
        </div>
        <ul class="acc-err-list">
          {#each allAcceptanceExpanded ? unmetAcceptanceItems : unmetAcceptanceItems.slice(0, 5) as crit, idx (idx)}
            <li>{crit}</li>
          {/each}
        </ul>
      </div>
    {:else}
      <div class="error-box selectable" data-testid="task-error">
        {actionErr || job.error}
      </div>
    {/if}
  {/if}

  {#if skillNotice}
    <div class="skill-notice">{skillNotice}</div>
  {/if}

  {#if !isRunning}
    <div
      class="completion-box"
      class:ok={job.status === "succeeded" || job.status === "ready_to_apply" || job.status === "preview_ready"}
      class:warn={job.status === "interrupted"}
      class:err={job.status === "failed" || job.status === "cancelled"}
    >
      {#if job.report?.summary}
        <div class="summary selectable">
          <MarkdownView text={job.report.summary} taskId={job.id} />
        </div>
      {/if}

      {#if elapsedText || changedFiles.length > 0}
        <div class="completion-meta-bar mono">
          {#if elapsedText}
            <span>Elapsed time: {elapsedText}</span>
          {/if}
          {#if changedFiles.length > 0}
            <button type="button" class="changes-summary-link" onclick={() => openDiffFor()}>
              <span>› {changedFiles.length} {job.report?.applied ? "changes accepted" : "files changed"} (View all)</span>
            </button>
          {/if}
        </div>
      {/if}

      <div class="completion-prompt">
        {#if job.status === "preview_ready"}
          {#if $locale === "th"}
            <b>Agent สร้าง Preview Diff เสร็จแล้ว ({changedFiles.length} ไฟล์):</b> ยังไม่ได้รันทดสอบและยังไม่ได้แก้ไขไฟล์จริงในโปรเจกต์ ต้องการรันตรวจสอบเพื่อเตรียม Apply เลยไหม?
          {:else}
            <b>Preview Diff ready ({changedFiles.length} files):</b> Verification has not run and workspace files are untouched. Run verification to prepare for Apply?
          {/if}
        {:else if job.status === "ready_to_apply"}
          {#if $locale === "th"}
            <b>Agent แก้ไขและรันตรวจสอบผ่านครบแล้ว ({changedFiles.length} ไฟล์):</b> ต้องการนำการเปลี่ยนแปลงนี้ไปใช้ (Apply) ลงโปรเจกต์เลยไหม?
          {:else}
            <b>All changes verified ({changedFiles.length} files):</b> Ready to apply these changes to your project?
          {/if}
        {:else if job.status === "succeeded"}
          <b>{$t("✓ Task completed successfully", "✓ Agent ทำงานเสร็จสมบูรณ์แล้ว")}</b>
          {job.report?.applied ? $t(" (Changes applied to Workspace)", " (นำการเปลี่ยนแปลงไปใช้ใน Workspace เรียบร้อยแล้ว)") : ""}
        {:else if job.status === "interrupted"}
          {#if $locale === "th"}
            <b>⏸ งานนี้ถูกพักชั่วคราว (Interrupted):</b> ขั้นตอนที่ทำไปแล้วถูกเก็บไว้ใน Git Worktree เรียบร้อย กด <b>ทำต่อ (Resume)</b> เพื่อให้ Agent ทำงานต่อจากจุดเดิมได้เลย
          {:else}
            <b>⏸ Task Interrupted:</b> Previous progress is saved in the Git Worktree. Click <b>Resume</b> to continue from the last checkpoint.
          {/if}
        {:else if job.status === "failed" || job.status === "cancelled"}
          {#if $locale === "th"}
            <b>งานนี้หยุดลงก่อนเสร็จสมบูรณ์:</b> ต้องการสั่งให้ Agent ทำงานต่อจากจุดเดิม (Resume) หรือเปิดดู Diff ก่อนไหม?
          {:else}
            <b>Task stopped before completion:</b> Would you like to Resume from the checkpoint or inspect the Diff first?
          {/if}
        {/if}
      </div>

      {#if $terminalScripts.length > 0 && (job.status === "succeeded" || job.status === "ready_to_apply")}
        <div class="task-launch-strip" data-testid="task-launch-strip">
          <div class="launch-strip-hdr">
            <span class="launch-strip-title">{$t("⚡ Project Run Commands & Ports (Click to run or copy)", "⚡ คำสั่งรันโปรเจกต์ & พอร์ต (กดรันหรือคัดลอกได้ทันที)")}</span>
            {#if $activeWorkspacePorts.length > 0}
              <div class="launch-active-ports">
                {#each $activeWorkspacePorts as ap (ap.port)}
                  <button
                    type="button"
                    class="launch-port-badge mono"
                    onclick={() => openExternalURL(ap.url)}
                    title={$t(`Open ${ap.url}`, `เปิด ${ap.url}`)}
                  >
                    <Globe size={11} />
                    <span>{$t(`Running: ${ap.url.replace(/^https?:\/\//i, "")}`, `กำลังรัน: ${ap.url.replace(/^https?:\/\//i, "")}`)}</span>
                    <ExternalLink size={10} />
                  </button>
                {/each}
              </div>
            {:else}
              <span class="launch-idle-note">{$t("Note: Execute mode verifies and closes processes automatically — click below to keep a dev server running", "หมายเหตุ: โหมด Execute รันตรวจสอบแล้วปิดโปรเซสอัตโนมัติ — กดปุ่มด้านล่างเพื่อเปิดเซิร์ฟเวอร์ค้างไว้")}</span>
            {/if}
          </div>

          <div class="launch-cmd-grid">
            {#each $terminalScripts.slice(0, 4) as sc (sc.id)}
              <div class="launch-cmd-row mono">
                <div class="launch-cmd-info">
                  <code class="launch-cmd-code">{sc.command}</code>
                  {#if sc.port_conflict && sc.default_port && sc.target_port}
                    <span
                      class="launch-conflict-badge"
                      title={$t(
                        `Port :${sc.default_port} is in use — APW automatically switched to free port :${sc.target_port}`,
                        `พอร์ต :${sc.default_port} มีระบบอื่นในเครื่องใช้อยู่แล้ว — APW สลับไปใช้พอร์ตว่าง :${sc.target_port} อัตโนมัติ`
                      )}
                    >
                      {$t(
                        `⚠️ Port :${sc.default_port} busy → switched to :${sc.target_port}`,
                        `⚠️ พอร์ต :${sc.default_port} ชนระบบอื่น → สลับไปใช้ :${sc.target_port}`
                      )}
                    </span>
                  {:else if sc.target_port}
                    <span class="launch-hint-url">{$t(`Port :${sc.target_port}`, `พอร์ต :${sc.target_port}`)}</span>
                  {/if}
                </div>
                <div class="launch-cmd-btns">
                  <button
                    type="button"
                    class="launch-btn run"
                    onclick={() => runCommandInAPW(sc.command, sc.hint_url, sc.target_port)}
                  >
                    <Play size={10} />
                    <span>{$t(`Run in APW${sc.target_port ? ` (:${sc.target_port})` : ""}`, `รันใน APW${sc.target_port ? ` (:${sc.target_port})` : ""}`)}</span>
                  </button>
                  <button
                    type="button"
                    class="launch-btn"
                    onclick={() => openCommandInExternalTerminal(sc.command, sc.target_port)}
                    title={$t("Run in a separate PowerShell window", "เปิดรันในหน้าต่าง PowerShell แยก")}
                  >
                    <ExternalLink size={10} />
                    <span>{$t("Open Terminal", "เปิด Terminal")}</span>
                  </button>
                  <button
                    type="button"
                    class="launch-btn"
                    onclick={() => flashTaskCopy("lsc-" + sc.id, sc.command)}
                  >
                    <Copy size={10} />
                    <span>{copiedCmdKey === "lsc-" + sc.id ? $t("✓ Copied", "✓ คัดลอกแล้ว") : $t("Copy", "คัดลอก")}</span>
                  </button>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      <div class="actions">
        {#if canApply}
          <button
            type="button"
            class="btn primary"
            onclick={onApply}
            disabled={busy}
            data-testid="apply-task-btn"
          >
            {$t("Apply to Project", "Apply ลงโปรเจกต์เลย")}
          </button>
        {/if}

        {#if canResume}
          <button
            type="button"
            class="btn primary"
            onclick={onResume}
            disabled={busy}
            data-testid="resume-task-btn"
          >
            {job.status === "preview_ready"
              ? $t("Verify & Prepare Apply (Resume)", "รันตรวจสอบและเตรียม Apply (Resume)")
              : $t("Resume", "ทำต่อ (Resume)")}
          </button>
        {/if}

        <button type="button" class="btn" onclick={() => openDiffFor()} data-testid="review-task-btn">
          {changedFiles.length > 0
            ? $t(`View Diff / Review (${changedFiles.length} files)`, `ดู diff / รีวิว (${changedFiles.length} ไฟล์)`)
            : $t("View Diff / Review", "ดู diff / รีวิว")}
        </button>

        {#if (job.status === "succeeded" || job.status === "ready_to_apply") && job.report?.passed}
          <button type="button" class="btn ghost" onclick={onSaveSkill} disabled={busy}>
            <BookmarkPlus size={12} />
            <span>Save as Skill</span>
          </button>
        {/if}
      </div>
    </div>
  {:else}
    <div class="actions">
      <button type="button" class="btn" onclick={() => openDiffFor()} data-testid="review-task-btn">
        {$t("View Diff / Review", "ดู diff / รีวิว")}
      </button>
      <button
        type="button"
        class="btn"
        onclick={onCancel}
        disabled={busy}
        data-testid="cancel-task-btn"
      >
        {$t("Cancel", "ยกเลิก (Cancel)")}
      </button>
    </div>
  {/if}
</div>

{#if inspectedTrace}
  <PromptInspectorModal
    trace={inspectedTrace}
    traces={promptTraces}
    onSelectTrace={(tr) => (inspectedTrace = tr)}
    onClose={() => (inspectedTrace = null)}
  />
{/if}

<style>
  .task-card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    box-shadow: var(--shadow-sm);
  }
  .hdr {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }
  .hdr-left {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    flex: 1;
  }
  .mode-label {
    font-size: 11px;
    color: var(--muted);
  }
  .sep {
    color: var(--faint);
  }
  .instr {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
  }
  .hdr-right {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex: none;
  }
  .timer-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--muted);
  }
  .timer-pill.running {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 45%, var(--line));
    background: var(--accent-soft);
  }
  .status-pill {
    font-weight: 600;
    font-size: 11.5px;
    color: var(--muted);
    flex: none;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--panel-subtle);
  }
  .status-pill.running {
    color: var(--accent);
    background: var(--accent-soft);
  }
  .status-pill.ok {
    color: var(--ok);
    background: var(--ok-soft);
  }
  .status-pill.warn {
    color: var(--warn);
    background: var(--panel-subtle);
  }
  .status-pill.err {
    color: var(--err);
    background: var(--err-soft);
  }
  .pulse-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
    flex: none;
    animation: pulse 1.2s ease-in-out infinite;
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.35;
      transform: scale(0.85);
    }
  }
  .live-banner {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--accent);
    background: var(--accent-soft);
    font-size: 12px;
  }
  .live-text {
    min-width: 0;
    flex: 1;
  }
  .live-sub {
    font-size: 11px;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .live-timer-box {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 2px;
    flex: none;
    padding: 3px 8px;
    border-radius: 6px;
    border: 1px solid color-mix(in srgb, var(--accent) 40%, var(--line));
    background: var(--panel);
  }
  .live-timer-main {
    font-size: 12px;
    font-weight: 700;
    color: var(--accent);
    line-height: 1.2;
  }
  .live-timer-stage {
    font-size: 10px;
    color: var(--muted);
    line-height: 1.2;
  }
  .steps {
    display: flex;
    gap: 4px;
  }
  .step-bar {
    flex: 1;
    height: 3.5px;
    border-radius: 2px;
    background: var(--line);
  }
  .step-bar.done {
    background: var(--ok);
  }
  .step-bar.active {
    background: var(--accent);
  }
  .step-bar.failed {
    background: var(--err);
  }
  .step-labels {
    display: flex;
    justify-content: space-between;
    color: var(--faint);
    font-size: 11px;
  }
  .step-labels span.active {
    color: var(--muted);
    font-weight: 500;
  }
  .plan-objective {
    font-size: 12.5px;
    color: var(--text);
    line-height: 1.55;
  }
  .skill-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 1px 6px;
    border-radius: 5px;
    border: 1px solid var(--line-strong);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11px;
    font-weight: 600;
    flex: none;
  }
  .skill-icon {
    color: var(--accent);
    font-weight: 700;
  }
  .subagents-block {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 6px 8px;
    border-radius: 8px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
  }
  .subagents-toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 12px;
    font-weight: 500;
    padding: 2px 0;
    align-self: flex-start;
  }
  .subagents-toggle:hover {
    color: var(--text);
  }
  .sub-toggle-icon,
  .sub-status-icon {
    display: inline-flex;
    align-items: center;
    color: var(--muted);
    flex: none;
  }
  .sub-toggle-icon.spin,
  .sub-status-icon.spin {
    color: var(--accent);
    animation: spinSub 1.2s linear infinite;
  }
  .sub-toggle-icon.ok,
  .sub-status-icon.ok {
    color: var(--ok);
  }
  .sub-status-icon.err {
    color: var(--err);
  }
  @keyframes spinSub {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }
  .subagents-list {
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding-left: 4px;
    border-top: 1px solid var(--line);
    padding-top: 5px;
  }
  .subagent-row {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    border: none;
    background: transparent;
    padding: 2px 4px;
    border-radius: 5px;
    font-size: 11.5px;
    color: var(--text);
    text-align: left;
  }
  .subagent-row.clickable:hover {
    background: var(--hover);
  }
  .sub-role {
    font-weight: 500;
  }
  .sub-target {
    color: var(--faint);
    font-size: 11px;
  }
  .activity-stream {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 6px 0;
    border-top: 1px solid var(--line);
    border-bottom: 1px solid var(--line);
  }
  .activity-block {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .activity-row {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--muted);
    border: none;
    background: transparent;
    padding: 2px 0;
    text-align: left;
    align-self: flex-start;
  }
  .activity-row.clickable:hover {
    color: var(--text);
  }
  .act-verb {
    color: var(--muted);
  }
  .act-file {
    color: var(--text);
    font-weight: 600;
  }
  .act-chevron {
    color: var(--faint);
    font-weight: 600;
  }
  .activity-detail {
    font-size: 12.5px;
    color: var(--text);
    line-height: 1.55;
    padding-left: 2px;
  }
  .diff-stat {
    font-size: 11px;
    font-weight: 600;
    padding: 0 5px;
    border-radius: 4px;
  }
  .diff-stat.add {
    color: var(--addt);
    background: var(--add);
  }
  .diff-stat.del {
    color: var(--delt);
    background: var(--del);
  }
  .file-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .file-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11px;
  }
  .file-chip:hover {
    background: var(--hover);
  }
  .completion-box {
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    background: var(--bg);
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .summary {
    font-size: 12.5px;
    color: var(--text);
    white-space: pre-wrap;
    line-height: 1.6;
  }
  .completion-prompt {
    font-size: 12.5px;
    color: var(--text);
    line-height: 1.55;
  }
  .error-box {
    padding: 8px 10px;
    border-radius: 6px;
    background: var(--err-soft);
    color: var(--err);
    font-size: 12px;
    white-space: pre-wrap;
  }
  .skill-notice {
    font-size: 11.5px;
    color: var(--ok);
  }
  .log-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 0;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 11.5px;
    align-self: flex-start;
  }
  .log-toggle:hover {
    color: var(--text);
  }
  .log-panel {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--panel-subtle);
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .log-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    padding: 6px 8px;
    border-bottom: 1px solid var(--line);
    background: var(--panel);
  }
  .log-filters {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .log-filter-pill {
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--muted);
    border-radius: 999px;
    padding: 2px 8px;
    font-size: 11px;
    font-weight: 500;
  }
  .log-filter-pill:hover {
    color: var(--text);
    border-color: var(--line-strong);
  }
  .log-filter-pill.active {
    background: var(--text);
    color: var(--panel);
    border-color: var(--text);
  }
  .log-filter-pill.warn:not(.active) {
    color: var(--warn);
    border-color: var(--warn);
  }
  .log-search {
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--panel-subtle);
    color: var(--text);
    padding: 3px 8px;
    font-size: 11px;
    min-width: 170px;
    outline: none;
  }
  .log-search:focus {
    border-color: var(--line-strong);
  }
  .log-box {
    padding: 6px 8px;
    font-size: 11px;
    max-height: 280px;
    resize: vertical;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .log-empty {
    padding: 12px 6px;
    color: var(--muted);
    text-align: center;
    font-size: 11.5px;
  }
  .log-item {
    display: flex;
    flex-direction: column;
    border-bottom: 1px solid color-mix(in srgb, var(--line) 55%, transparent);
    padding: 4px 2px;
  }
  .log-item:last-child {
    border-bottom: none;
  }
  .log-row {
    display: flex;
    align-items: baseline;
    gap: 7px;
    min-width: 0;
    line-height: 1.45;
  }
  .log-seq {
    color: var(--faint);
    font-size: 10px;
    min-width: 28px;
    flex: none;
  }
  .log-time {
    color: var(--faint);
    font-size: 10px;
    flex: none;
  }
  .log-badge {
    font-size: 9.5px;
    font-weight: 700;
    padding: 1px 6px;
    border-radius: 4px;
    letter-spacing: 0.02em;
    flex: none;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
  }
  .log-badge.plan {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 45%, var(--line));
  }
  .log-badge.step {
    color: var(--text);
    border-color: var(--line-strong);
  }
  .log-badge.agent {
    color: var(--ok);
    border-color: color-mix(in srgb, var(--ok) 45%, var(--line));
  }
  .log-badge.verify {
    color: var(--ok);
    background: var(--ok-soft);
    border-color: color-mix(in srgb, var(--ok) 45%, var(--line));
  }
  .log-badge.warn {
    color: var(--warn);
    border-color: var(--warn);
  }
  .log-badge.save {
    color: var(--faint);
    border-color: var(--line);
  }
  .log-msg {
    color: var(--text);
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .log-more-btn {
    flex: none;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    border-radius: 4px;
    padding: 0 6px;
    font-size: 10px;
  }
  .log-more-btn:hover {
    color: var(--text);
    border-color: var(--line-strong);
  }
  .log-full-msg {
    margin: 5px 0 2px 34px;
    padding: 6px 8px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--muted);
    font-size: 10.5px;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 180px;
    overflow-y: auto;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding-top: 2px;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 5px 12px;
    border-radius: 6px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    color: var(--text);
    font-size: 12px;
    font-weight: 500;
  }
  .btn:hover:not(:disabled) {
    background: var(--hover);
  }
  .btn.primary {
    background: var(--text);
    color: var(--panel);
    border-color: var(--text);
    font-weight: 600;
  }
  .btn.ghost {
    margin-left: auto;
    color: var(--muted);
  }
  .thinking-block {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .thinking-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 11.5px;
    font-weight: 500;
    padding: 0;
    align-self: flex-start;
  }
  .thinking-toggle:hover {
    color: var(--text);
  }
  .plan-objective.collapsed {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .ran-commands-wrap {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .ran-commands-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    border: none;
    background: transparent;
    color: var(--muted);
    font-size: 12px;
    font-weight: 500;
    padding: 2px 0;
    align-self: flex-start;
  }
  .ran-commands-toggle:hover {
    color: var(--text);
  }
  .ran-commands-box {
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--panel-subtle);
    padding: 8px 10px;
    max-height: 200px;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: 8px;
    font-size: 11px;
  }
  .cmd-hdr {
    font-weight: 600;
    color: var(--text);
  }
  .cmd-hdr.ok {
    color: var(--ok);
  }
  .cmd-hdr.err {
    color: var(--err);
  }
  .cmd-out {
    margin: 4px 0 0;
    padding: 6px 8px;
    border-radius: 6px;
    background: var(--bg);
    color: var(--muted);
    overflow-x: auto;
    white-space: pre;
    font-size: 11px;
    line-height: 1.5;
  }
  .task-checklist-card {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--panel-subtle);
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  .chk-row {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--muted);
    line-height: 1.45;
  }
  .chk-row.done,
  .chk-row.active {
    color: var(--text);
  }
  .chk-row.failed {
    color: var(--err);
  }
  .chk-icon {
    width: 15px;
    height: 15px;
    border-radius: 50%;
    border: 1.5px solid var(--faint);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 9.5px;
    font-weight: 700;
    flex: none;
  }
  .chk-icon.done {
    background: var(--text);
    color: var(--panel);
    border-color: var(--text);
  }
  .chk-icon.active {
    border-style: dashed;
    border-color: var(--accent);
    animation: spinSub 1.4s linear infinite;
  }
  .chk-icon.failed {
    border-color: var(--err);
    color: var(--err);
  }
  .chk-label {
    flex: 1;
    min-width: 0;
  }
  .chk-file {
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    border-radius: 4px;
    padding: 1px 6px;
    font-size: 10.5px;
    flex: none;
  }
  .chk-file:hover {
    color: var(--text);
    border-color: var(--line-strong);
  }
  .thinking-status-line {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 11.5px;
    color: var(--muted);
    padding: 2px 0;
  }
  .thinking-dots {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .interrupted-banner {
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--warn);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 12px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .interrupted-title {
    font-weight: 600;
    color: var(--warn);
  }
  .interrupted-desc {
    color: var(--muted);
    line-height: 1.5;
  }
  .acc-err-hdr {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    font-weight: 600;
    margin-bottom: 6px;
  }
  .acc-expand-btn {
    border: 1px solid var(--err);
    background: transparent;
    color: var(--err);
    border-radius: 4px;
    padding: 1px 6px;
    font-size: 11px;
  }
  .acc-err-list {
    margin: 0;
    padding-left: 18px;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .completion-meta-bar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
    font-size: 11px;
    color: var(--muted);
    padding-top: 4px;
    border-top: 1px solid var(--line);
  }
  .changes-summary-link {
    border: none;
    background: transparent;
    color: var(--accent);
    font-size: 11px;
    font-weight: 600;
    padding: 0;
  }
  .changes-summary-link:hover {
    text-decoration: underline;
  }
  .cmd-hdr-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .cmd-copy-btn {
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--muted);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 10px;
  }
  .cmd-copy-btn:hover {
    background: var(--hover);
    color: var(--text);
  }
  .task-launch-strip {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line));
    background: var(--panel);
  }
  .launch-strip-hdr {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
  }
  .launch-strip-title {
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text);
  }
  .launch-idle-note {
    font-size: 11px;
    color: var(--muted);
  }
  .launch-active-ports {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .launch-port-badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid var(--ok);
    background: var(--ok-soft);
    color: var(--ok);
    font-size: 11px;
    font-weight: 600;
  }
  .launch-cmd-grid {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .launch-cmd-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    flex-wrap: wrap;
    padding: 4px 8px;
    border-radius: 6px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
  }
  .launch-cmd-info {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .launch-cmd-code {
    font-size: 11.5px;
    font-weight: 600;
    color: var(--text);
  }
  .launch-hint-url {
    border: none;
    padding: 1px 6px;
    border-radius: 4px;
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 11px;
    font-weight: 500;
  }
  .launch-conflict-badge {
    padding: 1px 7px;
    border-radius: 4px;
    background: var(--warn-soft);
    color: var(--warn);
    font-size: 11px;
    font-weight: 600;
  }
  .launch-cmd-btns {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    flex-wrap: wrap;
  }
  .launch-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
    font-size: 11px;
    font-weight: 500;
  }
  .launch-btn.run {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
  }
  .launch-btn:hover {
    background: var(--hover);
  }
</style>
