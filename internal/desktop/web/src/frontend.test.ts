import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/svelte";
import { mapTaskSteps, extractTaskSubagents } from "./lib/stores/tasks";
import { detectActiveToken, queryMentionCandidates, parseInstructionSkillPills } from "./lib/stores/search";
import { workspaceFiles } from "./lib/stores/workspace";
import { parseUnifiedDiff, tokenizeCodeLines, MAX_HIGHLIGHT_LINES } from "./lib/diff";
import TaskCard from "./components/TaskCard.svelte";
import Message from "./components/Message.svelte";
import SubagentsCard from "./components/cards/SubagentsCard.svelte";
import FileTreeView from "./components/FileTreeView.svelte";
import AgentBoardView from "./components/AgentBoardView.svelte";
import ChangesCard from "./components/cards/ChangesCard.svelte";
import VerificationCard from "./components/cards/VerificationCard.svelte";
import DiffView from "./components/DiffView.svelte";
import type { TaskJob, SearchPayload } from "./lib/api";
import failedJobFixture from "./fixtures/failed-job.json";

describe("stores & helpers", () => {
  it("maps real task status to Plan -> Patch -> Verify -> Apply steps without simulated progress", () => {
    const runningJob: TaskJob = {
      id: "task-1",
      instruction: "แก้บั๊ก governor",
      mode: "execute",
      status: "patching",
      report: { plan: { plan_id: "p1", objective: "obj" }, steps: [{ subtask_id: "s1", action: "modify", success: true, message: "ok" }] }
    };
    const runningSteps = mapTaskSteps(runningJob);
    expect(runningSteps.steps).toEqual([
      { label: "Plan", state: "done" },
      { label: "Patch", state: "active" },
      { label: "Verify", state: "pending" },
      { label: "Apply", state: "pending" }
    ]);

    const readyJob: TaskJob = { ...runningJob, status: "ready_to_apply" };
    expect(mapTaskSteps(readyJob).steps).toEqual([
      { label: "Plan", state: "done" },
      { label: "Patch", state: "done" },
      { label: "Verify", state: "done" },
      { label: "Apply", state: "active" }
    ]);

    const doneJob: TaskJob = { ...runningJob, status: "succeeded" };
    expect(mapTaskSteps(doneJob).steps.every((s) => s.state === "done")).toBe(true);
  });

  it("discards stale mention search responses when a newer query supersedes it", async () => {
    let resolveFirst!: (val: SearchPayload | null) => void;
    const firstPromise = new Promise<SearchPayload | null>((r) => {
      resolveFirst = r;
    });
    const secondPromise = Promise.resolve<SearchPayload>({
      files: ["src/new_match.ts"],
      symbols: [],
      skills: []
    });

    const fetcher = vi
      .fn<(q: string) => Promise<SearchPayload | null>>()
      .mockReturnValueOnce(firstPromise)
      .mockReturnValueOnce(secondPromise);

    const tok1 = detectActiveToken("@old", 4);
    const tok2 = detectActiveToken("@new", 4);

    const p1 = queryMentionCandidates(tok1, undefined, fetcher);
    const p2 = queryMentionCandidates(tok2, undefined, fetcher);

    const res2 = await p2;
    expect(res2.map((c) => c.insert)).toEqual(["src/new_match.ts"]);

    resolveFirst({ files: ["src/stale_old.ts"], symbols: [], skills: [] });
    const res1 = await p1;
    expect(res1).toEqual([]);
  });
});

describe("components", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders TaskCard actions per status and keeps Thai text intact", () => {
    const readyJob: TaskJob = {
      id: "task-thai-1",
      instruction: "ปรับปรุงหน้าตั้งค่าภาษาไทยและตรวจสอบสระไม่ซ้อนทับ",
      mode: "execute",
      status: "ready_to_apply",
      report: {
        passed: true,
        modified_files: ["internal/desktop/server.go"],
        summary: "แก้ไขการแสดงผลภาษาไทยเรียบร้อยแล้ว"
      }
    };

    const { unmount } = render(TaskCard, { props: { job: readyJob, events: [] } });
    expect(screen.getByText("ปรับปรุงหน้าตั้งค่าภาษาไทยและตรวจสอบสระไม่ซ้อนทับ")).toBeInTheDocument();
    expect(screen.getByTestId("apply-task-btn")).toBeInTheDocument();
    expect(screen.queryByTestId("cancel-task-btn")).not.toBeInTheDocument();
    unmount();

    const runningJob: TaskJob = {
      ...readyJob,
      status: "patching"
    };
    const r2 = render(TaskCard, { props: { job: runningJob, events: [] } });
    expect(screen.getByTestId("cancel-task-btn")).toBeInTheDocument();
    expect(screen.queryByTestId("apply-task-btn")).not.toBeInTheDocument();
    r2.unmount();

    const failedJob: TaskJob = {
      ...readyJob,
      status: "failed",
      error: "go test failed"
    };
    render(TaskCard, { props: { job: failedJob, events: [] } });
    expect(screen.getByTestId("resume-task-btn")).toBeInTheDocument();
    expect(screen.getByTestId("task-error")).toHaveTextContent("go test failed");
  });

  it("renders ChangesCard with +/- counts and VerificationCard with expandable failed log", async () => {
    // Real report shape (generated from the Go types by internal/desktop/devfixture).
    const job = failedJobFixture as unknown as TaskJob;

    render(ChangesCard, { job });
    expect(screen.getByTestId("changes-card")).toHaveTextContent("internal/store/autonomy.go");
    expect(screen.getByTestId("changes-card")).toHaveTextContent("+2");
    expect(screen.getByTestId("changes-card")).toHaveTextContent("−1");

    render(VerificationCard, { job });
    expect(screen.getByTestId("verification-card")).toHaveTextContent("0/2");
    expect(screen.getAllByTestId("verification-log")[0]).toHaveTextContent("--- FAIL: TestGovernorPause");
  });

  it("highlights supported languages in DiffView, falls back to plain text on unknown language and >3000-line diffs", async () => {
    const goTokens = await tokenizeCodeLines(["package main", "func Hello() int { return 42 }"], "main.go", false);
    expect(goTokens).not.toBeNull();
    expect(goTokens!.length).toBe(2);

    const unknownTokens = await tokenizeCodeLines(["some custom content"], "file.unknownext", false);
    expect(unknownTokens).toBeNull();

    const bigLines: string[] = ["--- a/big.go", "+++ b/big.go", "@@ -1,1 +1,3005 @@"];
    for (let i = 0; i < MAX_HIGHLIGHT_LINES + 5; i++) {
      bigLines.push(`+line ${i}`);
    }
    const bigJob: TaskJob = {
      id: "task-big",
      instruction: "big diff",
      mode: "preview",
      status: "preview_ready",
      report: {
        modified_files: ["big.go"],
        diff: bigLines.join("\n")
      }
    };
    render(DiffView, { job: bigJob });
    expect(screen.getByTestId("plain-fallback-banner")).toBeInTheDocument();
  }, 30_000); // first Shiki grammar/theme load takes ~4s, more on a busy machine

  it("renders Skill Pills, SubagentsCard, and FileTreeView", async () => {
    const parsed = parseInstructionSkillPills("@skill:diagram-design ช่วยทำ diagram ของทุกระบบ");
    expect(parsed.skills).toEqual(["diagram-design"]);
    expect(parsed.cleanText).toBe("ช่วยทำ diagram ของทุกระบบ");

    const { unmount: unmountMsg } = render(Message, {
      props: {
        turn: {
          index: 1,
          timestamp: "2026-10-21T00:00:00Z",
          instruction: "@skill:diagram-design ช่วยทำ diagram ของทุกระบบ",
          success: true,
          applied: false,
          summary: "done"
        }
      }
    });
    expect(screen.getByTestId("skill-pill")).toHaveTextContent("diagram-design");
    unmountMsg();

    const subJob: TaskJob = {
      id: "task-sub-1",
      instruction: "@skill:diagram-design วิเคราะห์ระบบ",
      mode: "review",
      status: "planning",
      report: {}
    };
    const subEvents = [
      {
        seq: 1,
        stage: "subagent_start",
        message: "[SUBAGENT · start] Backend Codebase Auditor — internal/app/service.go (auditor-backend)"
      },
      {
        seq: 2,
        stage: "subagent_start",
        message: "[SUBAGENT · start] Frontend Codebase Auditor — internal/desktop/web/src/App.svelte (auditor-frontend)"
      }
    ];
    const extracted = extractTaskSubagents(subJob, subEvents);
    expect(extracted.length).toBe(2);
    expect(extracted[0].role).toBe("Backend Codebase Auditor");
    expect(extracted[1].role).toBe("Frontend Codebase Auditor");

    const { unmount: unmountSub } = render(SubagentsCard, { props: { job: subJob, events: subEvents } });
    expect(screen.getByTestId("subagents-card")).toHaveTextContent("Subagents (2)");
    expect(screen.getByTestId("subagents-card")).toHaveTextContent("2 running");
    unmountSub();

    workspaceFiles.set([
      { path: ".apw/skills/diagram-design/SKILL.md", size: 120 },
      { path: "cmd/apw-desktop/main.go", size: 340 },
      { path: "internal/executor/subagent.go", size: 900 }
    ]);
    render(FileTreeView);
    expect(screen.getByTestId("files-tree-view")).toHaveTextContent(".apw");
    expect(screen.getByTestId("files-tree-view")).toHaveTextContent("cmd");
    expect(screen.getByTestId("files-tree-view")).toHaveTextContent("internal");
  });

  it("renders AgentBoardView with Interactive Diagram Canvas, Structural Diff (Auto-collapsed Tests/Docs), Decision Log Red Pins, and Privacy Telemetry switch", async () => {
    render(AgentBoardView);
    expect(screen.getByTestId("agent-board-view")).toBeInTheDocument();
    expect(screen.getByTestId("board-readonly-badge")).toHaveTextContent("READ-ONLY REVIEW");

    // 1. Visual Sequence Lifeline SVG & XYFlow Interactive Node Canvas are rendered
    const sequence = screen.getByTestId("visual-sequence-diagram");
    expect(within(sequence).getByRole("img", { name: /Sequence Diagram/ })).toBeInTheDocument();
    expect(screen.getByTestId("xyflow-canvas-shell")).toBeInTheDocument();
    expect(screen.getByTestId("xyflow-viewport")).toBeInTheDocument();

    // Highlight element on Diagram Canvas -> Visual Feedback Loop dock shows up
    const highlightBtn = screen.getByTestId("highlight-node-mcp");
    await fireEvent.click(highlightBtn);
    expect(screen.getByTestId("copy-visual-feedback-btn")).toBeInTheDocument();

    // 2. Structural Diff has Core expanded by default and Tests/Docs auto-collapsed
    expect(screen.getByTestId("diff-group-core")).toHaveTextContent("INPUT");
    expect(screen.getByTestId("diff-group-test")).toHaveTextContent("พับเก็บอัตโนมัติ");
    await fireEvent.click(screen.getByTestId("toggle-test-group-btn"));
    expect(screen.getByTestId("diff-group-test")).toHaveTextContent("กางออกแล้ว");

    // 3. Decision Log shows red pins for unprompted agent decisions
    expect(screen.getAllByTestId("red-pin-indicator").length).toBeGreaterThan(0);

    // 4. Privacy telemetry switch can be toggled
    const telemetryToggle = screen.getByTestId("telemetry-switch-input") as HTMLInputElement;
    expect(telemetryToggle.checked).toBe(false);
    await fireEvent.click(telemetryToggle);
    expect(telemetryToggle.checked).toBe(true);
  });
});
