// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The website's screenshots (make site-screens): the design's shop-api story,
// reproduced with real runs against the fixture API on testdata/showcase, in
// WebKit at 1440x900 @2x, dark then light. Each frame is written to
// marketing-shots/<id>-<theme>.png (not committed), with no baseline compare;
// site/scripts/optimize-screens.mjs turns them into the site's images.
//
// Run with E2E_MARKETING=1 (see the config: its servers are the only ones).
// Every frame's text is checked: no host path, no --variable, no other
// tool's name, no blank where a time shows (the times are pinned, see
// marketing/pin.ts).
//
// Names of other tools in the app's own text, by screen. The ones that stay
// name the file format; the screens that would show the others are not
// captured (the welcome screen, the import dialog and its result).
//
// | id       | in the frame                                    | decision |
// |----------|-------------------------------------------------|----------|
// | hero     | toolbar chip ".hurl · Hurl 8", status "Hurl 8 syntax" | stay: the file format |
// | run      | status "Hurl 8 syntax"                          | stays    |
// | palette  | status "Hurl 8 syntax"                          | stays    |
// | form     | toolbar chip ".hurl · Hurl 8", "Hurl 8 syntax"  | stay     |
// | streams  | chip ".sonde · extensions"                      | none     |
// | agents   | status "Hurl 8 syntax"                          | stays    |
// | env      | chip and status as hero                         | stay     |
// | coverage | chip and status as hero                         | stay     |

import { expect, test, type Locator, type Page } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
import { dark, home, light, open, panel, run } from "./visual/snap";
import { pinVolatile } from "./marketing/pin";

test.skip(!process.env.E2E_MARKETING, "the website's screenshots: make site-screens");
test.describe.configure({ mode: "serial" });

const out = "marketing-shots";
const results = (page: Page) => page.getByRole("region", { name: "Results" });
const tab = (page: Page, name: string) => results(page).getByRole("tab", { name: new RegExp(`^${name}`) });
const crops: Record<string, { x: number; y: number; w: number; h: number }> = {};

/** What must not show in a frame. */
const forbidden: [string, RegExp][] = [
  ["another tool's name", /Postman|Bruno|Insomnia/],
  ["--variable", /--variable/],
  ["a host path", /\/Users\/|\/home\/|\/var\/folders|\/tmp\//],
];

test.beforeAll(async () => {
  await mkdir(out, { recursive: true });
});

test.afterAll(async () => {
  await writeFile(`${out}/crops.json`, JSON.stringify(crops, null, 2) + "\n");
});

test.beforeEach(async ({ page }) => {
  await home(page);
  await dark(page);
  // A trusted folder shows no question.
  const trust = page.getByRole("button", { name: "Trust", exact: true });
  if (await trust.isVisible()) await trust.click();
  await expect(page.getByRole("note")).toHaveCount(0);
});

/** The frame's text holds none of what must not show, and no time is blank. */
async function checkText(page: Page, id: string) {
  const text = await page.evaluate(() => document.body.innerText);
  for (const [what, re] of forbidden) expect(text, `${id}: ${what}`).not.toMatch(re);
  const blank = await page.evaluate(() =>
    [...document.querySelectorAll("[data-volatile]")].filter((el) => !el.matches(".req-row .ms, td") && !el.textContent?.trim()).length,
  );
  expect(blank, `${id}: blank volatile slots`).toBe(0);
}

/**
 * Saves the frame in both themes. stage() puts the screen in its state
 * (idempotent: it runs once per theme, as a theme change closes an overlay);
 * reset() takes it back.
 */
async function capture(page: Page, id: string, stage: () => Promise<void> = async () => {}, reset: () => Promise<void> = async () => {}) {
  for (const theme of ["dark", "light"] as const) {
    await (theme === "dark" ? dark : light)(page);
    await stage();
    await page.evaluate(() => document.fonts.ready);
    await pinVolatile(page);
    await page.waitForTimeout(150);
    await pinVolatile(page);
    await checkText(page, `${id}-${theme}`);
    await page.screenshot({ path: `${out}/${id}-${theme}.png` });
    await reset();
  }
  await dark(page);
}

/** The box of locators together, padded, in CSS px of the frame. */
async function boxOf(id: string, parts: Locator[], pad = 20) {
  const boxes = await Promise.all(parts.map((l) => l.boundingBox()));
  const b = boxes.filter((x) => x !== null);
  expect(b.length, `${id}: crop elements`).toBe(parts.length);
  const x1 = Math.max(0, Math.min(...b.map((r) => r!.x)) - pad);
  const y1 = Math.max(0, Math.min(...b.map((r) => r!.y)) - pad);
  const x2 = Math.min(1440, Math.max(...b.map((r) => r!.x + r!.width)) + pad);
  const y2 = Math.min(900, Math.max(...b.map((r) => r!.y + r!.height)) + pad);
  crops[id] = { x: Math.round(x1), y: Math.round(y1), w: Math.round(x2 - x1), h: Math.round(y2 - y1) };
}

/** The tabs of the story: login, checkout (shown), events. */
async function openTabs(page: Page) {
  await open(page, "login.hurl");
  await open(page, "events.sonde");
  await open(page, "checkout.hurl");
}

test("hero: the file after a run, a request failed", async ({ page }) => {
  await openTabs(page);
  await run(page, /Failed/);
  await results(page).locator(".req-row").nth(4).click();
  await tab(page, "Asserts").click();
  await expect(results(page)).toContainText("Assert failed");
  await capture(page, "hero");
});

test("run: a test run of every file", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await panel(page, "Test run");
  await page.getByRole("button", { name: /^Run \d+ files?$/ }).click();
  await expect(page.getByLabel("Test summary")).toContainText("Executed files:", { timeout: 30_000 });
  await capture(page, "run");
});

test("palette: files and commands under one shortcut", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await capture(
    page,
    "palette",
    async () => {
      await page.keyboard.press("ControlOrMeta+KeyK");
      await page.keyboard.type("che");
      await expect(page.getByRole("dialog").getByRole("option").first()).toBeVisible();
    },
    () => page.keyboard.press("Escape"),
  );
});

test("form: the asserts as rows", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await page.getByRole("group", { name: "Editor view" }).getByRole("button", { name: "Form" }).click();
  await page.getByRole("tablist", { name: "Requests" }).getByRole("tab").nth(4).click();
  await page.getByRole("tablist", { name: "Request parts" }).getByRole("tab", { name: /^Asserts/ }).click();
  await capture(page, "form");
});

test("streams: server-sent events, closed at the count", async ({ page }) => {
  await open(page, "events.sonde");
  await run(page, /Passed/);
  await expect(results(page).locator(".stream-stop")).toBeVisible();
  await capture(page, "streams");
});

test("agents: the AI agents panel", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await panel(page, "AI agents");
  await page.getByRole("switch", { name: "Allow the agent to send requests" }).click();
  await expect(page.getByLabel("Agent configuration")).toContainText("--allow-run");
  await capture(page, "agents");
});

test("env: variables listed while typing {{", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  const header = () => page.locator(".cm-line", { hasText: "Authorization: Bearer {{token}}" }).last();
  await capture(
    page,
    "env",
    async () => {
      await header().click();
      await page.keyboard.press("End");
      await page.keyboard.press("Enter");
      await page.keyboard.type("X-User: {{");
      await expect(page.locator(".cm-tooltip-autocomplete.cm-var-complete")).toBeVisible();
      await boxOf("env", [page.locator(".cm-tooltip-autocomplete.cm-var-complete")], 0);
    },
    async () => {
      await page.keyboard.press("Escape");
      await page.keyboard.press("ControlOrMeta+KeyZ");
    },
  );
  await expect(page.locator(".cm-line", { hasText: "X-User" })).toHaveCount(0);
});

test("coverage: operations the runs called", async ({ page }) => {
  await panel(page, "Contract & mock");
  await page.getByRole("switch", { name: "Check responses against the OpenAPI spec" }).setChecked(true);
  for (const file of ["health.hurl", "login.hurl", "events.sonde", "create-user.hurl"]) {
    await panel(page, "Files");
    await open(page, file);
    await run(page, /Passed/);
  }
  await panel(page, "Files");
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await results(page).locator(".req-row").nth(4).click();
  await tab(page, "Asserts").click();
  await panel(page, "Contract & mock");
  await expect(page.getByLabel(/\d+ of \d+ operations covered/)).toBeVisible();
  await boxOf("coverage", [page.locator(".panel-section", { has: page.locator(".coverage-bar") })], 6);
  await capture(page, "coverage");
});

test("og: the share image", async ({ page }) => {
  const here = process.cwd();
  const file = (p: string) => `file://${here}/${p}`;
  const logo = file("../../editors/vscode/images/icon.svg");
  const html = `<!doctype html><html data-theme="dark"><head><meta charset="utf-8">
<link rel="stylesheet" href="${file("node_modules/@fontsource/geist/500.css")}">
<link rel="stylesheet" href="${file("node_modules/@fontsource/geist/600.css")}">
<link rel="stylesheet" href="${file("src/app/theme/tokens.css")}">
<style>
  html, body { margin: 0; width: 1200px; height: 630px; overflow: hidden; background: var(--bg); color: var(--text); font-family: "Geist", system-ui, sans-serif; }
  .brand { position: absolute; left: 72px; top: 64px; display: flex; align-items: center; gap: 16px; font-size: 32px; font-weight: 600; }
  .brand img { width: 56px; height: 56px; }
  h1 { position: absolute; left: 72px; top: 160px; width: 520px; margin: 0; font-size: 58px; line-height: 1.08; font-weight: 600; letter-spacing: -0.02em; }
  .shot { position: absolute; left: 678px; top: 96px; width: 450px; height: 438px; border-radius: 14px; border: 1px solid var(--accent-line); overflow: hidden; }
  .shot img { position: absolute; left: -990px; top: -70px; width: 1440px; height: 900px; }
</style></head><body>
<div class="brand"><img src="${logo}" alt="">Sonde</div>
<h1>Plain-text HTTP tests for humans, CI and AI agents.</h1>
<div class="shot"><img src="${file(`${out}/hero-dark.png`)}" alt=""></div>
</body></html>`;
  await writeFile(`${out}/og.html`, html);
  await page.setViewportSize({ width: 1200, height: 630 });
  await page.goto(file(`${out}/og.html`));
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(() => Promise.all([...document.images].map((i) => i.decode())));
  // WebKit paints a large image a moment after it decodes.
  await page.waitForTimeout(500);
  await page.screenshot({ path: `${out}/og.png`, scale: "css" });
});
