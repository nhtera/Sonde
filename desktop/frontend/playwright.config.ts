// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Browser tests run against the test-only harness build (task
// build:harness); only e2e/server-mode.spec.ts starts the server build
// (E2E_SERVER_BIN) to test its sign-in. E2E_PORT picks the harness port
// (default 34115); E2E_BASE_URL points at an already running harness.
// e2e/shell.spec.ts runs on its own harness over testdata/shop-api
// (E2E_SHELL_PORT, default 34116) with the fixture API on 34120.

import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.E2E_PORT) || 34115;
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${port}`;
const harness = process.env.E2E_HARNESS_BIN ?? "../bin/sonde-desktop-harness";
const fixtureServer = process.env.E2E_FIXTURE_BIN ?? "../bin/fixture-server";
export const shellPort = Number(process.env.E2E_SHELL_PORT) || 34116;
const shellURL = process.env.E2E_SHELL_URL ?? `http://127.0.0.1:${shellPort}`;
// e2e/perf.spec.ts: a generated project of 1000 request files in 20 folders.
const perfPort = Number(process.env.E2E_PERF_PORT) || 34117;
const perfURL = `http://127.0.0.1:${perfPort}`;
const bigProject =
  `d=$(mktemp -d) && for f in $(seq -w 1 20); do mkdir "$d/folder$f"; ` +
  `for i in $(seq -w 1 50); do printf "GET {{base_url}}/items/%s/%s\\nHTTP 204\\n" $f $i > "$d/folder$f/request$i.hurl"; done; done`;

export default defineConfig({
  testDir: "e2e",
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "list" : "line",
  use: { baseURL },
  projects: [
    { name: "chromium", testIgnore: /server-mode|shell|perf/, use: { ...devices["Desktop Chrome"] } },
    { name: "webkit", testIgnore: /server-mode|shell|perf/, use: { ...devices["Desktop Safari"] } },
    { name: "perf", testMatch: /perf/, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: perfURL } },
    // The shell's tests share one harness (its settings, its runs): one
    // browser, one worker, repeats included.
    { name: "shell", testMatch: /shell/, fullyParallel: false, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: shellURL } },
    // Server mode's sign-in (E2E_SERVER_BIN, a server build).
    { name: "server-chromium", testMatch: /server-mode/, use: { ...devices["Desktop Chrome"] } },
    { name: "server-webkit", testMatch: /server-mode/, use: { ...devices["Desktop Safari"] } },
  ],
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : [
        {
          command: `${harness} --root e2e/fixture --port ${port}`,
          url: `${baseURL}/health`,
          reuseExistingServer: !process.env.CI,
          timeout: 30_000,
        },
        {
          command: `${fixtureServer} --port 34120`,
          url: "http://127.0.0.1:34120/health",
          reuseExistingServer: !process.env.CI,
          timeout: 30_000,
        },
        {
          // A copy of shop-api, so a test that writes never changes the repo.
          command: `sh -c 'd=$(mktemp -d)/shop-api && mkdir "$d" && cp -R ../testdata/shop-api/. "$d" && exec ${harness} --root "$d" --port ${shellPort}'`,
          url: `${shellURL}/health`,
          reuseExistingServer: !process.env.CI,
          timeout: 30_000,
        },
        {
          command: `sh -c '${bigProject} && exec ${harness} --root "$d" --port ${perfPort}'`,
          url: `${perfURL}/health`,
          reuseExistingServer: !process.env.CI,
          timeout: 30_000,
        },
      ],
});
