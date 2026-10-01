// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Design screens on shop-api (see e2e/visual/snap.ts). In order: later
// screens show what earlier ones left (a run in the history, a new file).

import { expect, test } from "@playwright/test";
import { dark, home, light, open, panel, run, snap } from "./visual/snap";

test.describe.configure({ mode: "serial" });

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await home(page);
  await dark(page);
});

/** Puts the cursor at the end of the line holding text. */
async function endOf(page: import("@playwright/test").Page, text: string) {
  await page.locator(".cm-line", { hasText: text }).first().click();
  await page.keyboard.press("End");
}

test("1a and 1b: edited after the run: stale warning and an undefined variable", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await endOf(page, "POST {{base_url}}/carts");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-Trace: {{trace_id}}");
  await expect(page.getByText(/Edited since this run/)).toBeVisible();
  await expect(page.locator(".cm-lintRange-warning, .cm-lintRange-error").first()).toBeVisible({ timeout: 10_000 });
  await page.keyboard.press("Escape");
  await snap(page, "1a");
  await light(page);
  await snap(page, "1b");
});

test("6a: the command palette", async ({ page }) => {
  await open(page, "checkout.hurl");
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type("ch");
  await expect(page.getByRole("dialog").getByRole("option").first()).toBeVisible();
  await snap(page, "6a");
});

test("6b: import a pasted curl command, secrets lifted", async ({ page }) => {
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type("Import curl");
  await page.keyboard.press("Enter");
  await page.getByLabel("Pasted input").fill("curl -H 'Authorization: Bearer visual-token-1234567' 'https://api.shop.test/orders?limit=5'");
  await expect(page.getByLabel("Preview")).toContainText("Bearer {{token}}");
  await snap(page, "6b");
});

test("6c: a variable's value and source on hover", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  const variable = page.locator(".cm-line", { hasText: "GET {{base_url}}/users" }).first().locator("span", { hasText: /base_url/ }).last();
  const box = (await variable.boundingBox())!;
  for (let i = 0; i < 6; i++) await page.mouse.move(box.x - 20 + i * 6, box.y + box.height / 2);
  await expect(page.locator(".cm-sonde-hover")).toHaveCount(1);
  await snap(page, "6c");
});

test("6d: variable autocomplete while typing {{ in a header", async ({ page }) => {
  await open(page, "checkout.hurl");
  await endOf(page, "Authorization: Bearer {{token}}");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-User: {{");
  await expect(page.locator(".cm-tooltip-autocomplete")).toBeVisible();
  await snap(page, "6d");
});

test("8a: requests under each file, a folder's menu", async ({ page }) => {
  await open(page, "checkout.hurl");
  await page.locator(".tree-row", { hasText: "data" }).first().click({ button: "right" });
  await expect(page.getByRole("menu")).toBeVisible();
  await snap(page, "8a");
});

test("8b: the filter searches requests across files", async ({ page }) => {
  await page.getByLabel("Filter requests in all files").fill("carts");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("/carts");
  await snap(page, "8b");
});

test("8e: Copy as", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await page.locator(".cm-line", { hasText: "POST {{base_url}}/carts" }).first().click();
  await page.getByRole("button", { name: "Copy as" }).click();
  await expect(page.getByRole("menuitem", { name: /^curl · this request/ })).toBeVisible();
  await expect(page.locator(".copy-preview").first()).not.toHaveText("…");
  await snap(page, "8e");
});

test("12a: Settings", async ({ page }) => {
  await panel(page, "Settings");
  await expect(page.getByRole("region", { name: "Appearance" })).toBeVisible();
  await snap(page, "12a");
});

test("12c: the cookie jar", async ({ page }) => {
  await panel(page, "Settings");
  const keep = page.getByRole("switch", { name: "Keep cookies between runs" });
  await keep.click();
  await expect(keep).toBeChecked();
  try {
    await open(page, "checkout.hurl");
    await run(page, /Failed/);
    await panel(page, "Settings");
    await page.getByRole("button", { name: "Manage the cookie jar" }).click();
    await expect(page.getByRole("dialog", { name: "Cookie jar" }).getByRole("row", { name: /sid/ })).toBeVisible();
    await snap(page, "12c");
    await page.keyboard.press("Escape");
  } finally {
    if (!(await keep.isVisible())) await panel(page, "Settings");
    await keep.click();
    await expect(keep).not.toBeChecked();
  }
});

test("8c: the history", async ({ page }) => {
  await panel(page, "History");
  await expect(page.getByRole("list", { name: "History" }).getByRole("button").first()).toBeVisible();
  await snap(page, "8c");
});

test("7d: a narrow window (1024)", async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 900 });
  // The side panel collapses to the rail: the file from the palette.
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type("checkout.hurl");
  await page.keyboard.press("Enter");
  await expect(page.locator(".cm-content")).toBeVisible();
  await run(page, /Failed/);
  await snap(page, "7d");
});

test("7c: a new file, never run", async ({ page }) => {
  await page.getByRole("button", { name: "New file" }).click();
  await page.getByRole("dialog").getByRole("textbox").fill("new-request.hurl");
  await page.getByRole("dialog").getByRole("textbox").press("Enter");
  await expect(page.getByRole("tablist", { name: "Open files" }).getByRole("tab", { selected: true })).toContainText("new-request.hurl");
  await snap(page, "7c");
});
