// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Design screens on results-api, made a git repository (see
// e2e/visual/snap.ts). In order: the mock started late, files changed for
// the Changes card, an import, then a new file.

import { expect, test, type Page } from "@playwright/test";
import { dark, home, open, panel, pinTimings, run, snap } from "./visual/snap";

test.describe.configure({ mode: "serial" });

const results = (page: Page) => page.getByRole("region", { name: "Results" });
const tab = (page: Page, name: string) => results(page).getByRole("tab", { name: new RegExp(`^${name}`) });

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await home(page);
  await dark(page);
  // The design screens show a trusted folder or none: the question waits.
  const notNow = page.getByRole("button", { name: "Not now" });
  if (await notNow.isVisible()) await notNow.click();
});

test("3a, 3b, 3c: body, timeline and cookies", async ({ page }) => {
  await open(page, "users.hurl");
  await run(page, /Failed/);
  // The failing request, its body searched for the value (as the design).
  await results(page).locator(".req-row").nth(1).click();
  await tab(page, "Body").click();
  await results(page).getByLabel("Search the body").fill("ada");
  await expect(results(page).locator(".body-search")).toContainText("1 of 1");
  await snap(page, "3a");
  await results(page).locator(".req-row").nth(0).click();
  await tab(page, "Timeline").click();
  await expect(results(page).getByRole("tabpanel").getByLabel("Log")).toBeVisible();
  await pinTimings(page);
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
  // Lines of NDJSON, as the design's export.
  await open(page, "export.hurl");
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
  await expect(results(page).locator(".session-state")).toHaveText("Open");
  await results(page).getByLabel("Message", { exact: true }).fill('{"type": "ping"}');
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(results(page).locator(".stream-row")).toHaveCount(2);
  // The next message being written, as the design shows it.
  await results(page).getByLabel("Message", { exact: true }).fill('{"type": "ping"}');
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
  await page.getByRole("switch", { name: "Check responses against the OpenAPI spec" }).click();
  await page.getByLabel("Mock port").fill("34129");
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page.getByRole("region", { name: "Mock server" })).toContainText("base_url points here");
  await open(page, "health.hurl");
  await run(page, /Passed/);
  // The user the API returns has no email: the spec requires one.
  await open(page, "users.hurl");
  await run(page, /Failed/);
  await results(page).locator(".req-row").nth(1).click();
  await tab(page, "Asserts").click();
  await panel(page, "Contract & mock");
  await expect(page.getByLabel(/2 of 2 operations covered/)).toBeVisible();
  await expect(page.getByLabel("Mock requests")).toContainText("/health");
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
  // A file's last run beside the environment, as the design has it.
  await open(page, "users.hurl");
  await run(page, /Failed/);
  await panel(page, "Environments");
  await expect(page.getByRole("table", { name: "Variables in local" })).toBeVisible();
  // A session override, then a secret being added.
  await page.getByRole("button", { name: "user actions" }).click();
  await page.getByRole("menuitem", { name: /Override for this session/ }).click();
  await page.getByRole("dialog").getByRole("textbox").fill("grace");
  await page.getByRole("dialog").getByRole("textbox").press("Enter");
  await expect(page.getByRole("button", { name: "Remove the override of user" })).toBeVisible();
  await page.getByRole("button", { name: "+ Add variable" }).click();
  await page.getByLabel("New variable name").fill("webhook_secret");
  await page.getByRole("switch", { name: "Secret" }).check();
  await page.getByLabel("New variable value").fill("whsec-visual");
  // The new row in full (a variable's menu would cover it), its name
  // being edited, as the design has it.
  await page.getByLabel("New variable name").focus();
  await snap(page, "5a");
  await page.getByRole("button", { name: "Remove the override of user" }).click();
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
  await expect(dialog.getByRole("radiogroup", { name: "Layout" })).toContainText("users/get-user.hurl");
  await snap(page, "12a2");
  await dialog.getByRole("button", { name: "Import", exact: true }).click();
  await expect(dialog.getByLabel("Import counts")).toBeVisible();
  await snap(page, "12b");
  await dialog.getByRole("button", { name: /Review \d+ suggestion/ }).click();
  await expect(dialog.getByRole("list", { name: "Files with suggestions" })).toBeVisible();
  // One change accepted, one to decide.
  await dialog.locator(".change-card").first().getByRole("button", { name: "Accept", exact: true }).click();
  await snap(page, "12b2");
});
