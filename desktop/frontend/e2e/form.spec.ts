// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Form view on the form-api project (its own harness): each edit lands
// in the text with every comment kept, options count, Text and Form share
// one undo history and cursor, UTF-16 columns, the gRPC picker, the login
// helper. Screens for the design approval go to E2E_CANDIDATES.

import { expect, test, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";

async function open(page: Page, file: string) {
  await page.goto("/");
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.locator(".cm-content")).toBeVisible();
}

const view = (page: Page, name: "Text" | "Form") => page.getByRole("group", { name: "Editor view" }).getByRole("button", { name }).click();
const tab = (page: Page, name: string) => page.getByRole("tablist", { name: "Request parts" }).getByRole("tab", { name: new RegExp(`^${name}`) }).click();
const request = (page: Page, n: number) => page.getByRole("tablist", { name: "Requests" }).getByRole("tab").nth(n - 1).click();

/** Commits a form field: fill, then Enter. */
async function set(page: Page, label: string, value: string) {
  const f = page.getByRole("textbox", { name: label, exact: true });
  await f.fill(value);
  await f.press("Enter");
}

/** The file's text, as the Text view holds it. */
async function text(page: Page) {
  await view(page, "Text");
  await expect(page.locator(".cm-content")).toBeVisible();
  const t = await page.locator(".cm-content").evaluate((el) => [...el.querySelectorAll(".cm-line")].map((l) => l.textContent).join("\n"));
  await view(page, "Form");
  return t;
}

const comments = ["# Orders, for the Form view's tests: comments must survive every edit.", "# Get an order", "# who asks", "# X-Debug: 1", "# the id first", "# Create a user"];

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test("form edits land in the text, and every comment stays", async ({ page }) => {
  await open(page, "orders.hurl");
  await view(page, "Form");
  await set(page, "URL", "{{base_url}}/orders/o-2");
  await tab(page, "Headers");
  await page.getByRole("checkbox", { name: "Disable Accept-Language" }).click();
  await expect(page.getByRole("checkbox", { name: "Enable Accept-Language" })).toBeVisible();
  await page.screenshot({ path: `${candidates}/form-headers-9e.png` });
  await request(page, 2);
  await tab(page, "Body");
  // The form's controls change once the file does: click, then wait.
  await page.getByRole("radio", { name: "x-www-form-urlencoded" }).click();
  await expect(page.getByRole("radio", { name: "x-www-form-urlencoded" })).toBeChecked();
  await expect(page.getByRole("textbox", { name: "Key 1", exact: true })).toHaveValue("field");
  await tab(page, "Asserts");
  await set(page, "Expected status", "400");
  const t = await text(page);
  expect(t).toContain("GET {{base_url}}/orders/o-2");
  expect(t).toContain("# Accept-Language: en-GB");
  // The JSON Content-Type went with the JSON body.
  expect(t).toContain("POST {{base_url}}/users\n[Form]\nfield: value\nHTTP 400");
  expect(t).not.toContain('{"name": "grace"}');
  for (const c of comments) expect(t).toContain(c);
});

test("options write their lines, and the tab counts them", async ({ page }) => {
  await open(page, "orders.hurl");
  await view(page, "Form");
  await tab(page, "Options");
  await set(page, "Max time", "30s");
  await page.getByRole("switch", { name: "Compressed" }).click();
  await expect(page.getByRole("switch", { name: "Compressed" })).toBeChecked();
  await page.getByRole("group", { name: "HTTP version" }).getByRole("button", { name: "2", exact: true }).click();
  await expect(page.getByRole("tablist", { name: "Request parts" }).getByRole("tab", { name: /^Options/ })).toHaveText("Options3");
  await expect(page.getByText("This request has 3 lines in [Options]")).toBeVisible();
  await page.screenshot({ path: `${candidates}/form-options-9f.png` });
  // Back to Auto and uncompressed: their lines go.
  await page.getByRole("group", { name: "HTTP version" }).getByRole("button", { name: "Auto" }).click();
  await page.getByRole("switch", { name: "Compressed" }).click();
  await expect(page.getByRole("switch", { name: "Compressed" })).not.toBeChecked();
  const t = await text(page);
  expect(t).toContain("[Options]\nmax-time: 30s\n");
  expect(t).not.toContain("http2");
  expect(t).not.toContain("compressed");
});

test("Text and Form share the undo history and the cursor", async ({ page }) => {
  await open(page, "orders.hurl");
  await page.locator(".cm-line", { hasText: "Accept-Language" }).click();
  await page.keyboard.press("End");
  const status = page.getByText(/^Ln \d+, Col \d+$/);
  const at = await status.textContent();
  await view(page, "Form");
  await set(page, "URL", "{{base_url}}/orders/o-3");
  await view(page, "Text");
  await expect(page.locator(".cm-line", { hasText: "/orders/o-3" })).toHaveCount(1);
  await page.locator(".cm-content").focus();
  await page.keyboard.press("ControlOrMeta+KeyZ");
  await expect(page.locator(".cm-line", { hasText: "GET {{base_url}}/orders/o-1" })).toHaveCount(1);
  // Switching views keeps the cursor where it was.
  await page.locator(".cm-line", { hasText: "Accept-Language" }).click();
  await page.keyboard.press("End");
  await view(page, "Form");
  await view(page, "Text");
  await expect(status).toHaveText(at!);
});

test("an edit next to emoji and CJK lands on the right columns", async ({ page }) => {
  await open(page, "unicode.hurl");
  await view(page, "Form");
  await tab(page, "Headers");
  await set(page, "Value 1", "🚀 lift-off 名前");
  await tab(page, "Params");
  await page.getByRole("button", { name: "+ Add parameter" }).click();
  await page.getByRole("textbox", { name: "New key" }).fill("lang");
  await page.getByRole("textbox", { name: "New value" }).fill("日本語");
  await page.getByRole("textbox", { name: "New value" }).press("Enter");
  await page.getByRole("tablist", { name: "Request parts" }).click();
  const t = await text(page);
  expect(t).toBe("# Café ☕ — non-ASCII lines, for UTF-16 offsets\nGET {{base_url}}/users/7?name=名前\nX-Émoji: 🚀 lift-off 名前\n[Query]\nlang: 日本語\nHTTP 200\n");
});

test("the gRPC picker lists the proto's methods; streaming clients are not offered", async ({ page }) => {
  await open(page, "inventory.sonde");
  await view(page, "Form");
  await page.getByRole("button", { name: "gRPC method" }).click();
  const methods = page.getByRole("menu");
  await expect(methods.getByRole("menuitemradio", { name: /WatchStock\s*server stream/ })).toBeEnabled();
  await expect(methods.getByRole("menuitemradio", { name: /ReserveStock\s*client stream · not supported/ })).toBeDisabled();
  await expect(methods.getByRole("menuitemradio", { name: /SyncStock\s*bidi stream · not supported/ })).toBeDisabled();
  await page.screenshot({ path: `${candidates}/form-grpc-11c.png` });
  await methods.getByRole("menuitemradio", { name: /ListStock/ }).click();
  await expect(page.getByRole("textbox", { name: "URL", exact: true })).toHaveValue("http://127.0.0.1:50051/inventory.v1.Inventory/ListStock");
  expect(await text(page)).toContain("POST http://127.0.0.1:50051/inventory.v1.Inventory/ListStock\n[SondeGrpc]\nproto: protos/inventory.proto");
});

test("the login helper adds a request that captures the token", async ({ page }) => {
  await open(page, "orders.hurl");
  await view(page, "Form");
  await tab(page, "Auth");
  await page.screenshot({ path: `${candidates}/form-auth-9c.png` });
  await page.getByRole("button", { name: "+ Add login request" }).click();
  await expect(page.getByRole("tablist", { name: "Requests" }).getByRole("tab")).toHaveCount(3);
  const t = await text(page);
  const login = t.indexOf("POST {{token_url}}");
  expect(login).toBeGreaterThan(0);
  expect(login).toBeLessThan(t.indexOf("GET {{base_url}}/orders/"));
  expect(t).toContain('token: jsonpath "$.access_token" redact');
});

test("candidate screenshots: params, asserts, form-data, urlencoded, GraphQL", async ({ page }) => {
  await open(page, "orders.hurl");
  await page.keyboard.press("ControlOrMeta+KeyR");
  await expect(page.getByRole("region", { name: "Results" }).locator(".outcome")).toHaveText(/Passed|Failed/);
  await view(page, "Form");
  await tab(page, "Params");
  await page.screenshot({ path: `${candidates}/form-params-9d.png` });
  await tab(page, "Asserts");
  await page.screenshot({ path: `${candidates}/form-asserts-2a.png` });
  await request(page, 2);
  await tab(page, "Body");
  await page.getByRole("radio", { name: "form-data" }).click();
  await expect(page.getByRole("radio", { name: "form-data" })).toBeChecked();
  await page.getByRole("group", { name: "field type" }).getByRole("button", { name: "File" }).click();
  await page.screenshot({ path: `${candidates}/form-formdata-9a.png` });
  await page.getByRole("radio", { name: "x-www-form-urlencoded" }).click();
  await expect(page.getByRole("radio", { name: "x-www-form-urlencoded" })).toBeChecked();
  await page.screenshot({ path: `${candidates}/form-urlencoded-9b.png` });
  await page.getByRole("radio", { name: "GraphQL" }).click();
  await expect(page.getByLabel("GraphQL query")).toBeVisible();
  await page.screenshot({ path: `${candidates}/form-graphql-2c.png` });
});
