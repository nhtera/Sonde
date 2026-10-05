// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Updates, against the harness's update service: the window app's own
// Service on scripted parts (internal/update/harness.go). One test in
// steps: the service keeps its state from one to the next.

import { expect, test, type Page } from "@playwright/test";

const harness = (page: Page, method: string, ...args: unknown[]) =>
  page.evaluate(([m, a]) => window.sondeHarness!.call(m as string, ...(a as unknown[])), [method, args] as const);

async function checkFromPalette(page: Page) {
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type("Check for updates");
  await page.keyboard.press("Enter");
}

test("updates: verify, offer, skip, install, restart", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  // The harness shows the window app's updates only when asked.
  await page.addInitScript(() => sessionStorage.setItem("sonde.updates", "1"));
  await page.goto("/");
  await page.waitForFunction(() => document.documentElement.dataset.harness === "ready");
  const bar = page.locator(".statusbar");
  const dialog = page.getByRole("dialog");

  await test.step("the page reaches nothing itself: its CSP is unchanged", async () => {
    const csp = await page.evaluate(() => document.querySelector('meta[http-equiv="Content-Security-Policy"]')?.getAttribute("content"));
    expect(csp).toBe("default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; frame-src 'self'; connect-src 'self'");
  });

  await test.step("a background check that does not verify warns in the status bar", async () => {
    await harness(page, "UpdateScenario", "verification");
    await harness(page, "UpdateBackgroundCheck");
    await expect(bar.getByRole("button", { name: "Update not verified" })).toBeVisible();
    await expect(dialog).toHaveCount(0);
    await bar.getByRole("button", { name: "Update not verified" }).click();
    await expect(dialog.getByText("The update could not be verified and was not installed.")).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Download" })).toBeVisible();
    await page.keyboard.press("Escape");
  });

  await test.step("a manual check offline gets an answer", async () => {
    await harness(page, "UpdateScenario", "offline");
    await checkFromPalette(page);
    await expect(dialog.getByText("Could not reach GitHub.")).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Download" })).toBeVisible();
    await page.keyboard.press("Escape");
  });

  await test.step("a copy that cannot replace itself offers the release page", async () => {
    await harness(page, "UpdateScenario", "cannot-install");
    await checkFromPalette(page);
    await expect(dialog.getByRole("heading", { name: "Sonde Desktop 0.2.1 is available" })).toBeVisible();
    await expect(dialog.getByText(/installed by another user/)).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Install" })).toHaveCount(0);
    await expect(dialog.getByRole("button", { name: "Download" })).toBeVisible();
    await page.keyboard.press("Escape");
  });

  await test.step("skip, then a manual check offers it again", async () => {
    await harness(page, "UpdateScenario", "available");
    await checkFromPalette(page);
    await expect(dialog.getByText(/<b>Not bold<\/b>/)).toBeVisible(); // the notes are text
    await dialog.getByRole("button", { name: "Skip this version" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(bar.getByRole("button", { name: /^Update / })).toHaveCount(0);
    await checkFromPalette(page);
    await expect(dialog.getByRole("heading", { name: "Sonde Desktop 0.2.1 (skipped) is available" })).toBeVisible();
  });

  await test.step("install: progress, then Restart to update", async () => {
    await dialog.getByRole("button", { name: "Install anyway" }).click();
    await expect(dialog.getByText(/Downloading… \d+%|Verifying the download…/)).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "Sonde Desktop 0.2.1 is ready to install" })).toBeVisible();
    // The modal hides the page from assistive tech: the bar after it.
    await page.keyboard.press("Escape");
    await expect(bar.getByRole("button", { name: "Restart to update" })).toBeVisible();
  });

  await test.step("other windows open: Restart waits for them", async () => {
    await harness(page, "UpdateScenario", "others");
    await checkFromPalette(page);
    await expect(dialog.getByText("Quit the other Sonde windows first (2 open).")).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Restart to update" })).toBeDisabled();
    await harness(page, "UpdateScenario", "available");
    await dialog.getByRole("button", { name: "Check again" }).click();
    await expect(dialog.getByRole("button", { name: "Restart to update" })).toBeEnabled();
    await page.keyboard.press("Escape");
  });

  await test.step("restart asks about an unsaved tab first", async () => {
    await page.locator(".tree-row", { hasText: "users.hurl" }).first().click();
    await page.locator(".cm-content").click();
    await page.keyboard.press("ControlOrMeta+End");
    await page.keyboard.type("\n# unsaved");
    await bar.getByRole("button", { name: "Restart to update" }).click();
    const ask = page.getByRole("dialog", { name: "Unsaved changes" });
    await expect(ask.getByText("users.hurl has unsaved changes. Restart without saving?")).toBeVisible();
    await ask.getByRole("button", { name: "Cancel" }).click();
    expect(await harness(page, "UpdateInstalls")).toBe(0);
    await bar.getByRole("button", { name: "Restart to update" }).click();
    await page.getByRole("dialog", { name: "Unsaved changes" }).getByRole("button", { name: "Restart without saving" }).click();
    await expect.poll(() => harness(page, "UpdateInstalls")).toBe(1);
  });
});
