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


  await page.getByTestId("new-conversation-btn").click();


  await page.getByTestId("composer-input").fill("/review ตรวจสอบไฟล์ autonomy.go");
  await page.getByTestId("composer-send-btn").click();
  await expect(page.getByText("ตรวจสอบไฟล์ autonomy.go").first()).toBeVisible();


  await page.getByTestId("composer-input").fill("/preview เตรียมแก้ limit ของ governor");
  await page.getByTestId("composer-send-btn").click();
  await expect(page.getByTestId("changes-card")).toContainText("internal/store/autonomy.go");
  await page.getByTestId("changes-file-btn").first().click();
  await expect(page.getByTestId("diff-view")).toBeVisible();
  await expect(page.getByTestId("diff-lines")).toContainText("defaultLimit");
  await page.getByTestId("diff-back-btn").click();


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


  await page.reload();
  await expect(page.getByTestId("app-shell")).toBeVisible();
  await expect(page.getByTestId("status-card")).toContainText("Applied");
  await expect(page.getByText("แก้ไขและรันทดสอบ governor").first()).toBeVisible();

  await page.getByTestId("new-conversation-btn").click();
  await expect(page.getByTestId("task-card")).toHaveCount(0);
  await expect(page.getByTestId("conversation-turn")).toHaveCount(0);

  await expect(page.getByTestId("session-item")).toHaveCount(1);
  await page.getByTestId("delete-session-btn").first().click({ force: true });
  await expect(page.getByTestId("session-item")).toHaveCount(0);

  expect(problems, problems.join("\n")).toEqual([]);
});

test("Desktop native shell behaviors: dark theme, custom context menu, blocked browser shortcuts (including Thai layout), drag-drop prevention, and external link interception", async ({
  page
}) => {
  await page.addInitScript(() => {
    const w = window as unknown as Record<string, unknown>;
    w.__openedExternalURLs = [] as string[];
    w.__revealedPaths = [] as string[];
    w.__savedThemes = [] as string[];
    w.runtime = {
      BrowserOpenURL: (url: string) => {
        (w.__openedExternalURLs as string[]).push(url);
      }
    };
    w.go = {
      main: {
        App: {
          ChooseWorkspace: async () => {},
          OpenWorkspace: async () => {},
          SetTheme: async (mode: string) => {
            (w.__savedThemes as string[]).push(mode);
          },
          GetShellState: async () => ({
            current_workspace: "D:/APW_Project/apw-agent",
            recent: ["D:/APW_Project/apw-agent"],
            theme: "dark",
            ui: {}
          }),
          SaveUIPreferences: async () => {},
          RevealInExplorer: async (target: string) => {
            (w.__revealedPaths as string[]).push(target);
          }
        }
      }
    };
  });

  await page.goto("/");
  await expect(page.getByTestId("app-shell")).toBeVisible();

  // Check #3: dark theme loaded from GetShellState and toggled via Settings, plus Messaging channel settings
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.keyboard.press("Control+,");
  await expect(page.getByTestId("theme-light-btn")).toBeVisible();
  await page.getByTestId("theme-light-btn").click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await page.getByTestId("settings-tab-messaging").click();
  await page.getByTestId("telegram-enabled-checkbox").check();
  await page.getByTestId("telegram-token-input").fill("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ");
  await page.getByTestId("telegram-allow-input").fill("111222333, 444555666");
  await page.getByTestId("save-settings-btn").click();
  await expect(page.getByTestId("settings-saved")).toBeVisible();
  await expect(page.getByTestId("telegram-token-status")).toContainText("1234...xYZ");
  await page.getByTestId("clear-telegram-token-btn").click();
  await expect(page.getByTestId("telegram-token-status")).toHaveCount(0);

  await page.keyboard.press("Escape");

  // Submit review & preview so we have a markdown link and a [data-file-path] element
  await page.getByTestId("composer-input").fill("/review ตรวจสอบลิงก์ภายนอก");
  await page.getByTestId("composer-send-btn").click();
  await page.getByTestId("composer-input").fill("/preview เตรียมแก้ไฟล์");
  await page.getByTestId("composer-send-btn").click();
  await expect(page.getByTestId("changes-file-btn").first()).toBeVisible();

  // Check #4: Right-click on file item opens app context menu and prevents browser menu
  const ctxPrevented = await page.getByTestId("changes-file-btn").first().evaluate((el) => {
    const ev = new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 200, clientY: 200 });
    el.dispatchEvent(ev);
    return ev.defaultPrevented;
  });
  expect(ctxPrevented).toBe(true);
  await expect(page.getByTestId("app-context-menu")).toBeVisible();
  await page.getByTestId("app-context-menu").getByText("เปิดใน Explorer").click();
  const revealed = await page.evaluate(() => (window as unknown as { __revealedPaths: string[] }).__revealedPaths);
  expect(revealed).toContain("internal/store/autonomy.go");

  // Check #5: Browser shortcuts blocked (including Thai Kedmanee layout physical key codes)
  const shortcutResults = await page.evaluate(() => {
    const cases = [
      { key: "r", code: "KeyR", ctrlKey: true, shiftKey: false },
      { key: "พ", code: "KeyR", ctrlKey: true, shiftKey: false },
      { key: "F5", code: "F5", ctrlKey: false, shiftKey: false },
      { key: "p", code: "KeyP", ctrlKey: true, shiftKey: false },
      { key: "ย", code: "KeyP", ctrlKey: true, shiftKey: false },
      { key: "F12", code: "F12", ctrlKey: false, shiftKey: false },
      { key: "I", code: "KeyI", ctrlKey: true, shiftKey: true },
      { key: "=", code: "Equal", ctrlKey: true, shiftKey: false },
      { key: "-", code: "Minus", ctrlKey: true, shiftKey: false },
      { key: "0", code: "Digit0", ctrlKey: true, shiftKey: false }
    ];
    return cases.map((c) => {
      const ev = new KeyboardEvent("keydown", { ...c, bubbles: true, cancelable: true });
      window.dispatchEvent(ev);
      return ev.defaultPrevented;
    });
  });
  expect(shortcutResults.every(Boolean)).toBe(true);

  // Check #6: Dragover and drop on window are prevented
  const dropPrevented = await page.evaluate(() => {
    const over = new Event("dragover", { bubbles: true, cancelable: true });
    const drop = new Event("drop", { bubbles: true, cancelable: true });
    window.dispatchEvent(over);
    window.dispatchEvent(drop);
    return over.defaultPrevented && drop.defaultPrevented;
  });
  expect(dropPrevented).toBe(true);

  // Check #7: Clicking external link in review message calls BrowserOpenURL without navigating
  await page.getByRole("button", { name: "go docs" }).first().click();
  const opened = await page.evaluate(() => (window as unknown as { __openedExternalURLs: string[] }).__openedExternalURLs);
  expect(opened).toContain("https://pkg.go.dev/context");
});
