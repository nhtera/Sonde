// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Results panel on the results-api project (its own harness, the
// fixture API on 34120): the first failure, + Assert with every digit, the
// timeline's redaction, the error card and the mock, streams, the
// interactive session, a large body, the preview's sandbox, and bodies
// that never carry a secret. Screens for the design approval go to
// E2E_CANDIDATES (default e2e-candidates/).

import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";

/** The project's declared secret (read, never printed). */
const password = /^password=(.*)$/m.exec(readFileSync("../testdata/results-api/secrets/local.secrets", "utf8"))![1].trim();

const results = (page: Page) => page.getByRole("region", { name: "Results" });

/** Opens file and runs it; resolves once the run has an outcome. */
async function run(page: Page, file: string, outcome: RegExp = /Passed|Failed|Error/) {
  await page.goto("/");
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.locator(".cm-content")).toBeVisible();
  await expect(page.locator(".cm-run-run").first()).toBeVisible();
  await page.keyboard.press("ControlOrMeta+KeyR");
  await expect(results(page).locator(".run-header .outcome")).toHaveText(outcome, { timeout: 20_000 });
}

const row = (page: Page, n: number) => results(page).locator(".req-row").nth(n - 1);
const tab = (page: Page, name: string) => results(page).getByRole("tab", { name: new RegExp(`^${name}`) });

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test("opens on the first failure, with the contract's violation as a row", async ({ page }) => {
  await run(page, "users.hurl", /Failed/);
  await expect(row(page, 2)).toHaveAttribute("aria-selected", "true");
  await expect(tab(page, "Asserts")).toHaveAttribute("aria-selected", "true");
  const panel = results(page).getByRole("tabpanel");
  await expect(panel.getByRole("alert")).toContainText(`jsonpath "$.name" == "grace"`);
  await expect(panel.getByRole("alert")).toContainText(`"ada"`);
  await expect(panel.locator(".assert-rows li.spec")).toContainText(`property "email" is missing`);
  await expect(results(page).locator(".entry-meta")).toContainText("200 OK");
  await page.screenshot({ path: `${candidates}/results-asserts-1a.png` });
});

test("+ Assert on a 64-bit id writes every digit, and Undo takes it back", async ({ page }) => {
  await run(page, "users.hurl", /Failed/);
  await tab(page, "Body").click();
  const big = results(page).locator(".jt-row", { hasText: '"big"' });
  await expect(big).toContainText("12345678901234567890");
  await big.hover();
  await page.screenshot({ path: `${candidates}/results-body-3a.png` });
  await big.getByRole("button", { name: "+ Assert" }).click();
  const added = page.locator(".cm-line", { hasText: 'jsonpath "$.big" == 12345678901234567890' });
  await expect(added).toHaveCount(1);
  await page.getByRole("status").getByRole("button", { name: /Undo/ }).click();
  await expect(added).toHaveCount(0);
});

test("+ Assert in a file with CRLF line endings lands on its own line", async ({ page }) => {
  await run(page, "crlf.hurl", /Passed/);
  await tab(page, "Body").click();
  const echo = results(page).locator(".jt-row", { hasText: '"echo"' });
  await echo.hover();
  await echo.getByRole("button", { name: "+ Assert" }).click();
  // The echoed secret is masked: the assert checks it exists.
  await expect(page.locator(".cm-line", { hasText: 'jsonpath "$.echo" exists' })).toHaveCount(1);
  await expect(page.locator(".cm-line", { hasText: /^HTTP 200$/ })).toHaveCount(1);
  await expect(page.locator(".cm-line", { hasText: /^\[Asserts\]$/ })).toHaveCount(1);
});

test("the timeline and captures show secrets as ***", async ({ page }) => {
  await run(page, "users.hurl", /Failed/);
  await tab(page, "Timeline").click();
  const panel = results(page).getByRole("tabpanel");
  await expect(panel.getByLabel("Log")).toContainText("> Authorization: Bearer ***");
  await expect(panel.getByLabel("Timing")).toContainText("Total");
  await page.screenshot({ path: `${candidates}/results-timeline-3b.png` });
  await row(page, 1).click();
  await tab(page, "Captures").click();
  await expect(panel).toContainText("→ token***");
  await tab(page, "Cookies").click();
  await expect(panel).toContainText("Received · Set-Cookie 1");
  await page.screenshot({ path: `${candidates}/results-cookies-3c.png` });
  for (const t of ["Body", "Headers", "Asserts", "Captures", "Cookies", "Timeline", "Request"]) {
    await tab(page, t).click();
    await expect(results(page)).not.toContainText(password);
  }
});

test("a server-sent stream stops at its count", async ({ page }) => {
  await run(page, "events.sonde", /Passed/);
  await expect(tab(page, "Stream")).toHaveAttribute("aria-selected", "true");
  await expect(results(page).locator(".stream-row")).toHaveCount(6);
  await expect(results(page).locator(".stream-stop")).toContainText("Stopped: count 6 reached.");
  await page.screenshot({ path: `${candidates}/results-sse-11a.png` });
});

test("an interactive WebSocket session sends, echoes, and writes 2 lines", async ({ page }) => {
  await run(page, "live.sonde", /Passed/);
  await page.screenshot({ path: `${candidates}/results-ws-11b.png` });
  await results(page).getByRole("button", { name: "Open interactive session" }).click();
  await expect(results(page).locator(".session-head .outcome")).toHaveText("Open");
  await results(page).getByLabel("Message", { exact: true }).fill('{"type": "ping"}');
  await results(page).getByRole("checkbox", { name: "Also write to file" }).check();
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(results(page).locator(".stream-row")).toHaveCount(2);
  await expect(results(page).locator(".stream-row.dir-received")).toContainText('{"type": "ping"}');
  await expect(page.locator(".cm-line", { hasText: 'send: {"type": "ping"}' })).toHaveCount(1);
  await expect(page.locator(".cm-line", { hasText: "receive: 1" })).toHaveCount(1);
  await page.screenshot({ path: `${candidates}/results-session-11b2.png` });
  await page.keyboard.press("Escape");
  await expect(results(page).locator(".session-head")).toHaveCount(0);
});

test("a 48 MB body shows its first megabyte, then formats off the page's thread", async ({ page }) => {
  await run(page, "big.hurl", /Passed/);
  await expect(results(page).getByLabel("Large response")).toContainText("48.0 MB response");
  await expect(results(page).getByRole("button", { name: "Pretty" })).toBeDisabled();
  await page.screenshot({ path: `${candidates}/results-large-10b.png` });
  // Long tasks of the page's thread while the tree is built and drawn.
  await page.evaluate(() => {
    const w = window as unknown as { longTasks: number[] };
    w.longTasks = [];
    new PerformanceObserver((l) => l.getEntries().forEach((e) => w.longTasks.push(e.duration))).observe({ type: "longtask" });
  });
  const t0 = Date.now();
  await results(page).getByRole("button", { name: "Format anyway" }).click();
  await expect(results(page).locator(".jt-row").first()).toBeVisible({ timeout: 10_000 });
  const ms = Date.now() - t0;
  // The orders are folded (too many rows otherwise): unfold the first.
  await results(page).locator(".jt-row").nth(1).getByRole("button", { name: "Unfold" }).click();
  await expect(results(page).locator(".jt-row", { hasText: "12345678901234567890" }).first()).toBeVisible();
  const longest = await page.evaluate(() => Math.max(0, ...(window as unknown as { longTasks: number[] }).longTasks));
  console.log(`48 MB tree: ${ms} ms to the first rows, longest main-thread task ${Math.round(longest)} ms`);
  expect(ms).toBeLessThan(3000);
  expect(longest).toBeLessThan(50);
});

test("the HTML preview runs no script and loads nothing", async ({ page }) => {
  // Loads that reached the network (a load the CSP blocks is reported as
  // a request that failed with "csp", never sent).
  const loads: string[] = [];
  const blocked: string[] = [];
  page.on("requestfinished", (r) => r.url().includes("from=preview") && loads.push(r.url()));
  page.on("requestfailed", (r) => r.url().includes("from=preview") && blocked.push(r.failure()?.errorText ?? ""));
  await run(page, "page.hurl", /Passed/);
  await row(page, 1).click();
  await tab(page, "Body").click();
  await results(page).getByRole("button", { name: "Preview" }).click();
  const frame = page.frameLocator("iframe[title='Response preview']");
  await expect(frame.locator("h1")).toHaveText("Receipt");
  await page.waitForTimeout(300);
  await expect(frame.locator("h1")).toHaveText("Receipt");
  expect(loads).toEqual([]);
  expect(blocked.every((e) => e === "csp")).toBe(true);
  await page.screenshot({ path: `${candidates}/results-preview-10a.png` });
});

test("binary and gzip bodies never carry the secret they echo", async ({ page }) => {
  const bodies: Buffer[] = [];
  page.on("response", async (r) => {
    if (r.url().includes("/_sonde/body/")) bodies.push(await r.body().catch(() => Buffer.alloc(0)));
  });
  await run(page, "echoes.hurl", /Passed/);
  for (const n of [1, 2]) {
    await row(page, n).click();
    await tab(page, "Body").click();
    await results(page).getByRole("button", { name: "Raw" }).click();
    await expect(results(page).getByLabel("Raw body")).toBeVisible();
  }
  await expect.poll(() => bodies.length).toBeGreaterThanOrEqual(2);
  for (const b of bodies) expect(b.includes(password)).toBe(false);
  expect(bodies.some((b) => b.includes("***"))).toBe(true);
});

// Last: the mock it starts stays on for the harness.
test("a refused connection offers the mock, which then answers", async ({ page }) => {
  await run(page, "health.hurl", /Failed/);
  await expect(tab(page, "Error")).toHaveAttribute("aria-selected", "true");
  await expect(results(page).getByRole("alert")).toContainText("Connection refused");
  await expect(results(page).locator(".entry-meta")).toContainText("No response");
  await page.screenshot({ path: `${candidates}/results-refused-10c.png` });
  await results(page).getByRole("button", { name: /Start mock on/ }).click();
  await expect(results(page).locator(".run-header .outcome")).toHaveText("Passed", { timeout: 20_000 });
  await expect(row(page, 1)).toContainText("204");
});
