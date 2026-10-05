// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The landing page's interactive parts, on the built site: keyboard tabs
// (roving tabindex), the install command's copy button, the mobile menu,
// and the scroll reveal after client-side navigation.
// Needs `npm run build` and a Chromium (npx playwright install chromium).

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { after, before, test } from "node:test";
import { chromium } from "playwright";
import { startServer } from "../../scripts/serve-dist.mjs";

const install = JSON.parse(readFileSync(new URL("../../content/generated/install.json", import.meta.url), "utf8"));
let server, browser, base;
before(async () => {
  server = await startServer();
  base = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch();
});
after(async () => {
  await browser?.close();
  server?.close();
});

test("install tabs: arrows, Home and End move focus and the command; copy copies it exactly", async () => {
  const ctx = await browser.newContext({ permissions: ["clipboard-read", "clipboard-write"] });
  const page = await ctx.newPage();
  await page.goto(`${base}/`);
  const tabs = page.getByRole("tablist", { name: "Install with" }).getByRole("tab");
  await tabs.first().focus();
  await page.keyboard.press("ArrowRight");
  assert.equal(await page.evaluate(() => document.activeElement?.textContent), install[1].label);
  assert.equal(await page.locator("#install-cmd pre").textContent(), install[1].command);
  await page.keyboard.press("End");
  assert.equal(await tabs.last().getAttribute("aria-selected"), "true");
  await page.keyboard.press("Home");
  assert.equal(await tabs.first().getAttribute("tabindex"), "0");
  assert.equal(await tabs.last().getAttribute("tabindex"), "-1");
  await page.keyboard.press("ArrowLeft"); // wraps to the last tab
  assert.equal(await tabs.last().getAttribute("aria-selected"), "true");
  await page.getByRole("button", { name: "Copy" }).click();
  await page.getByRole("button", { name: "Copied" }).waitFor();
  assert.equal(await page.evaluate(() => navigator.clipboard.readText()), install.at(-1).command);
  await ctx.close();
});

test("desktop showcase: arrow keys switch the visible panel", async () => {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  await page.goto(`${base}/`);
  const tabs = page.getByRole("tablist", { name: "Desktop screens" }).getByRole("tab");
  assert.equal(await page.getByRole("tablist", { name: "Desktop screens" }).getAttribute("aria-orientation"), "vertical");
  await tabs.first().focus();
  await page.keyboard.press("ArrowDown");
  assert.equal(await tabs.nth(1).getAttribute("aria-selected"), "true");
  assert.equal(await page.locator("#p-run").isHidden(), true);
  assert.equal(await page.locator("#p-palette").isVisible(), true);
  // Under 980px the tabs are a row.
  await page.setViewportSize({ width: 768, height: 900 });
  await page.waitForFunction(() => document.querySelector('[aria-label="Desktop screens"]')?.getAttribute("aria-orientation") === "horizontal", null, { timeout: 5000 });
  await page.close();
});

test("mobile menu: opens, focuses its first link, closes on Esc back to its button", async () => {
  const page = await browser.newPage({ viewport: { width: 375, height: 812 } });
  await page.goto(`${base}/`);
  const button = page.getByRole("button", { name: "Open menu" });
  await button.click();
  assert.equal(await page.locator("#mnav").isVisible(), true);
  assert.equal(await page.evaluate(() => document.activeElement?.textContent), "Docs");
  await page.keyboard.press("Escape");
  assert.equal(await page.locator("#mnav").isHidden(), true);
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute("aria-label")), "Open menu");
  await page.close();
});

test("sections reveal after client-side navigation to the landing page", async () => {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  await page.goto(`${base}/docs`);
  await page.waitForLoadState("networkidle");
  await page.locator('#nd-sidebar a[href="/"]').first().click();
  await page.waitForURL(`${base}/`);
  for (const id of ["#three", "#features", "#start"]) {
    await page.locator(id).scrollIntoViewIfNeeded();
    await page.waitForTimeout(800);
    const hidden = await page.evaluate(() =>
      [...document.querySelectorAll(".reveal")].filter((e) => {
        const r = e.getBoundingClientRect();
        return getComputedStyle(e).opacity === "0" && r.top < innerHeight && r.bottom > 0;
      }).length,
    );
    assert.equal(hidden, 0, `hidden sections in view at ${id}`);
  }
  await page.close();
});
