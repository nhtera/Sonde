// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The whole journey on one project (a copy of results-api, its own
// harness, the fixture API on 34120): run a file with a failure, Send a
// request, + Assert, edit in the Form and save, mark a variable secret,
// start the mock and pass against it, a test run, import a Postman
// collection and apply a suggestion, then run the command Copy as gives.
// Server mode has no export (a dialog): the Go tests cover it.

import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";

const cli = resolve(process.env.E2E_SONDE_BIN ?? "../bin/sonde");
const results = (page: Page) => page.getByRole("region", { name: "Results" });
const outcome = (page: Page) => results(page).locator(".run-header .outcome");
const editorText = (page: Page) =>
  page.locator(".cm-content").evaluate((el) => [...el.querySelectorAll(".cm-line")].map((l) => l.textContent).join("\n"));

async function open(page: Page, file: string) {
  // The rail's button toggles: Files only when the tree is not showing.
  if (!(await page.getByRole("tree", { name: "Project files" }).isVisible())) {
    await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name: "Files", exact: true }).click();
  }
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.getByRole("tablist", { name: "Open files" }).getByRole("tab", { selected: true })).toContainText(file);
  await expect(page.locator(".cm-run-run").first()).toBeVisible();
}

async function panel(page: Page, name: string) {
  await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name, exact: true }).click();
}

async function command(page: Page, title: string) {
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type(title);
  await page.keyboard.press("Enter");
}

test("the whole journey", async ({ page, context }) => {
  test.setTimeout(180_000);
  await page.setViewportSize({ width: 1440, height: 900 });
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  await expect(page.locator(".tree-row").first()).toBeVisible();

  await test.step("run a file: its failing assert first", async () => {
    await open(page, "users.hurl");
    await page.getByRole("banner").getByRole("button", { name: /^Run file/ }).click();
    await expect(outcome(page)).toHaveText("Failed", { timeout: 20_000 });
    await expect(results(page).getByRole("alert")).toContainText(`jsonpath "$.name" == "grace"`);
  });

  await test.step("Send request 2, reusing the run's captures", async () => {
    await page.locator(".cm-line", { hasText: "GET {{api}}/users/{{user_id}}" }).click();
    await command(page, "Send request at cursor");
    await expect(results(page)).toContainText("Sent request 2");
  });

  await test.step("+ Assert from the response body", async () => {
    await results(page).getByRole("tab", { name: /^Body/ }).click();
    const id = results(page).locator(".jt-row", { hasText: '"id"' }).first();
    await id.hover();
    await id.getByRole("button", { name: "+ Assert" }).click();
    await expect(page.locator(".cm-line", { hasText: /^jsonpath "\$\.id" == / })).toHaveCount(1);
  });

  await test.step("edit in the Form, save", async () => {
    await page.getByRole("group", { name: "Editor view" }).getByRole("button", { name: "Form" }).click();
    await page.getByRole("tablist", { name: "Request parts" }).getByRole("tab", { name: /^Options/ }).click();
    await page.getByRole("switch", { name: "Compressed" }).click();
    await expect(page.getByRole("switch", { name: "Compressed" })).toBeChecked();
    await page.getByRole("group", { name: "Editor view" }).getByRole("button", { name: "Text" }).click();
    await page.keyboard.press("ControlOrMeta+KeyS");
    await page.reload();
    await expect(page.locator(".tree-row").first()).toBeVisible();
    await open(page, "users.hurl");
    const text = await editorText(page);
    expect(text).toContain("compressed: true");
    expect(text).toMatch(/jsonpath "\$\.id" == /);
  });

  await test.step("mark a variable secret", async () => {
    await panel(page, "Environments");
    await page.getByRole("button", { name: "+ Add variable" }).click();
    await page.getByLabel("New variable name").fill("signing_key");
    await page.getByLabel("New variable value").fill("full-flow-signing-value");
    await page.getByRole("button", { name: "Add", exact: true }).click();
    const row = page.getByRole("table", { name: "Variables in local" }).getByRole("row", { name: /signing_key/ });
    await row.getByRole("button", { name: "signing_key actions" }).click();
    await page.getByRole("menuitem", { name: /Mark as secret/ }).click();
    await expect(row).toContainText("secrets/local.secrets");
  });

  await test.step("start the mock: a file against it passes", async () => {
    await panel(page, "Contract & mock");
    await page.getByLabel("Mock port").fill("34129");
    await page.getByRole("button", { name: "Start", exact: true }).click();
    await expect(page.getByRole("region", { name: "Mock server" })).toContainText("base_url points here");
    await open(page, "health.hurl");
    await page.getByRole("banner").getByRole("button", { name: /^Run file/ }).click();
    await expect(outcome(page)).toHaveText("Passed", { timeout: 20_000 });
  });

  await test.step("a test run of two files", async () => {
    await panel(page, "Test run");
    const files = page.getByRole("list", { name: "Files to run" });
    for (const f of await files.locator("input:checked + span").allTextContents()) {
      if (f !== "users.hurl" && f !== "health.hurl") await files.getByText(f, { exact: true }).click();
    }
    await page.getByRole("button", { name: "Run 2 files" }).click();
    await expect(page.getByLabel("Test summary")).toContainText("Executed files:    2", { timeout: 20_000 });
    await expect(page.getByLabel("Test summary")).toContainText("Succeeded files:   1");
    await panel(page, "Contract & mock");
    await page.getByRole("button", { name: "Stop" }).click();
  });

  await test.step("import a Postman collection, apply one suggestion", async () => {
    await command(page, "Import Postman");
    const dialog = page.getByRole("dialog");
    const chooser = page.waitForEvent("filechooser");
    await dialog.getByRole("button", { name: "Choose file…" }).click();
    await (await chooser).setFiles("../testdata/import/shop.postman_collection.json");
    await expect(dialog.getByRole("radiogroup", { name: "Layout" })).toContainText("users/get-user.hurl");
    await dialog.getByRole("button", { name: "Import", exact: true }).click();
    await dialog.getByRole("button", { name: /Review suggestions/ }).click();
    await dialog.getByRole("list", { name: "Files with suggestions" }).getByRole("button", { name: /get-user\.hurl/ }).click();
    await dialog.getByRole("region", { name: /get-user\.hurl/ }).getByRole("button", { name: "Accept all in file" }).click();
    await dialog.getByRole("button", { name: /^Apply 2 accepted/ }).click();
    await expect(page.getByText("Applied the suggestions to 1 file")).toBeVisible();
    await open(page, "get-user.hurl");
    expect(await editorText(page)).toContain('jsonpath "$.id" == 42');
  });

  await test.step("Copy as sonde, run in the project: it fails where the app's run does", async () => {
    await open(page, "users.hurl");
    await page.getByRole("button", { name: "Copy as" }).click();
    await page.getByRole("menuitem", { name: /^sonde · test run/ }).click();
    await expect(page.getByText(/^Copied/)).toBeVisible();
    const line = (await page.evaluate(() => navigator.clipboard.readText())).split("\n").find((l) => l.startsWith("sonde "))!;
    const run = spawnSync("sh", ["-c", `"${cli}" ${line.slice("sonde ".length)}`], { cwd: "../testdata/results-api", env: { ...process.env, NO_COLOR: "1" }, encoding: "utf8" });
    expect(run.status).toBe(4);
    expect(run.stderr).toContain("users.hurl:15");
  });
});
