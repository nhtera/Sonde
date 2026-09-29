// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Browser tests run against the test-only harness build (task
// build:harness); only e2e/server-mode.spec.ts starts the server build
// (E2E_SERVER_BIN) to test its sign-in. E2E_PORT picks the harness port
// (default 34115); E2E_BASE_URL points at an already running harness.

import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.E2E_PORT) || 34115;
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${port}`;
const harness = process.env.E2E_HARNESS_BIN ?? "../bin/sonde-desktop-harness";

export default defineConfig({
  testDir: "e2e",
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "list" : "line",
  use: { baseURL },
  projects: [
    { name: "chromium", testIgnore: /server-mode/, use: { ...devices["Desktop Chrome"] } },
    { name: "webkit", testIgnore: /server-mode/, use: { ...devices["Desktop Safari"] } },
    // Server mode's sign-in (E2E_SERVER_BIN, a server build).
    { name: "server-chromium", testMatch: /server-mode/, use: { ...devices["Desktop Chrome"] } },
    { name: "server-webkit", testMatch: /server-mode/, use: { ...devices["Desktop Safari"] } },
  ],
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: `${harness} --root e2e/fixture --port ${port}`,
        url: `${baseURL}/health`,
        reuseExistingServer: !process.env.CI,
        timeout: 30_000,
      },
});
