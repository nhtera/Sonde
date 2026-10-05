// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The cascade, checked in a real browser on the built site: the desktop
// tokens (layer base) lose to Tailwind utilities and the site's components,
// and Fumadocs surfaces use token colors in both themes.
// Needs `npm run build` and a Chromium (npx playwright install chromium).

import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { chromium } from "playwright";
import { startServer } from "../../scripts/serve-dist.mjs";

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

async function page(theme) {
  const ctx = await browser.newContext();
  if (theme) await ctx.addInitScript((t) => localStorage.setItem("sonde-site-theme", t), theme);
  return ctx.newPage();
}

const token = (p, name) => p.evaluate((n) => {
  const probe = document.createElement("div");
  probe.style.color = `var(${n})`;
  document.body.append(probe);
  const c = getComputedStyle(probe).color;
  probe.remove();
  return c;
}, name);

test("dark is the default, with no class or attribute flash", async () => {
  const p = await page();
  await p.goto(`${base}/docs`);
  const html = await p.evaluate(() => [document.documentElement.classList.contains("dark"), document.documentElement.classList.contains("light"), document.documentElement.dataset.theme]);
  assert.deepEqual(html, [true, false, "dark"]);
  await p.context().close();
});

test("utilities beat the tokens.css globals", async () => {
  const p = await page();
  await p.goto(`${base}/docs`);
  const probe = await p.evaluate(() => {
    const b = document.createElement("button");
    b.className = "text-sm text-fd-muted-foreground";
    b.textContent = "probe";
    document.body.append(b);
    const s = getComputedStyle(b);
    return { size: s.fontSize, color: s.color };
  });
  assert.equal(probe.size, "14px");
  assert.equal(probe.color, await token(p, "--muted"));
  await p.context().close();
});

for (const theme of ["dark", "light"]) {
  test(`${theme}: docs surfaces use token colors`, async () => {
    const p = await page(theme);
    await p.goto(`${base}/docs/getting-started`);
    assert.equal(await p.evaluate(() => document.documentElement.dataset.theme), theme);
    const bodyBg = await p.evaluate(() => getComputedStyle(document.body).backgroundColor);
    assert.equal(bodyBg, await p.evaluate(() => {
      const d = document.createElement("div");
      d.style.background = "var(--bg)";
      document.body.append(d);
      return getComputedStyle(d).backgroundColor;
    }));
    // The docs sidebar sits on --panel (Fumadocs' own sidebar palette is not imported).
    const sidebarBg = await p.$eval("#nd-sidebar", (el) => getComputedStyle(el).backgroundColor);
    assert.equal(sidebarBg, await p.evaluate(() => {
      const d = document.createElement("div");
      d.style.background = "var(--panel)";
      document.body.append(d);
      return getComputedStyle(d).backgroundColor;
    }));
    // Code text uses the syntax tokens.
    const codeColors = await p.$$eval("pre code span span", (els) => [...new Set(els.map((e) => getComputedStyle(e).color))]);
    assert.ok(codeColors.includes(await token(p, "--t-str")), `no --t-str in ${codeColors}`);
    await p.context().close();
  });
}

test("primary button text is --accent-ink", async () => {
  const p = await page();
  await p.goto(`${base}/`);
  const color = await p.evaluate(() => {
    const a = document.createElement("a");
    a.className = "btn primary";
    a.textContent = "probe";
    document.body.append(a);
    return getComputedStyle(a).color;
  });
  assert.equal(color, await token(p, "--accent-ink"));
  await p.context().close();
});
