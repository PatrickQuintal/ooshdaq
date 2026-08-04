import { defineConfig, devices } from "@playwright/test";

const baseURL = process.env.OOSHDAQ_E2E_BASE_URL || "http://127.0.0.1:18080";

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.spec.mjs",
  outputDir: new URL("../test-results", import.meta.url).pathname,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["line"]],
  timeout: 90_000,
  expect: {
    timeout: 10_000,
  },
  use: {
    baseURL,
    headless: true,
    locale: "en-AU",
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: "off",
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
      },
    },
  ],
  webServer: {
    command: "bash e2e/start-server.sh",
    url: `${baseURL}/health/ready`,
    cwd: new URL("..", import.meta.url).pathname,
    reuseExistingServer: false,
    timeout: 120_000,
    stdout: "pipe",
    stderr: "pipe",
  },
});
