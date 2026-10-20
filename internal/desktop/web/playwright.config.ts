import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  retries: 0,
  use: {
    headless: true,
    channel: process.platform === "win32" && !process.env.CI ? "msedge" : undefined,
    viewport: { width: 1360, height: 860 }
  }
});
