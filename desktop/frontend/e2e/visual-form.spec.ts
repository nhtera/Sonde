// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Design screens of the Form view, on form-api (see e2e/visual/snap.ts).
// Nothing is saved: each screen starts from the files as they are.

import { expect, test, type Page } from "@playwright/test";
import { dark, home, light, open, run, snap } from "./visual/snap";

test.describe.configure({ mode: "serial" });

const view = (page: Page, name: "Text" | "Form") => page.getByRole("group", { name: "Editor view" }).getByRole("button", { name }).click();
const tab = (page: Page, name: string) => page.getByRole("tablist", { name: "Request parts" }).getByRole("tab", { name: new RegExp(`^${name}`) }).click();
const request = (page: Page, n: number) => page.getByRole("tablist", { name: "Requests" }).getByRole("tab").nth(n - 1).click();

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await home(page);
  await dark(page);
  await open(page, "orders.hurl");
});

test("2a and 2b: asserts as rows, a failing one explained", async ({ page }) => {
  await run(page, /Passed|Failed/);
  await view(page, "Form");
  await tab(page, "Asserts");
  await snap(page, "2a");
  await light(page);
  await snap(page, "2b");
});

test("9d, 9e, 9c, 9f: params, headers, auth, options", async ({ page }) => {
  await view(page, "Form");
  await tab(page, "Params");
  await snap(page, "9d");
  await tab(page, "Headers");
  await snap(page, "9e");
  await tab(page, "Auth");
  await snap(page, "9c");
  await tab(page, "Options");
  await snap(page, "9f");
});

test("9g: the Send split button", async ({ page }) => {
  await view(page, "Form");
  await request(page, 2);
  await page.getByRole("button", { name: "More ways to run" }).click();
  await expect(page.getByRole("menu")).toBeVisible();
  await snap(page, "9g");
});

test("9a, 9b, 2c: form-data, urlencoded, GraphQL bodies", async ({ page }) => {
  await view(page, "Form");
  await request(page, 2);
  await tab(page, "Body");
  await page.getByRole("radio", { name: "form-data" }).click();
  await page.getByRole("group", { name: "field type" }).getByRole("button", { name: "File" }).click();
  await snap(page, "9a");
  await page.getByRole("radio", { name: "x-www-form-urlencoded" }).click();
  await expect(page.getByRole("radio", { name: "x-www-form-urlencoded" })).toBeChecked();
  await snap(page, "9b");
  await page.getByRole("radio", { name: "GraphQL" }).click();
  await expect(page.getByLabel("GraphQL query")).toBeVisible();
  await snap(page, "2c");
  // Leave the file as it was.
  for (let i = 0; i < 3; i++) await page.keyboard.press("ControlOrMeta+KeyZ");
});

test("11c: a gRPC request from the .proto", async ({ page }) => {
  await open(page, "inventory.sonde");
  await view(page, "Form");
  await expect(page.getByRole("combobox", { name: "gRPC method" })).toBeVisible();
  await snap(page, "11c");
});
