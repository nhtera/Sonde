// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The app shell on the shop-api project (its own harness, see
// playwright.config.ts), against the fixture API. Candidate screenshots
// for the design approval go to E2E_CANDIDATES (default e2e-candidates/).

import { expect, test, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";

async function open(page: Page, file = "checkout.hurl") {
  await page.goto("/");
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.getByRole("tab", { name: new RegExp(file) })).toHaveAttribute("aria-selected", "true");
}

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test("opens the project and a file", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator(".titlebar")).toContainText("shop-api");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("checkout.hurl");
  await open(page);
  await expect(page.getByRole("navigation", { name: "Path" })).toHaveText("checkout.hurl");
  await expect(page.locator(".cm-content")).toContainText("POST {{base_url}}/login");
  // Requests are listed under the open file.
  await expect(page.locator(".tree-row.req")).toHaveCount(5);
});

test("a request in the tree opens its file at the request", async ({ page }) => {
  await open(page);
  // POST /carts/{{cart_id}}/items, on line 22 of checkout.hurl.
  await page.locator(".tree-row.req").nth(3).click();
  await expect(page.locator(".statusbar")).toContainText("Ln 22, Col 1");
});

test("filters requests across files", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("checkout.hurl");
  await page.keyboard.press("ControlOrMeta+Shift+KeyF");
  const filter = page.getByLabel("Filter requests in all files");
  await expect(filter).toBeFocused();
  await filter.fill("carts");
  await expect(page.getByText(/^3 requests in 1 file ·/)).toBeVisible();
  await expect(page.locator(".tree-row.req .hl").first()).toHaveText("carts");
  await filter.press("Escape");
  await expect(filter).toHaveValue("");
});

test("runs a file and shows every result as it arrives", async ({ page }) => {
  await open(page);
  // The run starts from the keyboard; its events are subscribed to before
  // the call, so a run that ends at once still shows every request.
  await page.keyboard.press("ControlOrMeta+KeyR");
  const results = page.getByRole("region", { name: "Results" });
  await expect(results.locator(".outcome")).toHaveText("Failed");
  const rows = results.getByRole("list", { name: "Requests" }).getByRole("listitem");
  await expect(rows).toHaveCount(5);
  await expect(rows.nth(0)).toHaveAccessibleName("POST /login passed");
  await expect(rows.nth(4)).toHaveAccessibleName(/checkout failed$/);
  await expect(page.locator(".statusbar")).toContainText("✗ 1");
});

test("switches the theme and keeps it", async ({ page }) => {
  await page.goto("/");
  const html = page.locator("html");
  await expect(html).toHaveAttribute("data-theme", /light|dark/);
  const before = await html.getAttribute("data-theme");
  await page.getByRole("button", { name: "Toggle theme" }).click();
  const after = before === "dark" ? "light" : "dark";
  await expect(html).toHaveAttribute("data-theme", after);
  await page.reload();
  await expect(html).toHaveAttribute("data-theme", after);
});

test("the palette opens files, and > lists commands only", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("users.hurl");
  await page.keyboard.press("ControlOrMeta+KeyK");
  const palette = page.getByRole("dialog");
  await page.keyboard.type("users");
  await expect(palette.getByRole("option", { name: /users\.hurl/ })).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("tab", { name: /users\.hurl/ })).toHaveAttribute("aria-selected", "true");

  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type(">");
  await expect(palette.getByRole("option", { name: /\.hurl/ })).toHaveCount(0);
  await page.keyboard.type("keyboard");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
});

test("rebinds a shortcut and refuses a conflict", async ({ page }) => {
  await open(page, "users.hurl");
  await page.getByRole("button", { name: "? Shortcuts" }).click();
  const sheet = page.getByRole("dialog", { name: "Keyboard shortcuts" });
  const rebind = sheet.getByRole("button", { name: "Rebind Run file" });

  // Keys another command uses are refused and reported.
  await rebind.click();
  await page.keyboard.press("ControlOrMeta+KeyK");
  await expect(sheet.getByRole("alert")).toContainText("is used by Search files and commands");

  await page.keyboard.press("Alt+Shift+KeyR");
  await expect(rebind).toContainText(/R/);
  await sheet.getByRole("button", { name: "Done" }).click();

  await page.keyboard.press("Alt+Shift+KeyR");
  await expect(page.getByRole("region", { name: "Results" }).locator(".outcome")).toHaveText(/Passed|Failed/);

  // Back to the default.
  await page.getByRole("button", { name: "? Shortcuts" }).click();
  await sheet.locator("li", { hasText: "Run file" }).getByRole("button", { name: "Reset" }).click();
  await expect(rebind).not.toContainText("Alt");
});

test("candidate screenshots: dark, light and 1024px", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await open(page);
  await page.keyboard.press("ControlOrMeta+KeyR");
  await expect(page.getByRole("region", { name: "Results" }).locator(".outcome")).toHaveText("Failed");
  const theme = (t: string) => page.evaluate((t) => document.documentElement.setAttribute("data-theme", t), t);
  await theme("dark");
  await page.screenshot({ path: `${candidates}/shell-dark.png` });
  await theme("light");
  await page.screenshot({ path: `${candidates}/shell-light.png` });
  await theme("dark");
  await page.setViewportSize({ width: 1024, height: 768 });
  await expect(page.locator(".window")).toHaveAttribute("data-narrow", "yes");
  await page.screenshot({ path: `${candidates}/shell-1024.png` });
  await page.getByRole("button", { name: "Results" }).click();
  await expect(page.getByRole("region", { name: "Results" })).toBeHidden();
});
