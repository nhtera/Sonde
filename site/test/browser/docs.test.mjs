// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Docs in a browser, on the built site: search from the static index,
// client navigation without a server, and the docs chrome.
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

for (const [query, expected] of [["openapi", /openapi/i], ["--jobs", /--jobs/], ["sonde.yaml", /sonde\.yaml/]]) {
  test(`search finds "${query}" from the static index`, async () => {
    const page = await browser.newPage();
    const requests = [];
    page.on("request", (r) => requests.push(new URL(r.url()).pathname));
    await page.goto(`${base}/docs`);
    await page.waitForLoadState("networkidle");
    await page.keyboard.press("ControlOrMeta+k");
    await page.getByRole("dialog").waitFor();
    await page.keyboard.type(query);
    const results = page.getByRole("dialog").locator('[role="option"], a');
    await assertEventually(async () => (await results.allTextContents()).some((t) => expected.test(t)));
    assert.ok(requests.includes("/api/search.json"));
    await page.close();
  });
}

test("client navigation reads the static docs index, never a server function", async () => {
  const page = await browser.newPage();
  await page.goto(`${base}/docs`);
  await page.waitForLoadState("networkidle");
  const requests = [];
  page.on("request", (r) => requests.push(new URL(r.url()).pathname));
  await page.locator('#nd-sidebar a[href="/docs/guides/openapi"]').click();
  await page.waitForURL("**/docs/guides/openapi");
  await page.getByRole("heading", { level: 1, name: "OpenAPI contracts" }).waitFor();
  assert.equal(await page.title(), "OpenAPI contracts · Sonde docs");
  const nonAsset = requests.filter((p) => !p.startsWith("/assets/"));
  assert.deepEqual(nonAsset, ["/api/docs-tree.json"]);
  await page.close();
});

test("a docs page links back to its source on GitHub", async () => {
  const page = await browser.newPage();
  await page.goto(`${base}/docs/guides/openapi`);
  const edit = page.getByRole("link", { name: "Edit on GitHub" });
  assert.equal(await edit.getAttribute("href"), "https://github.com/nhtera/sonde/edit/main/docs/guides/openapi.md");
  await page.close();
});

// The first search also loads the dialog chunk and the index.
async function assertEventually(fn, timeout = 15000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if (await fn()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  assert.fail("condition not met in time");
}
