// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Design screens on results-api, made a git repository (see
// e2e/visual/snap.ts). In order: the mock started late, files changed for
// the Changes card, an import, then a new file.

import { expect, test, type Page } from "@playwright/test";
import { dark, home, open, panel, run, snap } from "./visual/snap";

test.describe.configure({ mode: "serial" });

const results = (page: Page) => page.getByRole("region", { name: "Results" });
const tab = (page: Page, name: string) => results(page).getByRole("tab", { name: new RegExp(`^${name}`) });

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await home(page);
  await dark(page);
});

test("3a, 3b, 3c: body, timeline and cookies", async ({ page }) => {
  await open(page, "users.hurl");
  await run(page, /Failed/);
  await results(page).locator(".req-row").nth(0).click();
  await tab(page, "Body").click();
  const id = results(page).locator(".jt-row", { hasText: '"token"' }).first();
  await id.hover();
  await snap(page, "3a");
  await tab(page, "Timeline").click();
  await expect(results(page).getByRole("tabpanel").getByLabel("Log")).toBeVisible();
  await snap(page, "3b");
  await tab(page, "Cookies").click();
  await expect(results(page).getByRole("tabpanel")).toContainText("Received");
  await snap(page, "3c");
});

test("10a: the sandboxed HTML preview", async ({ page }) => {
  await open(page, "page.hurl");
  await run(page, /Passed/);
  await results(page).locator(".req-row").nth(0).click();
  await tab(page, "Body").click();
  await results(page).getByRole("button", { name: "Preview" }).click();
  await expect(page.frameLocator("iframe[title='Response preview']").locator("h1")).toHaveText("Receipt");
  await snap(page, "10a");
});

test("10b: a very large body", async ({ page }) => {
  await open(page, "big.hurl");
  await run(page, /Passed/);
  await expect(results(page).getByLabel("Large response")).toBeVisible();
  await snap(page, "10b");
});

test("11a: a server-sent event stream", async ({ page }) => {
  await open(page, "events.sonde");
  await run(page, /Passed/);
  await expect(results(page).locator(".stream-stop")).toBeVisible();
  await snap(page, "11a");
});

test("11b and 11b2: a WebSocket transcript, then an interactive session", async ({ page }) => {
  await open(page, "live.sonde");
  await run(page, /Passed/);
  await snap(page, "11b");
  await results(page).getByRole("button", { name: "Open interactive session" }).click();
  await expect(results(page).locator(".session-head .outcome")).toHaveText("Open");
  await results(page).getByLabel("Message", { exact: true }).fill('{"type": "ping"}');
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(results(page).locator(".stream-row")).toHaveCount(2);
  await snap(page, "11b2");
  await page.keyboard.press("Escape");
});

test("10c: a refused connection", async ({ page }) => {
  await open(page, "health.hurl");
  await run(page, /Failed/);
  await expect(results(page).getByRole("alert")).toContainText("Connection refused");
  await snap(page, "10c");
});

test("10d: the same card for a name that does not resolve", async ({ page }) => {
  await page.getByRole("button", { name: "New file" }).click();
  await page.getByRole("dialog").getByRole("textbox").fill("dns.hurl");
  await page.getByRole("dialog").getByRole("textbox").press("Enter");
  await expect(page.locator(".cm-content")).toBeVisible();
  await page.locator(".cm-content").click();
  await page.keyboard.press("ControlOrMeta+KeyA");
  await page.keyboard.type("GET http://no-such-host.invalid/health\nHTTP 204\n");
  await page.keyboard.press("ControlOrMeta+KeyS");
  await run(page, /Failed|Error/);
  await expect(results(page).getByRole("alert")).toBeVisible();
  await snap(page, "10d");
});

test("5b: contract and mock", async ({ page }) => {
  await panel(page, "Contract & mock");
  await page.getByLabel("Mock port").fill("34129");
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page.getByRole("region", { name: "Mock server" })).toContainText("base_url points here");
  await open(page, "health.hurl");
  await run(page, /Passed/);
  await panel(page, "Contract & mock");
  await expect(page.getByLabel(/1 of 2 operations covered/)).toBeVisible();
  await snap(page, "5b");
});

test("4a: a test run", async ({ page }) => {
  await panel(page, "Test run");
  const files = page.getByRole("list", { name: "Files to run" });
  for (const f of await files.locator("input:checked + span").allTextContents()) {
    if (!["users.hurl", "health.hurl", "page.hurl"].includes(f)) await files.getByText(f, { exact: true }).click();
  }
  await page.getByRole("button", { name: "Run 3 files" }).click();
  await expect(page.getByLabel("Test summary")).toContainText("Executed files:    3", { timeout: 20_000 });
  await snap(page, "4a");
  await panel(page, "Contract & mock");
  await page.getByRole("button", { name: "Stop" }).click();
});

test("5a: environments", async ({ page }) => {
  await panel(page, "Environments");
  await expect(page.getByRole("table", { name: "Variables in local" })).toBeVisible();
  await snap(page, "5a");
});

test("5c: AI agents", async ({ page }) => {
  await panel(page, "AI agents");
  await page.getByRole("switch", { name: "Allow the agent to send requests" }).click();
  await expect(page.getByLabel("Agent configuration")).toContainText("--allow-run");
  await snap(page, "5c");
});

test("5d: files with git, Changes", async ({ page }) => {
  await page.getByRole("region", { name: "Changes" }).getByRole("button", { name: "Trust this folder to see changes" }).click();
  await expect(page.getByRole("region", { name: "Changes" }).getByRole("list", { name: "Changed files" })).toContainText("dns.hurl");
  await page.getByRole("region", { name: "Changes" }).getByLabel("Commit message").fill("Add the DNS check");
  await snap(page, "5d");
});

test("12a2, 12b, 12b2: import a Postman collection, then its suggestions", async ({ page }) => {
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type("Import Postman");
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog");
  const chooser = page.waitForEvent("filechooser");
  await dialog.getByRole("button", { name: "Choose file…" }).click();
  await (await chooser).setFiles("../testdata/import/shop.postman_collection.json");
  await expect(dialog.getByRole("list", { name: "Files written" })).toContainText("imported/users/get-user.hurl");
  await snap(page, "12a2");
  await dialog.getByRole("button", { name: "Import", exact: true }).click();
  await expect(dialog.getByLabel("Import counts")).toBeVisible();
  await snap(page, "12b");
  await dialog.getByRole("button", { name: /Review suggestions/ }).click();
  await expect(dialog.getByRole("list", { name: "Files with suggestions" })).toBeVisible();
  await snap(page, "12b2");
});
