// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Text editor on the shop-api project (the shell's harness): results in
// the gutter and as ghost text, Send at the cursor, the stale banner, the
// language server's warnings and hover.

import { expect, test, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";

async function open(page: Page, file: string) {
  await page.goto("/");
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.locator(".cm-content")).toBeVisible();
  // The language server has the file once ▸ marks show (Go's model).
  await expect(page.locator(".cm-run-run").first()).toBeVisible();
}

/** Puts the cursor at the end of the first line holding text. */
async function endOf(page: Page, text: string) {
  await page.locator(".cm-line", { hasText: text }).first().click();
  await page.keyboard.press("End");
}

async function runFile(page: Page) {
  await page.keyboard.press("ControlOrMeta+KeyR");
  await expect(page.getByRole("region", { name: "Results" }).locator(".outcome")).toHaveText(/Passed|Failed/);
}

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test("shows a run's results in the gutter and as ghost text", async ({ page }) => {
  await open(page, "checkout.hurl");
  await runFile(page);
  await expect(page.locator(".cm-run-fail")).toHaveCount(1);
  await expect(page.locator(".cm-line.cm-ghost-fail")).toHaveAttribute("data-ghost", '✗ got "pending"');
  await expect(page.locator(".cm-line.cm-ghost-capture").first()).toHaveAttribute("data-ghost", '= "***"'); // the redacted token
  await expect(page.locator(".cm-run-pass").first()).toBeVisible();
});

test("Send at the cursor reuses the run: earlier entries dim", async ({ page }) => {
  await open(page, "checkout.hurl");
  await runFile(page);
  await endOf(page, "/checkout");
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(page.locator(".cm-line.cm-ghost-dim").first()).toHaveAttribute("data-ghost", /from the run at \d\d:\d\d$/);
  // The sent request itself is not dimmed.
  await expect(page.locator(".cm-line", { hasText: "/checkout" })).not.toHaveClass(/cm-ghost-dim/);
});

test("an edit makes the results stale: banner, and Send is refused", async ({ page }) => {
  await open(page, "checkout.hurl");
  await runFile(page);
  await endOf(page, '"password"');
  await page.keyboard.type(" ");
  await expect(page.getByText(/Edited since this run · line 3/)).toBeVisible();
  await endOf(page, "/checkout");
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(page.getByRole("button", { name: "Run 1–5" })).toBeVisible();
  // Nothing ran: the results and the banner stay the earlier run's.
  await expect(page.getByText(/Edited since this run · line 3/)).toBeVisible();
  await expect(page.locator(".cm-line.cm-ghost-fail")).toHaveCount(1);
});

test("flags undefined variables, a capture of another file's run too", async ({ page }) => {
  // Runs are per file: checkout.hurl's cart_id is not users.hurl's.
  await open(page, "checkout.hurl");
  await runFile(page);
  await page.locator(".tree-row", { hasText: "users.hurl" }).first().click();
  await expect(page.locator(".cm-run-run").first()).toBeVisible();
  await endOf(page, "POST {{base_url}}/users");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-Typo: {{typo}}");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-Cart: {{cart_id}}");
  await expect(page.locator(".cm-ghost-warn", { hasText: '"typo"' })).toBeVisible();
  await expect(page.locator(".cm-ghost-warn", { hasText: '"cart_id"' })).toBeVisible();
  // A variable of the environment is defined.
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-User: {{user}}");
  await page.waitForTimeout(600);
  await expect(page.locator(".cm-ghost-warn", { hasText: '"user"' })).toHaveCount(0);
});

test("reports diagnostics soon after typing stops", async ({ page }) => {
  await open(page, "users.hurl");
  const times: number[] = [];
  for (let i = 0; i < 10; i++) {
    await endOf(page, "POST {{base_url}}/users");
    await page.keyboard.press("Enter");
    await page.keyboard.type(`X-${i}: {{missing${i}}}`);
    const start = Date.now();
    await expect(page.locator(".cm-ghost-warn", { hasText: `missing${i}` })).toBeVisible();
    times.push(Date.now() - start);
  }
  times.sort((a, b) => a - b);
  // Typing stops, then 200 ms of idle before the edit is sent; the rest is
  // the server's answer (and the polling of this test).
  const p95 = times[Math.ceil(times.length * 0.95) - 1] - 200;
  console.log(`diagnostics after the last keystroke: ${times.join(", ")} ms; after idle p95 ${p95} ms`);
  expect(p95).toBeLessThan(150);
});

test("one hover for a variable, with its redacted value", async ({ page }) => {
  await open(page, "checkout.hurl");
  await runFile(page);
  const variable = page.locator(".cm-line", { hasText: "Authorization: Bearer {{token}}" }).first().locator("span", { hasText: /token/ }).last();
  const box = (await variable.boundingBox())!;
  // In from the left, like a pointer (the hover waits for it to rest).
  for (let i = 0; i < 6; i++) await page.mouse.move(box.x - 20 + i * 6, box.y + box.height / 2);
  const hover = page.locator(".cm-sonde-hover");
  await expect(hover).toHaveCount(1);
  await expect(hover.locator(".cm-var-value")).toHaveText("***");
  await expect(hover.locator(".cm-var-foot")).toContainText(/Set by request 1 · line \d+ · last run/);
  await expect(page.locator(".cm-tooltip")).toHaveCount(1);
});

test("a cursor after emoji maps to the right request", async ({ page }) => {
  await open(page, "checkout.hurl");
  await endOf(page, "POST {{base_url}}/carts");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-Emoji: 🎉🎉🎉🎉🎉🎉");
  await page.keyboard.press("ControlOrMeta+Shift+Enter");
  const rows = page.getByRole("region", { name: "Results" }).getByRole("list", { name: "Requests" }).getByRole("listitem");
  await expect(rows.nth(2)).toHaveAccessibleName(/passed$/);
  await expect(rows.nth(3)).toHaveAccessibleName(/pending$/);
});

test("the app's keys act once in the editor: ⌘/ comments, ⌘↵ adds no line", async ({ page }) => {
  await open(page, "users.hurl");
  const lines = await page.locator(".cm-line").count();
  await endOf(page, "HTTP 422");
  await page.keyboard.press("ControlOrMeta+Slash");
  await expect(page.locator(".cm-line", { hasText: "HTTP 422" }).first()).toHaveText("# HTTP 422");
  await page.keyboard.press("ControlOrMeta+Slash");
  await expect(page.locator(".cm-line", { hasText: "HTTP 422" }).first()).toHaveText("HTTP 422");
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(page.getByRole("region", { name: "Results" }).locator(".outcome")).toHaveText(/Passed|Failed/);
  expect(await page.locator(".cm-line").count()).toBe(lines);
});

test("closing a tab with unsaved edits asks in the app", async ({ page }) => {
  await open(page, "users.hurl");
  await endOf(page, "HTTP 422");
  await page.keyboard.type(" ");
  const tab = page.getByRole("tab", { name: /users\.hurl/ });
  await tab.hover();
  await page.getByRole("button", { name: "Close users.hurl" }).click();
  const dialog = page.getByRole("dialog", { name: "Unsaved changes" });
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(tab).toBeVisible();
  await tab.hover();
  await page.getByRole("button", { name: "Close users.hurl" }).click();
  await dialog.getByRole("button", { name: "Close without saving" }).click();
  await expect(tab).toHaveCount(0);
});

test("picks a data file and runs once per row", async ({ page }) => {
  await open(page, "data-login.hurl");
  await page.getByRole("button", { name: "Data file: none" }).click();
  await page.getByRole("menuitemradio", { name: "data/logins.csv" }).click();
  await expect(page.getByRole("button", { name: "Data file: data/logins.csv" })).toBeVisible();
  await runFile(page);
  const results = page.getByRole("region", { name: "Results" });
  const pills = results.getByRole("tablist", { name: "Data rows" }).getByRole("tab");
  await expect(pills).toHaveCount(2);
  await expect(results).toContainText("Ran all 2 rows of logins.csv");
  // The password column is a secret: its value never shows.
  await expect(pills.first()).toHaveText(/Row 1 · ada/);
  await pills.nth(1).click();
  await expect(pills.nth(1)).toHaveAttribute("aria-selected", "true");
  await results.getByRole("tab", { name: /^Request/ }).click();
  await expect(results.getByRole("tabpanel")).not.toContainText("fixture-password");
});

test("variable completion lists earlier captures with their line", async ({ page }) => {
  await open(page, "checkout.hurl");
  await runFile(page);
  await endOf(page, "Authorization: Bearer {{token}}");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-User: {{");
  const list = page.locator(".cm-tooltip-autocomplete.cm-var-complete");
  await expect(list).toBeVisible();
  // The capture of request 1 first, its value from the run; a redacted
  // one as ***.
  await expect(list.locator("li").first()).toContainText("user_id");
  await expect(list.locator("li", { hasText: "token" }).locator(".cm-var-option-source")).toHaveText("capture · line 6");
  await expect(list.locator("li", { hasText: "token" }).locator(".cm-var-option-value")).toHaveText("***");
  // Picking one closes the template.
  await expect(list.locator("li[aria-selected=true]")).toContainText("user_id");
  // A list takes Enter 75 ms after it opens (no pick by a fast typist).
  await page.waitForTimeout(100);
  await page.keyboard.press("Enter");
  await expect(page.locator(".cm-line", { hasText: "X-User:" })).toHaveText("X-User: {{user_id}}");
});

test("candidate screenshot: the editor after a run (1a)", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await open(page, "checkout.hurl");
  await runFile(page);
  await endOf(page, "Authorization: Bearer {{token}}");
  await page.keyboard.press("Enter");
  await page.keyboard.type("X-Client: {{name}}");
  await expect(page.locator(".cm-ghost-warn")).toBeVisible();
  await page.screenshot({ path: `${candidates}/editor-1a-dark.png` });
});
