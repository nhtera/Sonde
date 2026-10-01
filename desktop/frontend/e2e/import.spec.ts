// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Import and Copy as on a copy of shop-api (its own harness;
// SONDE_VARIABLE_region=eu in the app's environment, the fixture API on
// 34120): a pasted curl command with its credentials lifted, a Postman
// collection with its counts and suggestions, and the commands Copy as
// gives, one of them run. Screens for the design approval go to
// E2E_CANDIDATES.

import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";
// Absolute: the command runs in the project folder.
const cli = resolve(process.env.E2E_SONDE_BIN ?? "../bin/sonde");
const fixtures = "../testdata/import";
const curlCommand = readFileSync(`${fixtures}/bearer.curl`, "utf8");
const curlSecrets = ["e2e-curl-bearer-sentinel-7731", "e2e-curl-apikey-sentinel-4410", "e2e-curl-query-sentinel-9902", "e2e-curl-password-sentinel-5521"];

test.beforeEach(async ({ page, context }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");
  await expect(page.locator(".tree-row").first()).toBeVisible();
});

async function command(page: Page, title: string) {
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type(title);
  await page.keyboard.press("Enter");
}

async function panel(page: Page, name: string) {
  await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name, exact: true }).click();
}

/** Opens file and runs it. */
async function runFile(page: Page, file: string) {
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.locator(".cm-content")).toContainText("POST");
  await page.getByRole("banner").getByRole("button", { name: /^Run file/ }).click();
}

const clipboard = (page: Page) => page.evaluate(() => navigator.clipboard.readText());

/** Opens file and copies the Copy as item named title; returns the text. */
async function copyAs(page: Page, file: string, title: RegExp) {
  await page.locator(".tree-row", { hasText: file }).first().click();
  await page.locator(".cm-line").nth(1).click();
  await page.getByRole("button", { name: "Copy as" }).click();
  await page.getByRole("menuitem", { name: title }).click();
  await expect(page.getByText(/^Copied/)).toBeVisible();
  return clipboard(page);
}

test("a pasted curl command: its credentials become {{names}}, their values the env's secrets", async ({ page }) => {
  await command(page, "Import curl");
  const dialog = page.getByRole("dialog", { name: "Import" });
  await dialog.getByLabel("Pasted input").fill(curlCommand);
  const preview = dialog.getByLabel("Preview");
  await expect(preview).toContainText("Authorization: Bearer {{token}}");
  await expect(preview).toContainText("api_key={{api_key}}");
  // shop-api's local env has a password: the curl one is password_2.
  await expect(preview).toContainText("alice:{{password_2}}");
  for (const v of curlSecrets) await expect(preview).not.toContainText(v);
  await page.screenshot({ path: `${candidates}/import-12a.png` });
  await dialog.getByRole("button", { name: "Import", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("token");
  await page.getByRole("button", { name: "Open the files" }).click();
  await expect(page.locator(".cm-content")).toContainText("Bearer {{token}}");
  const html = await page.content();
  for (const v of curlSecrets) expect(html).not.toContain(v);
  // The values went to the secrets file: listed as secrets of local.
  await panel(page, "Environments");
  const table = page.getByRole("table", { name: "Variables in local" });
  for (const name of ["token", "api_key", "x-api-key", "password_2"]) {
    await expect(table.getByRole("row", { name: new RegExp(`^${name}\\b`) })).toContainText("secrets/local.secrets");
  }
});

test("a Postman collection: honest counts, then suggestions accepted for one file and rejected for another", async ({ page }) => {
  await command(page, "Import Postman");
  const dialog = page.getByRole("dialog");
  let chooser = page.waitForEvent("filechooser");
  await dialog.getByRole("button", { name: "Choose file…" }).click();
  await (await chooser).setFiles(`${fixtures}/shop.postman_collection.json`);
  chooser = page.waitForEvent("filechooser");
  await dialog.getByRole("button", { name: "+ Add" }).click();
  await (await chooser).setFiles(`${fixtures}/dev.postman_environment.json`);
  await dialog.getByLabel("Into folder").fill("imported/shop");
  await expect(dialog.getByRole("list", { name: "Files written" })).toContainText("imported/shop/users/get-user.hurl");
  await page.screenshot({ path: `${candidates}/import-12a2.png` });
  await dialog.getByRole("button", { name: "Import", exact: true }).click();
  const counts = dialog.getByLabel("Import counts");
  await expect(counts).toContainText("3requests → 3 files");
  await expect(counts).toContainText("2environments");
  await expect(counts).toContainText("1status checks → HTTP lines · 1 path variable → {{name}}");
  await expect(counts).toContainText("1secret stubs to fill in");
  await expect(dialog).toContainText("2 scripts kept as # comments");
  await page.screenshot({ path: `${candidates}/import-12b.png` });
  await dialog.getByRole("button", { name: /Review suggestions/ }).click();
  const files = dialog.getByRole("list", { name: "Files with suggestions" });
  await files.getByRole("button", { name: /get-user\.hurl/ }).click();
  await dialog.getByRole("region", { name: /get-user\.hurl/ }).getByRole("button", { name: "Accept", exact: true }).click();
  await files.getByRole("button", { name: /list-orders\.hurl/ }).click();
  await dialog.getByRole("region", { name: /list-orders\.hurl/ }).getByRole("button", { name: "Reject", exact: true }).click();
  await expect(dialog).toContainText("1 accepted · 1 rejected");
  await page.screenshot({ path: `${candidates}/import-12b2.png` });
  await dialog.getByRole("button", { name: "Apply 1 accepted" }).click();
  await expect(page.getByText("Applied the suggestions to 1 file")).toBeVisible();
  await page.locator(".tree-row", { hasText: "get-user.hurl" }).first().click();
  await expect(page.locator(".cm-content")).toContainText('jsonpath "$.id" == 42');
  await expect(page.locator(".cm-content")).toContainText("# test script (never executed):");
  await page.locator(".tree-row", { hasText: "list-orders.hurl" }).first().click();
  await expect(page.locator(".cm-content")).not.toContainText("access_token");
});

test("Copy as sonde: --file-root ., the env, a flag per override, the app's SONDE_VARIABLE_", async ({ page }) => {
  const text = await copyAs(page, "checkout.hurl", /^sonde · run this file/);
  expect(text).toContain("--file-root .");
  expect(text).toContain("--env local");
  expect(text).toContain("--variable region=eu");
  await page.getByRole("button", { name: "Copy as" }).click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: `${candidates}/copyas-8e.png` });
  // No secret values in server mode: no ⌥ item, ⌥ changes nothing.
  await expect(page.getByRole("menu")).not.toContainText("Copy with secret values");
  await page.keyboard.down("Alt");
  await expect(page.getByRole("menu")).toContainText("secrets as variable references");
  await page.keyboard.up("Alt");
  await page.keyboard.press("Escape");
});

test("Copy as curl takes a session override", async ({ page }) => {
  await panel(page, "Environments");
  const row = page.getByRole("table", { name: "Variables in local" }).getByRole("row", { name: /^base_url\b/ });
  await row.getByRole("button", { name: "base_url actions" }).click();
  await page.getByRole("menuitem", { name: /Override for this session/ }).click();
  const value = page.getByRole("dialog").getByLabel("Value");
  await value.fill("http://localhost:9090");
  await value.press("Enter");
  try {
    await panel(page, "Files");
    const text = await copyAs(page, "checkout.hurl", /^curl · this request/);
    expect(text).toContain("http://localhost:9090/login");
  } finally {
    await panel(page, "Environments");
    await page.getByRole("button", { name: "Remove the override of base_url" }).click();
  }
});

test("after a Send, Copy as sonde runs up to the request sent, and says so", async ({ page }) => {
  await runFile(page, "checkout.hurl");
  const results = page.getByRole("region", { name: "Results" });
  // The fixture's checkout stays pending: the run fails at its last assert.
  await expect(results.locator(".run-header .outcome")).toHaveText("Failed", { timeout: 20_000 });
  // The cursor in request 2, then Send.
  await page.locator(".cm-line", { hasText: "GET {{base_url}}/users" }).click();
  await command(page, "Send request at cursor");
  await expect(results).toContainText("Sent request 2");
  await page.getByRole("button", { name: "Copy as" }).click();
  await page.getByRole("menuitem", { name: /^sonde · the Send of request 2/ }).click();
  await expect(page.getByText(/Send reused captures from the run at \d\d:\d\d; this command runs 1–2/)).toBeVisible();
  expect(await clipboard(page)).toContain("--to-entry 2");
});

test("the copied test command, run in the project, fails where the desktop's run fails", async ({ page }) => {
  await runFile(page, "checkout.hurl");
  const results = page.getByRole("region", { name: "Results" });
  await expect(results.locator(".run-header .outcome")).toHaveText("Failed", { timeout: 20_000 });
  // The desktop: the checkout's status assert, line 32.
  const failure = results.locator(".failure-box").first();
  await expect(failure).toContainText("line 32");
  const text = await copyAs(page, "checkout.hurl", /^sonde · test run/);
  const line = text.split("\n").find((l) => l.startsWith("sonde "))!;
  const run = spawnSync("sh", ["-c", `"${cli}" ${line.slice("sonde ".length)}`], {
    cwd: "../testdata/shop-api",
    env: { ...process.env, SONDE_VARIABLE_region: "eu", NO_COLOR: "1" },
    encoding: "utf8",
  });
  // The command: assert failures only (exit 4), at the same line, after
  // the same requests.
  expect(run.status).toBe(4);
  expect(run.stderr).toContain("checkout.hurl:32");
  expect(run.stderr).toContain("Failure checkout.hurl (5 request(s)");
});
