import { test, expect } from "@playwright/test";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

interface MockJob {
  id: string;
  session_id?: string;
  instruction: string;
  mode: string;
  status: string;
  error?: string;
  worktree_path?: string;
  report?: any;
}

test("Desktop completion gate: create conversation -> review -> preview -> inspect diff -> execute/verify -> cancel/resume -> apply -> reload keeps task", async ({
  page
}) => {
  // This E2E test runs against a real `apw desktop --no-open` URL when APW_DESKTOP_URL is set,
  // or spins up a local contract server serving the built dist/ with the exact desktop.Server API.
  const externalUrl = process.env.APW_DESKTOP_URL;
  let baseUrl = externalUrl || "";
  let closeServer: (() => Promise<void>) | null = null;

  if (!baseUrl) {
    const fs = await import("node:fs");
    const path = await import("node:path");
    const distDir = path.resolve(import.meta.dirname, "../dist");
    const token = "e2e-test-session-token-12345";

    const sessions: Array<{ id: string; updated_at: string; turn_count: number; last_instruction: string }> = [];
    const turnsBySession: Record<string, any[]> = {};
    const jobs: MockJob[] = [];

    const srv = createServer(async (req: IncomingMessage, res: ServerResponse) => {
      const url = new URL(req.url || "/", "http://127.0.0.1");
      if (url.pathname === "/") {
        const html = fs
          .readFileSync(path.join(distDir, "index.html"), "utf8")
          .replace("__APW_TOKEN__", token);
        res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
        res.end(html);
        return;
      }
      if (url.pathname.startsWith("/assets/")) {
        const assetPath = path.join(distDir, url.pathname);
        if (fs.existsSync(assetPath) && fs.statSync(assetPath).isFile()) {
          const ext = path.extname(assetPath);
          const mime =
            ext === ".js"
              ? "application/javascript"
              : ext === ".css"
                ? "text/css"
                : ext === ".woff2"
                  ? "font/woff2"
                  : "application/octet-stream";
          res.writeHead(200, { "Content-Type": mime });
          res.end(fs.readFileSync(assetPath));
          return;
        }
      }

      if (req.headers.authorization !== `Bearer ${token}`) {
        res.writeHead(401, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: "unauthorized", message: "missing token" }));
        return;
      }

      const readBody = () =>
        new Promise<any>((resolve) => {
          let raw = "";
          req.on("data", (c) => (raw += c));
          req.on("end", () => {
            try {
              resolve(raw ? JSON.parse(raw) : {});
            } catch {
              resolve({});
            }
          });
        });

      const sendJSON = (data: unknown, status = 200) => {
        res.writeHead(status, { "Content-Type": "application/json" });
        res.end(JSON.stringify(data));
      };

      if (url.pathname === "/api/status") {
        return sendJSON({
          version: "0.1.0-e2e",
          brain1_name: "Reflex Engine",
          brain1_model: "APW Flash",
          brain1_state: "Available",
          brain1_latency_ms: 4,
          brain2_name: "Architect",
          brain2_model: "APW Opus",
          brain2_state: "Standby",
          daemon_running: true,
          daemon_pid: 4242,
          skills_loaded: 3,
          workspace: "D:/APW_Project/apw-agent",
          stack: "Go",
          trusted: true,
          sandbox_mode: "off",
          instructions_loaded: true,
          instructions_path: "APW.md",
          instructions_lines: 12,
          instructions_size: 240
        });
      }
      if (url.pathname === "/api/sessions" && req.method === "GET") {
        return sendJSON(sessions);
      }
      if (url.pathname === "/api/sessions" && req.method === "POST") {
        const id = `conv-${sessions.length + 1}`;
        turnsBySession[id] = [];
        return sendJSON({ id });
      }
      if (url.pathname.startsWith("/api/sessions/")) {
        const id = decodeURIComponent(url.pathname.slice("/api/sessions/".length));
        return sendJSON({ id, turns: turnsBySession[id] || [] });
      }
      if (url.pathname === "/api/approvals") {
        return sendJSON({ pending: [], always_allowed: [] });
      }
      if (url.pathname === "/api/autonomy") {
        return sendJSON({
          governor: {
            max_tasks_per_hour: 6,
            max_consecutive_failures: 3,
            consecutive_failures: 0,
            paused: false
          },
          goals: [],
          schedules: [],
          episodes: [],
          skills: [],
          skills_loaded: 3
        });
      }
      if (url.pathname === "/v1/tasks" && req.method === "GET") {
        return sendJSON(jobs);
      }
      if (url.pathname.startsWith("/v1/tasks/")) {
        const rest = url.pathname.slice("/v1/tasks/".length);
        if (rest.endsWith("/events")) {
          return sendJSON([
            { seq: 1, stage: "plan", message: "Planned changes" },
            { seq: 2, stage: "patch", message: "Patched files" }
          ]);
        }
        if (rest.endsWith("/cancel")) {
          const id = rest.slice(0, -"/cancel".length);
          const job = jobs.find((j) => j.id === id);
          if (job) job.status = "cancelled";
          return sendJSON(job || {});
        }
        if (rest.endsWith("/resume")) {
          const id = rest.slice(0, -"/resume".length);
          const job = jobs.find((j) => j.id === id);
          if (job) {
            job.status = "ready_to_apply";
            job.report = {
              passed: true,
              modified_files: ["internal/store/autonomy.go"],
              summary: "ทำงานต่อจาก checkpoint จนผ่านการตรวจสอบ",
              diff: "--- a/internal/store/autonomy.go\n+++ b/internal/store/autonomy.go\n@@ -10,2 +10,3 @@\n func UpdateGovernor() {\n+  // verified fix\n }\n",
              reflex_attempts: [
                {
                  Number: 1,
                  Result: {
                    passed: true,
                    stages: [
                      { name: "go vet", passed: true },
                      { name: "go test", passed: true }
                    ]
                  }
                }
              ]
            };
          }
          return sendJSON(job || {});
        }
        const job = jobs.find((j) => j.id === rest);
        return sendJSON(job || {});
      }
      if (url.pathname === "/api/run" && req.method === "POST") {
        const body = await readBody();
        const id = `task-${jobs.length + 1}`;
        const mode = body.mode || "execute";
        const status =
          mode === "review"
            ? "succeeded"
            : mode === "preview"
              ? "preview_ready"
              : "running";
        const job: MockJob = {
          id,
          session_id: body.session_id,
          instruction: body.instruction,
          mode,
          status,
          worktree_path: `apw/${id}`,
          report: {
            passed: status !== "running",
            modified_files: ["internal/store/autonomy.go"],
            summary: `${mode.toUpperCase()} summary for ${body.instruction}`,
            diff: "--- a/internal/store/autonomy.go\n+++ b/internal/store/autonomy.go\n@@ -1,2 +1,3 @@\n package store\n-var oldLimit = 0\n+var defaultLimit = 6\n+var keepExisting = true\n"
          }
        };
        jobs.unshift(job);
        if (body.session_id) {
          const turns = turnsBySession[body.session_id] || [];
          turns.push({
            index: turns.length + 1,
            task_id: id,
            timestamp: new Date().toISOString(),
            instruction: body.instruction,
            success: true,
            applied: false,
            summary: job.report.summary
          });
          turnsBySession[body.session_id] = turns;
          const existingSess = sessions.find((s) => s.id === body.session_id);
          if (existingSess) {
            existingSess.last_instruction = body.instruction;
            existingSess.turn_count = turns.length;
            existingSess.updated_at = new Date().toISOString();
          } else {
            sessions.unshift({
              id: body.session_id,
              updated_at: new Date().toISOString(),
              turn_count: turns.length,
              last_instruction: body.instruction
            });
          }
        }
        return sendJSON(job);
      }
      if (url.pathname.startsWith("/api/tasks/") && url.pathname.endsWith("/apply")) {
        const id = url.pathname.slice("/api/tasks/".length, -"/apply".length);
        const job = jobs.find((j) => j.id === id);
        if (job) {
          job.status = "succeeded";
        }
        return sendJSON(job || {});
      }
      return sendJSON({});
    });

    await new Promise<void>((resolve) => srv.listen(0, "127.0.0.1", resolve));
    const addr = srv.address() as { port: number };
    baseUrl = `http://127.0.0.1:${addr.port}`;
    closeServer = () => new Promise<void>((resolve) => srv.close(() => resolve()));
  }

  try {
    await page.goto(baseUrl);
    await expect(page.getByTestId("app-shell")).toBeVisible();

    // 1. Create a new conversation
    await page.getByTestId("new-conversation-btn").click();

    // 2. Review
    await page.getByTestId("composer-input").fill("/review ตรวจสอบไฟล์ autonomy.go");
    await page.getByTestId("composer-send-btn").click();
    await expect(page.getByText("ตรวจสอบไฟล์ autonomy.go").first()).toBeVisible();

    // 3. Preview & inspect diff
    await page.getByTestId("composer-input").fill("/preview เตรียมแก้ limit ของ governor");
    await page.getByTestId("composer-send-btn").click();
    await expect(page.getByTestId("changes-card")).toContainText("internal/store/autonomy.go");
    await page.getByTestId("changes-file-btn").first().click();
    await expect(page.getByTestId("diff-view")).toBeVisible();
    await expect(page.getByTestId("diff-lines")).toContainText("defaultLimit");
    await page.getByTestId("diff-back-btn").click();

    // 4. Execute -> Cancel -> Resume -> Verify -> Apply
    await page.getByTestId("composer-input").fill("แก้ไขและรันทดสอบ governor");
    await page.getByTestId("composer-send-btn").click();

    const task3Card = page.locator('[data-task-id="task-3"]');
    const cancelBtn = task3Card.getByTestId("cancel-task-btn");
    await expect(cancelBtn).toBeVisible();
    await cancelBtn.click();

    const resumeBtn = task3Card.getByTestId("resume-task-btn");
    await expect(resumeBtn).toBeVisible();
    await resumeBtn.click();

    const applyBtn = task3Card.getByTestId("apply-task-btn");
    await expect(applyBtn).toBeVisible();
    await applyBtn.click();

    await expect(page.getByTestId("status-card")).toContainText("Applied");

    // 5. Reload and confirm we land on the same task without losing history
    await page.reload();
    await expect(page.getByTestId("app-shell")).toBeVisible();
    await expect(page.getByTestId("status-card")).toContainText("Applied");
    await expect(page.getByText("แก้ไขและรันทดสอบ governor").first()).toBeVisible();
  } finally {
    if (closeServer) await closeServer();
  }
});
