import { test, expect } from "@playwright/test";

// Runs against the real desktop.Server (embedded dist, CSP, token guard, approvals) served by
// `go run ./internal/desktop/e2ehelper` (see playwright.config.ts) with a deterministic backend
// built from the real Go types. Set APW_DESKTOP_URL to point at another running Desktop.

test("Desktop completion gate: create conversation -> review -> preview -> inspect diff -> execute/verify -> cancel/resume -> apply -> reload keeps task", async ({
  page,
  request
}) => {
  const problems: string[] = [];
  page.on("pageerror", (err) => problems.push("pageerror: " + err.message));
  page.on("console", (msg) => {
    if (msg.type() === "error") problems.push(`console: ${msg.text()} (${msg.location().url})`);
  });
  page.on("response", (res) => {
    if (res.status() >= 400) problems.push(`HTTP ${res.status()} ${res.request().method()} ${new URL(res.url()).pathname}`);
  });

  const index = await request.get("/");
  expect(index.status()).toBe(200);
  const csp = index.headers()["content-security-policy"] || "";
  expect(csp).toContain("script-src 'self'");
  expect(csp).not.toContain("'unsafe-eval'");
  expect(await index.text()).not.toContain("__APW_TOKEN__");

  await page.goto("/");
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

  // 4. Execute -> Cancel -> Resume -> Verify -> Apply (with the real approval gate)
  await page.getByTestId("composer-input").fill("แก้ไขและรันทดสอบ governor");
  await page.getByTestId("composer-send-btn").click();

  const task3Card = page.locator('[data-task-id="task-3"]');
  const cancelBtn = task3Card.getByTestId("cancel-task-btn");
  await expect(cancelBtn).toBeVisible();
  await cancelBtn.click();

  const resumeBtn = task3Card.getByTestId("resume-task-btn");
  await expect(resumeBtn).toBeVisible();
  await resumeBtn.click();

  // Verification evidence comes from the real report fields (acceptance criterion/evidence).
  await expect(page.getByTestId("verification-card")).toContainText("pause keeps configured limits");

  const applyBtn = task3Card.getByTestId("apply-task-btn");
  await expect(applyBtn).toBeVisible();
  await applyBtn.click();

  // desktop.Server asks before applying (approval "ask"): approve once.
  const approval = page.getByTestId("approval-card");
  await expect(approval).toBeVisible();
  await approval.getByTestId("approve-once-btn").click();

  await expect(page.getByTestId("status-card")).toContainText("Applied");

  // 5. Reload and confirm we land on the same task without losing history
  await page.reload();
  await expect(page.getByTestId("app-shell")).toBeVisible();
  await expect(page.getByTestId("status-card")).toContainText("Applied");
  await expect(page.getByText("แก้ไขและรันทดสอบ governor").first()).toBeVisible();

  expect(problems, problems.join("\n")).toEqual([]);
});
