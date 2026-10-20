// Tests on fixtures generated from the real Go types (internal/desktop/devfixture). If a Go field
// is renamed, TestFrontendFixturesMatchGoTypes fails until the fixtures are regenerated, and then
// these tests fail until the frontend follows.
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import { get } from "svelte/store";
import readyJob from "./fixtures/ready-job.json";
import failedJob from "./fixtures/failed-job.json";
import previewJob from "./fixtures/preview-job.json";
import statuses from "./fixtures/task-statuses.json";
import type { TaskJob } from "./lib/api";
import { ACTIVE_TASK_STATUSES, RESUMABLE_TASK_STATUSES, mapTaskSteps } from "./lib/stores/tasks";
import { isBlockedBrowserShortcut } from "./lib/shell";
import { collapsedCards, contextMenu, toggleCardCollapsed } from "./lib/stores/settings";
import VerificationCard from "./components/cards/VerificationCard.svelte";
import ChangesCard from "./components/cards/ChangesCard.svelte";
import TaskCard from "./components/TaskCard.svelte";
import ContextMenu from "./components/ContextMenu.svelte";

const asJob = (j: unknown) => j as TaskJob;

describe("contract with the Go API types", () => {
  it("shows the failed reflex stage, its raw output and the acceptance evidence", () => {
    render(VerificationCard, { job: asJob(failedJob) });
    const card = screen.getByTestId("verification-card");
    expect(card).toHaveTextContent("0/2");
    expect(card).toHaveTextContent("Reflex (test)");
    expect(card).toHaveTextContent("pause keeps configured limits");
    const logs = screen.getAllByTestId("verification-log").map((el) => el.textContent || "");
    expect(logs.some((l) => l.includes("--- FAIL: TestGovernorPause"))).toBe(true);
    expect(logs.some((l) => l.includes("max_tasks_per_hour reset to 10"))).toBe(true);
  });

  it("counts passed verification and acceptance for a ready task", () => {
    render(VerificationCard, { job: asJob(readyJob) });
    const card = screen.getByTestId("verification-card");
    expect(card).toHaveTextContent("2/2");
    expect(card).toHaveTextContent("pause keeps configured limits");
  });

  it("lists the changed file with line counts from the real diff", () => {
    render(ChangesCard, { job: asJob(previewJob) });
    const card = screen.getByTestId("changes-card");
    expect(card).toHaveTextContent("internal/store/autonomy.go");
    expect(card).toHaveTextContent("+2");
    expect(card).toHaveTextContent("−1");
  });

  it("polls exactly the in-flight core statuses", () => {
    expect([...ACTIVE_TASK_STATUSES].sort()).toEqual([...statuses.in_flight].sort());
  });

  it("offers Resume for every status the service can resume, including interrupted", () => {
    expect([...RESUMABLE_TASK_STATUSES].sort()).toEqual([...statuses.resumable].sort());
    for (const status of statuses.resumable) {
      const { unmount } = render(TaskCard, { props: { job: { ...asJob(failedJob), status }, events: [] } });
      expect(screen.getByTestId("resume-task-btn")).toBeInTheDocument();
      unmount();
    }
  });

  it("maps a real in-flight status (patching) onto the step bar", () => {
    const steps = mapTaskSteps({ ...asJob(readyJob), status: "patching" }).steps;
    expect(steps.map((s) => s.state)).toEqual(["done", "active", "pending", "pending"]);
  });
});

describe("native shell behaviour", () => {
  const key = (init: KeyboardEventInit) => new KeyboardEvent("keydown", init);

  it("blocks browser shortcuts by physical key, so they stay blocked under the Thai layout", () => {
    // Kedmanee: Ctrl+R reports e.key "พ"; only e.code identifies the key.
    expect(isBlockedBrowserShortcut(key({ key: "พ", code: "KeyR", ctrlKey: true }))).toBe(true);
    expect(isBlockedBrowserShortcut(key({ key: "ด", code: "KeyF", ctrlKey: true }))).toBe(true);
    expect(isBlockedBrowserShortcut(key({ key: "F5", code: "F5" }))).toBe(true);
    expect(isBlockedBrowserShortcut(key({ key: "ฉ", code: "KeyI", ctrlKey: true, shiftKey: true }))).toBe(true);
    expect(isBlockedBrowserShortcut(key({ key: "ั", code: "KeyB", ctrlKey: true }))).toBe(false);
    expect(isBlockedBrowserShortcut(key({ key: "r", code: "KeyR" }))).toBe(false);
  });

  it("persists collapsed cards so they survive a restart", () => {
    // Node's own (file-less) localStorage global shadows jsdom's; give the test a working one.
    const mem = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (k: string) => mem.get(k) ?? null,
      setItem: (k: string, v: string) => void mem.set(k, String(v)),
      removeItem: (k: string) => void mem.delete(k),
      clear: () => mem.clear()
    });
    collapsedCards.set({});
    toggleCardCollapsed("verification");
    expect(get(collapsedCards).verification).toBe(true);
    const saved = JSON.parse(localStorage.getItem("apw_desktop_ui_v2") || "{}");
    expect(saved.collapsed_cards).toEqual({ verification: true });
  });
});

describe("app context menu", () => {
  beforeEach(() => {
    contextMenu.set(null);
    vi.restoreAllMocks();
  });

  it("copies a file path and closes; Explorer is offered only inside the native shell", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    render(ContextMenu);
    contextMenu.set({ x: 10, y: 20, filePath: "internal/store/autonomy.go" } as never);
    await Promise.resolve();

    const menu = await screen.findByTestId("app-context-menu");
    expect(menu).toHaveTextContent("Copy path");
    expect(menu).not.toHaveTextContent("Explorer");
    await fireEvent.click(screen.getByText(/Copy path/));
    expect(writeText).toHaveBeenCalledWith("internal/store/autonomy.go");
    expect(get(contextMenu)).toBeNull();
  });
});
