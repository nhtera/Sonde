// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The side panels on a copy of results-api made a git repository (its own
// harness; SONDE_VARIABLE_region=eu in the app's environment): the Test
// run says what `sonde --test` says, History masks tokens and cookies,
// Mark secret, the overrides chip and the command for CI, the mock, the
// Changes card, the agents snippet. Screens for the design approval go to
// E2E_CANDIDATES.

import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";
// The fixture API's session cookie (internal/fixture).
const session = "fixture-session-2718";
// Absolute: the command runs in the project folder.
const cli = resolve(process.env.E2E_SONDE_BIN ?? "../bin/sonde");

async function panel(page: Page, name: string) {
  await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name, exact: true }).click();
}

async function openApp(page: Page) {
  await page.goto("/");
  await expect(page.locator(".tree-row").first()).toBeVisible();
}

/** The totals of a summary, numbers that depend on time taken out. */
function totals(text: string): string {
  return text
    .slice(text.indexOf("Executed files:"))
    .replace(/\(\d+(\.\d+)?\/s\)/, "(N/s)")
    .replace(/Duration:.*/, "Duration: N")
    .trim();
}

/** A summary's file lines (in name order: parallel files finish in any
 * order) and totals, times taken out. */
function summaryOf(text: string): string {
  const lines = text
    .split("\n")
    .filter((l) => /^(Success|Failure) /.test(l))
    .map((l) => l.replace(/ in \d+ ms\)$/, " in N ms)"))
    .sort();
  return [...lines, totals(text)].join("\n");
}

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test("the Test run says what sonde --test says", async ({ page }) => {
  await openApp(page);
  await panel(page, "Test run");
  // Leave out the large bodies (big, and the 48 MB export, which writes
  // its output file) and the refused health check: the rest.
  for (const f of ["big.hurl", "export.hurl", "health.hurl"]) await page.getByRole("list", { name: "Files to run" }).getByText(f, { exact: true }).click();
  const files = await page.getByRole("list", { name: "Files to run" }).locator("input:checked + span").allTextContents();
  await page.getByRole("button", { name: /^Run \d+ files?$/ }).click();
  const summary = page.getByLabel("Test summary");
  await expect(summary).toContainText("Executed files:", { timeout: 20_000 });
  await expect(page.getByRole("table", { name: "Files" })).toContainText("users.hurl");
  await page.screenshot({ path: `${candidates}/panels-testrun-4a.png` });
  const cmd = await page.getByLabel("Test run command").textContent();
  expect(cmd).toContain("--variable region=eu"); // the app's SONDE_VARIABLE_ is a flag for CI
  const run = spawnSync(cli, ["--test", "--file-root", ".", "--env", "local", ...files], {
    cwd: "../testdata/results-api",
    env: { ...process.env, SONDE_VARIABLE_region: "eu", NO_COLOR: "1" },
    encoding: "utf8",
  });
  expect(run.stderr).toContain("Executed files:");
  expect(totals(await summary.textContent() ?? "")).toBe(totals(run.stderr));
  // The whole output, as copied: each file line and the totals.
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "Copy the output" }).click();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).not.toMatch(/^(Success|Failure) \//m); // project paths, not paths on disk
  expect(summaryOf(copied)).toBe(summaryOf(run.stderr));
});

test("history shows the token and the session cookie as ***", async ({ page }) => {
  await openApp(page);
  await page.locator(".tree-row", { hasText: "users.hurl" }).first().click();
  await expect(page.locator(".cm-content")).toBeVisible();
  await page.keyboard.press("ControlOrMeta+KeyR");
  const results = page.getByRole("region", { name: "Results" });
  await expect(results.locator(".run-header .outcome")).toHaveText("Failed", { timeout: 20_000 });
  await panel(page, "History");
  await page.getByRole("list", { name: "History" }).getByRole("button", { name: /users\.hurl/ }).first().click();
  await expect(results.getByRole("status")).toContainText("From the history");
  await results.locator(".req-row").nth(1).click();
  await results.getByRole("tab", { name: /^Request/ }).click();
  const panelText = results.getByRole("tabpanel");
  // Credential headers are masked whole, the scheme too.
  await expect(panelText).toContainText("Authorization***");
  await expect(panelText).not.toContainText("Bearer");
  await expect(panelText).toContainText("sid***");
  await expect(results).not.toContainText(session);
  await page.screenshot({ path: `${candidates}/panels-history-8c.png` });
});

test("Mark secret moves a variable out of sonde.yaml, comments kept", async ({ page }) => {
  await openApp(page);
  await panel(page, "Environments");
  await page.getByRole("button", { name: "+ Add variable" }).click();
  await page.getByLabel("New variable name").fill("webhook");
  await page.getByLabel("New variable value").fill("whsec-e2e-value");
  // ⌘S saves it, as the header's Save says.
  await page.keyboard.press("ControlOrMeta+KeyS");
  await expect(page.getByRole("button", { name: "Save", exact: true })).toBeHidden();
  const row = page.getByRole("table", { name: "Variables in local" }).getByRole("row", { name: /webhook/ });
  await expect(row).toContainText("sonde.yaml");
  await row.getByRole("button", { name: "webhook actions" }).click();
  await page.getByRole("menuitem", { name: /Mark as secret/ }).click();
  await expect(row).toContainText("secrets/local.secrets");
  await expect(row.getByLabel("webhook value")).toHaveValue("");
  await page.screenshot({ path: `${candidates}/panels-env-5a.png` });
  // sonde.yaml: the variable is gone, the name listed in secrets: only
  // (never the value), its comments stay.
  await page.getByRole("button", { name: /^Edit sonde\.yaml/ }).click();
  await expect(page.locator(".cm-content")).toBeVisible();
  const yaml = await page.locator(".cm-content").textContent();
  expect(yaml).toContain("# results-api: the Results panel's browser tests");
  expect(yaml).toMatch(/secrets:\s*- webhook/);
  expect(yaml?.match(/webhook/g)).toHaveLength(1);
  expect(yaml).not.toContain("whsec-e2e-value");
});

test("a proxy in Settings counts as an override and is a flag for CI", async ({ page }) => {
  await openApp(page);
  const chip = page.locator(".titlebar .overrides-chip");
  await expect(chip).toContainText("1 override"); // SONDE_VARIABLE_region
  await panel(page, "Settings");
  const proxy = page.getByRole("textbox", { name: "Proxy" });
  try {
    await proxy.fill("http://proxy.e2e.test:3128");
    await proxy.press("Enter");
    await expect(chip).toContainText("2 overrides");
    await page.screenshot({ path: `${candidates}/panels-settings-12a.png` });
    await panel(page, "Test run");
    await expect(page.getByLabel("Test run command")).toContainText("--proxy http://proxy.e2e.test:3128");
  } finally {
    // The harness is shared: the next tests must not go through the proxy.
    await panel(page, "Settings");
    await proxy.fill("");
    await proxy.press("Enter");
  }
  await expect(chip).toContainText("1 override");
});

test("kept cookies show in the jar without their values, and delete", async ({ page }) => {
  await openApp(page);
  await panel(page, "Settings");
  const keep = page.getByRole("switch", { name: "Keep cookies between runs" });
  // The switch follows the saved settings.
  await keep.click();
  await expect(keep).toBeChecked();
  try {
    await panel(page, "Files");
    await page.locator(".tree-row", { hasText: "users.hurl" }).first().click();
    await expect(page.locator(".cm-content")).toBeVisible();
    await page.keyboard.press("ControlOrMeta+KeyR");
    await expect(page.getByRole("region", { name: "Results" }).locator(".run-header .outcome")).toHaveText("Failed", { timeout: 20_000 });
    await panel(page, "Settings");
    await page.getByRole("button", { name: "Manage the cookie jar" }).click();
    const jar = page.getByRole("dialog", { name: "Cookie jar" });
    const sid = jar.getByRole("row", { name: /sid/ });
    await expect(sid).toContainText("HttpOnly");
    await expect(jar).not.toContainText(session);
    await page.screenshot({ path: `${candidates}/panels-cookies-12c.png` });
    await sid.getByRole("button", { name: "Delete sid" }).click();
    await expect(sid).toHaveCount(0);
  } finally {
    await page.keyboard.press("Escape");
    // The rail's button toggles: open Settings only when it is not open.
    if (!(await keep.isVisible())) await panel(page, "Settings");
    if (await keep.isChecked()) await keep.click();
    await expect(keep).not.toBeChecked();
  }
});

test("the mock answers for the spec, then stops", async ({ page }) => {
  await openApp(page);
  await panel(page, "Contract & mock");
  await expect(page.getByLabel(/of 2 operations covered/)).toBeVisible();
  await page.getByLabel("Mock port").fill("34131");
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page.getByRole("region", { name: "Mock server" })).toContainText("base_url points here");
  await page.screenshot({ path: `${candidates}/panels-contract-5b.png` });
  await panel(page, "Files");
  await page.locator(".tree-row", { hasText: "health.hurl" }).first().click();
  await expect(page.locator(".cm-content")).toBeVisible();
  await page.keyboard.press("ControlOrMeta+KeyR");
  await expect(page.getByRole("region", { name: "Results" }).locator(".run-header .outcome")).toHaveText("Passed", { timeout: 20_000 });
  await panel(page, "Contract & mock");
  await expect(page.getByLabel(/1 of 2 operations covered/)).toBeVisible();
  await page.getByRole("button", { name: "Stop" }).click();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeVisible();
});

test("the agents snippet names the project and refuses every host in the repo", async ({ page }) => {
  await openApp(page);
  await panel(page, "AI agents");
  await expect(page.getByLabel("Agent configuration")).toContainText("sonde mcp --root");
  await page.getByRole("switch", { name: "Allow the agent to send requests" }).click();
  await expect(page.getByLabel("Agent configuration")).toContainText("--allow-run --allow-host localhost");
  await page.getByRole("button", { name: "In the repo" }).click();
  await page.getByLabel("Add a host").fill("*");
  await page.getByLabel("Add a host").press("Enter");
  await expect(page.getByText(/every host \(\*\) is refused/)).toBeVisible();
  await page.screenshot({ path: `${candidates}/panels-agents-5c.png` });
});

test("the Changes card commits after the folder is trusted", async ({ page }) => {
  await openApp(page);
  await page.locator(".tree-row", { hasText: "page.hurl" }).first().click();
  await page.locator(".cm-line").first().click();
  await page.keyboard.press("End");
  await page.keyboard.type(" (edited)");
  await page.keyboard.press("ControlOrMeta+KeyS");
  const card = page.getByRole("region", { name: "Changes" });
  // The harness's data is new: the folder is not trusted yet.
  await card.getByRole("button", { name: "Trust this folder to see changes" }).click();
  const changed = card.getByRole("list", { name: "Changed files" });
  await expect(changed).toContainText("page.hurl");
  // Mark secret (above) changed the secrets file: listed, never committed.
  const secrets = changed.getByRole("listitem").filter({ hasText: "secrets/local.secrets" });
  await expect(secrets).toContainText("never committed");
  await card.getByLabel("Commit message").fill("Edit the receipt page");
  await page.screenshot({ path: `${candidates}/panels-changes-5d.png` });
  await card.getByRole("button", { name: /^Commit to main/ }).click();
  await expect(page.getByText(/Committed \w+ to main/)).toBeVisible();
  await expect(changed).not.toContainText("page.hurl");
  await expect(secrets).toHaveCount(1);
});
