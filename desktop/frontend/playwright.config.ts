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
// e2e/editor.spec.ts in WebKit (the macOS app's engine), on its own harness.
const webkitEditorPort = Number(process.env.E2E_EDITOR_WEBKIT_PORT) || 34119;
const webkitEditorURL = `http://127.0.0.1:${webkitEditorPort}`;
/** A harness over a fresh copy of a test project (tests never change the
 * repo); the copy and the harness's data go when the server stops. */
const shopHarness = (port: number, project = "shop-api") =>
  `sh -c 't=$(mktemp -d); trap "rm -rf \\"$t\\"" EXIT INT TERM; ` +
  `mkdir "$t/${project}" && cp -R ../testdata/${project}/. "$t/${project}" && ${harness} --root "$t/${project}" --data "$t/data" --port ${port}'`;
// e2e/results.spec.ts: the Results panel on testdata/results-api.
const resultsPort = Number(process.env.E2E_RESULTS_PORT) || 34122;
const resultsURL = `http://127.0.0.1:${resultsPort}`;
// e2e/form.spec.ts: the Form view on testdata/form-api.
const formPort = Number(process.env.E2E_FORM_PORT) || 34124;
const formURL = `http://127.0.0.1:${formPort}`;
// e2e/panels.spec.ts: the side panels on a copy of results-api made a git
// repository, with a SONDE_VARIABLE_ in the app's environment.
const panelsPort = Number(process.env.E2E_PANELS_PORT) || 34126;
const panelsURL = `http://127.0.0.1:${panelsPort}`;
const gitEnv = "GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_AUTHOR_NAME=e2e GIT_AUTHOR_EMAIL=e2e@sonde.test GIT_COMMITTER_NAME=e2e GIT_COMMITTER_EMAIL=e2e@sonde.test";
/** A visual harness: a copy of project at a fixed path named name (paths
 * and the project name show in the screens, so they must not change from
 * run to run), made a git repository when git is set. */
const visualHarness = (port: number, project: string, name: string, git = false) =>
  `sh -c 't=/tmp/sonde-visual-${port}; rm -rf "$t"; trap "rm -rf \\"$t\\"" EXIT INT TERM; ` +
  `mkdir -p "$t/${name}" && cp -R ../testdata/${project}/. "$t/${name}" && ` +
  (git ? `(cd "$t/${name}" && git init -q -b main && ${gitEnv} git add -A && ${gitEnv} git commit -qm init) && ` : "") +
  `${gitEnv} ${harness} --root "$t/${name}" --data "$t/data" --port ${port}'`;
const panelsHarness =
  `sh -c 't=$(mktemp -d); trap "rm -rf \\"$t\\"" EXIT INT TERM; ` +
  `mkdir "$t/p" && cp -R ../testdata/results-api/. "$t/p" && (cd "$t/p" && git init -q -b main && ${gitEnv} git add -A && ${gitEnv} git commit -qm init) && ` +
  `${gitEnv} SONDE_VARIABLE_region=eu ${harness} --root "$t/p" --data "$t/data" --port ${panelsPort}'`;
// e2e/import.spec.ts: Import and Copy as on a copy of shop-api, with a
// SONDE_VARIABLE_ in the app's environment.
const importPort = Number(process.env.E2E_IMPORT_PORT) || 34128;
const importURL = `http://127.0.0.1:${importPort}`;
const importHarness =
  `sh -c 't=$(mktemp -d); trap "rm -rf \\"$t\\"" EXIT INT TERM; ` +
  `mkdir "$t/p" && cp -R ../testdata/shop-api/. "$t/p" && SONDE_VARIABLE_region=eu ${harness} --root "$t/p" --data "$t/data" --port ${importPort}'`;
// e2e/full-flow.spec.ts: the whole journey on a copy of results-api.
const fullPort = Number(process.env.E2E_FULL_PORT) || 34132;
const fullURL = `http://127.0.0.1:${fullPort}`;
// e2e/visual-*.spec.ts: the design screens in WebKit, each on its own
// harness, only with E2E_VISUAL=1 (their baselines are approved screens).
const visual = !!process.env.E2E_VISUAL;
const visualPorts = { shop: 34140, results: 34141, form: 34142, themes: 34143 };
const visualURL = (k: keyof typeof visualPorts) => `http://127.0.0.1:${visualPorts[k]}`;
// The preview's sandbox and framing in WebKit (the macOS app's engine).
const resultsWebkitPort = Number(process.env.E2E_RESULTS_WEBKIT_PORT) || 34123;
const resultsWebkitURL = `http://127.0.0.1:${resultsWebkitPort}`;
// e2e/perf.spec.ts: a generated project of 1000 request files in 20
// folders, and big.hurl (5,000 lines).
const perfPort = Number(process.env.E2E_PERF_PORT) || 34117;
const perfURL = `http://127.0.0.1:${perfPort}`;
const bigProject =
  `d=$(mktemp -d) && for f in $(seq -w 1 20); do mkdir "$d/folder$f"; ` +
  `for i in $(seq -w 1 50); do printf "GET {{base_url}}/items/%s/%s\\nHTTP 204\\n" $f $i > "$d/folder$f/request$i.hurl"; done; done && ` +
  // big.hurl: 5,000 lines for typing in the editor.
  `for i in $(seq 1 385); do printf "# Order %s\\nPOST {{base_url}}/orders?page=%s\\nAuthorization: Bearer {{token}}\\n{\\n  \\"sku\\": \\"TEA-%s\\",\\n  \\"quantity\\": {{quantity}}\\n}\\nHTTP 201\\n[Captures]\\norder_id: jsonpath \\"\\$.id\\"\\n[Asserts]\\njsonpath \\"\\$.status\\" == \\"created\\"\\n\\n" $i $i $i; done > "$d/big.hurl"`;

export default defineConfig({
  testDir: "e2e",
  snapshotPathTemplate: "{testDir}/__screenshots__/{platform}/{arg}{ext}",
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "list" : "line",
  use: { baseURL },
  projects: [
    { name: "chromium", testIgnore: /server-mode|shell|editor|perf|results|form\.spec|panels|import|full-flow|visual/, use: { ...devices["Desktop Chrome"] } },
    { name: "webkit", testIgnore: /server-mode|shell|editor|perf|results|form\.spec|panels|import|full-flow|visual/, use: { ...devices["Desktop Safari"] } },
    // Timings are measured alone, after the other browser tests (the load
    // of seven test servers and browsers would skew them).
    {
      name: "perf",
      testMatch: /perf/,
      workers: 1,
      dependencies: ["chromium", "webkit", "shell", "results", "results-webkit", "form", "panels", "import", "full", "editor-webkit"],
      use: { ...devices["Desktop Chrome"], baseURL: perfURL },
    },
    // The shell's tests share one harness (its settings, its runs): one
    // browser, one worker, repeats included.
    { name: "shell", testMatch: /shell|editor/, fullyParallel: false, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: shellURL } },
    // The Results panel's tests share one harness (the mock they start).
    { name: "results", testMatch: /results/, testIgnore: /visual-/, fullyParallel: false, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: resultsURL } },
    // The Form view's tests share one harness (they edit its files).
    { name: "form", testMatch: /form\.spec/, testIgnore: /visual-/, fullyParallel: false, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: formURL } },
    // The panels' tests share one harness (they change its files and settings).
    { name: "panels", testMatch: /panels/, fullyParallel: false, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: panelsURL } },
    ...(visual
      ? (["shop", "results", "form", "themes"] as const).map((k) => ({
          name: `visual-${k}`,
          testMatch: new RegExp(`visual-${k}\\.spec`),
          fullyParallel: false,
          workers: 1,
          use: { ...devices["Desktop Safari"], baseURL: visualURL(k), colorScheme: "dark" as const },
        }))
      : []),
    // The full journey: one test, its own harness.
    { name: "full", testMatch: /full-flow/, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: fullURL } },
    // Import and Copy as share one harness (they write its files).
    { name: "import", testMatch: /import/, fullyParallel: false, workers: 1, use: { ...devices["Desktop Chrome"], baseURL: importURL } },
    { name: "results-webkit", testMatch: /results/, testIgnore: /visual-/, grep: /preview|Assert/, workers: 1, use: { ...devices["Desktop Safari"], baseURL: resultsWebkitURL } },
    { name: "editor-webkit", testMatch: /editor/, fullyParallel: false, workers: 1, use: { ...devices["Desktop Safari"], baseURL: webkitEditorURL } },
    // Server mode's sign-in (E2E_SERVER_BIN, a server build).
    { name: "server-chromium", testMatch: /server-mode/, use: { ...devices["Desktop Chrome"] } },
    { name: "server-webkit", testMatch: /server-mode/, use: { ...devices["Desktop Safari"] } },
  ],
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : [
        ...(visual
          ? [
              { command: visualHarness(visualPorts.shop, "shop-api", "shop-api"), url: `${visualURL("shop")}/health` },
              { command: visualHarness(visualPorts.results, "results-api", "shop-api", true), url: `${visualURL("results")}/health` },
              { command: visualHarness(visualPorts.form, "form-api", "shop-api"), url: `${visualURL("form")}/health` },
              { command: visualHarness(visualPorts.themes, "results-api", "shop-api"), url: `${visualURL("themes")}/health` },
            ].map((w) => ({ ...w, reuseExistingServer: false, gracefulShutdown: { signal: "SIGTERM" as const, timeout: 3000 }, timeout: 30_000 }))
          : []),
        {
          command: `sh -c 't=$(mktemp -d); trap "rm -rf \\"$t\\"" EXIT INT TERM; ${harness} --root e2e/fixture --data "$t" --port ${port}'`,
          url: `${baseURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: `${fixtureServer} --port 34120`,
          url: "http://127.0.0.1:34120/health",
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: shopHarness(shellPort),
          url: `${shellURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: shopHarness(resultsPort, "results-api"),
          url: `${resultsURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: shopHarness(resultsWebkitPort, "results-api"),
          url: `${resultsWebkitURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: shopHarness(formPort, "form-api"),
          url: `${formURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: panelsHarness,
          url: `${panelsURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: shopHarness(fullPort, "results-api"),
          url: `${fullURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: importHarness,
          url: `${importURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: shopHarness(webkitEditorPort),
          url: `${webkitEditorURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
        {
          command: `sh -c '${bigProject} && trap "rm -rf \\"$d\\"" EXIT INT TERM && ${harness} --root "$d" --data "$d/.data" --port ${perfPort}'`,
          url: `${perfURL}/health`,
          reuseExistingServer: !process.env.CI,
          // SIGTERM (not the default SIGKILL): the server's temp folder goes.
          gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
          timeout: 30_000,
        },
      ],
});
