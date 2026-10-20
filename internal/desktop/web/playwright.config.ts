import { defineConfig } from "@playwright/test";

const external = process.env.APW_DESKTOP_URL;
const port = Number(process.env.APW_E2E_PORT || 47911);

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  retries: 0,
  use: {
    baseURL: external || `http://127.0.0.1:${port}`,
    headless: true,
    channel: process.platform === "win32" && !process.env.CI ? "msedge" : undefined,
    viewport: { width: 1360, height: 860 }
  },
  // The real desktop.Server with a deterministic backend (internal/desktop/e2ehelper). It embeds
  // web/dist at compile time, so run `npm run build` first.
  webServer: external
    ? undefined
    : {
        command: `go run ./internal/desktop/e2ehelper -addr 127.0.0.1:${port}`,
        cwd: "../../..",
        url: `http://127.0.0.1:${port}/`,
        timeout: 180_000,
        reuseExistingServer: !process.env.CI
      }
});
