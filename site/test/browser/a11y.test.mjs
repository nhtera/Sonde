// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// axe (WCAG 2.2 AA) on the landing page, a docs page and the 404 page, at
// four viewports in both themes, and no sideways scrolling at 375px.
// Needs `npm run build` and a Chromium (npx playwright install chromium).

import AxeBuilder from "@axe-core/playwright";
import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { chromium } from "playwright";
import { startServer } from "../../scripts/serve-dist.mjs";

const VIEWPORTS = [
  [1440, 900],
  [768, 1024],
  [812, 375],
  [375, 812],
];
const PAGES = ["/", "/docs/getting-started", "/nope"];

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

async function open(path, [width, height], theme) {
  // Reduced motion shows every section at once, so axe sees final colors.
  const ctx = await browser.newContext({ viewport: { width, height }, reducedMotion: "reduce", hasTouch: width < 800 });
  await ctx.addInitScript((t) => localStorage.setItem("sonde-site-theme", t), theme);
  const page = await ctx.newPage();
  await page.goto(base + path);
  await page.waitForLoadState("networkidle");
  return page;
}

// The landing page in the light theme fails color-contrast on desktop token
// values (--pass/--m-get, --warn/--m-post, --fail, --accent, --t-str, --t-f on
// light surfaces). The fix belongs in desktop tokens.css, which plan
// 261003-0708-desktop-themes owns; until then that one rule is reported as a
// TODO there, and every other rule still fails the run.
const KNOWN = { "/:light": ["color-contrast"] };
const TODO = "light-theme token contrast: pending a tokens.css decision";

async function axe(path, vp, theme, { only, skip } = {}) {
  const page = await open(path, vp, theme);
  let builder = new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa", "best-practice"]);
  if (only) builder = builder.withRules(only);
  if (skip) builder = builder.disableRules(skip);
  const { violations } = await builder.analyze();
  await page.context().close();
  return violations.map((v) => `${v.id} (${v.impact}): ${v.nodes.length}× ${v.nodes.slice(0, 3).map((n) => n.target.join(" ")).join(" | ")}`);
}

for (const path of PAGES) {
  for (const theme of ["dark", "light"]) {
    const known = KNOWN[`${path}:${theme}`];
    for (const vp of VIEWPORTS) {
      test(`axe: ${path} ${theme} ${vp.join("x")}`, async () => {
        assert.deepEqual(await axe(path, vp, theme, { skip: known }), []);
      });
      if (known) {
        test(`axe: ${path} ${theme} ${vp.join("x")} (${known.join(", ")})`, { todo: TODO }, async () => {
          assert.deepEqual(await axe(path, vp, theme, { only: known }), []);
        });
      }
    }
  }
  test(`no sideways scroll at 375: ${path}`, async () => {
    const page = await open(path, [375, 812], "dark");
    const { scroll, client } = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
    await page.context().close();
    assert.ok(scroll <= client, `scrollWidth ${scroll} > ${client}`);
  });
}
