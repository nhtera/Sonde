// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Design screens on shop-api (see e2e/visual/snap.ts). In order: later
// screens show what earlier ones left (a run in the history, a new file).

import { expect, test, type Page } from "@playwright/test";
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
  await page.keyboard.type("che");
  await expect(page.getByRole("dialog").getByRole("option").first()).toBeVisible();
  await snap(page, "6a");
});

test("6b: import a pasted curl command, secrets lifted", async ({ page }) => {
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type("Import curl");
  await page.keyboard.press("Enter");
  // The design's command; the token is a stand-in.
  await page
    .getByLabel("Pasted input")
    .fill("curl -X POST https://api.shop.dev/carts/c_8f2a41/items \\\n  -H 'Authorization: Bearer visual-token-1234567' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\"sku\":\"TEA-EARL-250\",\"quantity\":2}'");
  await expect(page.getByLabel("Preview")).toContainText("Bearer {{token}}");
  await page.getByLabel("Save as").fill("orders/add-item.hurl");
  await expect(page.getByLabel("Preview").locator("..")).toContainText("orders/add-item.hurl");
  await snap(page, "6b");
});

test("6c: a variable's value and source on hover", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  // A capture: its value, the request and line that set it.
  const variable = page.locator(".cm-line", { hasText: "/carts/{{cart_id}}/items" }).first().locator("span", { hasText: /cart_id/ }).last();
  const box = (await variable.boundingBox())!;
  for (let i = 0; i < 6; i++) await page.mouse.move(box.x - 20 + i * 6, box.y + box.height / 2);
  await expect(page.locator(".cm-var-card")).toHaveCount(1);
  await expect(page.locator(".cm-var-foot")).toContainText("Set by request 3");
  await snap(page, "6c");
});

test("6d: variable autocomplete while typing {{ in a header", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await endOf(page, "Authorization: Bearer {{token}}");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-User: {{");
  await expect(page.locator(".cm-tooltip-autocomplete.cm-var-complete")).toBeVisible();
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
  // A match opens its request; the filter stays.
  await page.locator(".tree-row", { hasText: "/carts/{{cart_id}}" }).first().click();
  await expect(page.locator(".cm-content")).toBeVisible();
  await page.getByLabel("Filter requests in all files").focus();
  await snap(page, "8b");
});

test("8e: Copy as", async ({ page }) => {
  await open(page, "checkout.hurl");
  await run(page, /Failed/);
  await page.locator(".cm-line", { hasText: "POST {{base_url}}/carts" }).first().click();
  await page.getByRole("button", { name: "Copy as" }).click();
  await expect(page.getByRole("menuitem", { name: /^curl · this request/ })).toBeVisible();
  await expect(page.locator(".copy-preview").first()).not.toHaveText("…");
  await page.getByRole("menuitem", { name: /^curl · this request/ }).hover();
  await snap(page, "8e");
});

test("12a: Settings", async ({ page }) => {
  await panel(page, "Settings");
  await expect(page.getByRole("region", { name: "Appearance" })).toBeVisible();
  // A section shows its card alone, marked in the nav; General all of them.
  const nav = page.getByRole("navigation", { name: "Settings sections" });
  await nav.getByRole("button", { name: "Network" }).click();
  await expect(page.getByRole("region", { name: "Network" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Appearance" })).toHaveCount(0);
  await expect(nav.getByRole("button", { name: "Network" })).toHaveAttribute("aria-current", "true");
  await nav.getByRole("button", { name: "General" }).click();
  await expect(page.getByRole("region", { name: "Appearance" })).toBeVisible();
  // The theme picked, as the design shows it (then back to the system's).
  await page.getByRole("radio", { name: "Manual" }).click();
  await page.getByRole("combobox", { name: "Theme", exact: true }).selectOption("dark");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await snap(page, "12a");
  await page.getByRole("radio", { name: "Sync with system" }).click();
});

test("12c: the cookie jar", async ({ page }) => {
  await panel(page, "Settings");
  const keep = page.getByRole("switch", { name: "Keep cookies between runs" });
  await keep.click();
  await expect(keep).toBeChecked();
  try {
    await panel(page, "Files");
    await open(page, "checkout.hurl");
    await run(page, /Failed/);
    // From the run's Cookies tab, as the design opens it.
    await page.getByRole("region", { name: "Results" }).getByRole("tab", { name: /^Cookies/ }).click();
    await page.getByRole("button", { name: "Open cookie jar" }).click();
    const jar = page.getByRole("dialog", { name: "Cookie jar" });
    await expect(jar.getByRole("row", { name: /sid/ })).toBeVisible();
    // A cookie added, then its value being replaced inline (the design's).
    await jar.getByRole("button", { name: "+ Add cookie" }).click();
    await jar.getByLabel("New cookie name").fill("cart");
    await jar.getByLabel("New cookie value").fill("c_8f2a41");
    await jar.getByLabel("New cookie value").press("Enter");
    await expect(jar.getByRole("row", { name: /cart/ })).toBeVisible();
    await jar.getByRole("button", { name: "Edit cart" }).click();
    await jar.getByLabel("New value of cart").fill("c_8f2a41");
    await snap(page, "12c");
    await page.keyboard.press("Escape");
    await expect(jar.getByLabel("New value of cart")).toBeHidden();
    await jar.getByRole("button", { name: "Delete cart" }).click();
    await expect(jar.getByRole("row", { name: /cart/ })).toBeHidden();
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
  // The request shown: the first, as the design has it.
  await page.getByRole("list", { name: "History" }).getByRole("button").first().click();
  await expect(page.getByRole("list", { name: "History" }).getByRole("button").first()).toHaveAttribute("aria-pressed", "true");
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

test("4b: a data-driven run, a pill per row", async ({ page }) => {
  // The design's case: a user per row, the third with an invalid email.
  await open(page, "create-user.hurl");
  await page.getByRole("button", { name: /^Data file:/ }).click();
  await page.getByRole("menuitemradio", { name: "data/new-users.csv" }).click();
  await expect(page.getByRole("button", { name: "Data file: data/new-users.csv" })).toBeVisible();
  await run(page, /Failed/);
  await expect(page.getByRole("region", { name: "Results" }).getByRole("tablist", { name: "Data rows" }).getByRole("tab")).toHaveCount(3);
  await snap(page, "4b");
});

/** Reloads the page with a workspace the harness cannot reach itself
 * (see lib/mode.ts harnessFixture). */
async function withWorkspace(page: Page, fixture: object) {
  await page.evaluate((f) => sessionStorage.setItem("sonde.workspace", JSON.stringify(f)), fixture);
  await page.reload();
}

test("7a and 7b: no folder open, dark and light", async ({ page }) => {
  await withWorkspace(page, { noProject: true });
  await expect(page.getByRole("heading", { name: "The desktop app for .hurl files." })).toBeVisible();
  await snap(page, "7a");
  // The theme is a setting: light, then back.
  await page.evaluate(() => sessionStorage.removeItem("sonde.workspace"));
  await page.reload();
  await light(page);
  await withWorkspace(page, { noProject: true });
  await expect(page.getByRole("heading", { name: "The desktop app for .hurl files." })).toBeVisible();
  await snap(page, "7b");
  await page.evaluate(() => sessionStorage.removeItem("sonde.workspace"));
  await page.reload();
  await dark(page);
});

test("8d: the recent folders", async ({ page }) => {
  const hours = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();
  await withWorkspace(page, {
    recent: [
      // "@project": the folder open (its path is the harness's).
      { id: "1", name: "shop-api", dir: "@project", openedAt: hours(0) },
      { id: "2", name: "payments-api", dir: "~/code/payments-api", openedAt: hours(2) },
      { id: "3", name: "inventory-grpc", dir: "~/work/inventory", openedAt: hours(30) },
      { id: "4", name: "hurl-examples", dir: "~/src/hurl-examples", openedAt: hours(24 * 8) },
    ],
  });
  await page.getByRole("banner").getByRole("button", { name: /^shop-api/ }).click();
  await expect(page.getByRole("menu")).toContainText("Recent folders");
  await snap(page, "8d");
});
