import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import { mapTaskSteps } from "./lib/stores/tasks";
import { detectActiveToken, queryMentionCandidates } from "./lib/stores/search";
import { parseUnifiedDiff, tokenizeCodeLines, MAX_HIGHLIGHT_LINES } from "./lib/diff";
import TaskCard from "./components/TaskCard.svelte";
import ChangesCard from "./components/cards/ChangesCard.svelte";
import VerificationCard from "./components/cards/VerificationCard.svelte";
import DiffView from "./components/DiffView.svelte";
import type { TaskJob, SearchPayload } from "./lib/api";

describe("stores & helpers", () => {
  it("maps real task status to Plan -> Patch -> Verify -> Apply steps without simulated progress", () => {
    const runningJob: TaskJob = {
      id: "task-1",
      instruction: "แก้บั๊ก governor",
      mode: "execute",
      status: "running",
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
      status: "running"
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
    const job: TaskJob = {
      id: "task-cards",
      instruction: "test cards",
      mode: "execute",
      status: "ready_to_apply",
      report: {
        modified_files: ["main.go"],
        diff: `--- a/main.go\n+++ b/main.go\n@@ -1,2 +1,3 @@\n package main\n-func old() {}\n+func newOne() {}\n+func newTwo() {}\n`,
        reflex_attempts: [
          {
            Number: 1,
            Result: {
              passed: false,
              failed_stage: "go test",
              stages: [
                { name: "go vet", passed: true },
                { name: "go test", passed: false, output: "FAIL: TestSomething (0.01s)" }
              ]
            }
          }
        ]
      }
    };

    render(ChangesCard, { job });
    expect(screen.getByTestId("changes-card")).toHaveTextContent("main.go");
    expect(screen.getByTestId("changes-card")).toHaveTextContent("+2");
    expect(screen.getByTestId("changes-card")).toHaveTextContent("−1");

    render(VerificationCard, { job });
    expect(screen.getByTestId("verification-card")).toHaveTextContent("1/2");
    expect(screen.getByTestId("verification-log")).toHaveTextContent("FAIL: TestSomething (0.01s)");
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
  });
});
