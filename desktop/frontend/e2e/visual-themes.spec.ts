// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Each built-in theme on the main screen after a failed run (see
// e2e/visual/snap.ts): its baseline is reviewed against the theme's own
// screenshots.

import { expect, test } from "@playwright/test";
import { themes } from "../src/app/theme/themes";
import { home, open, run, snap } from "./visual/snap";

test.describe.configure({ mode: "serial" });

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await home(page);
});

for (const t of themes) {
  test(`theme: ${t.label}`, async ({ page }) => {
    await page.waitForFunction(() => !!window.sondeHarness);
    await page.evaluate((id) => window.sondeHarness!.setTheme({ theme: id, dayTheme: "light", nightTheme: "dark" }), t.id);
    await expect(page.locator("html")).toHaveAttribute("data-theme", t.id);
    await open(page, "users.hurl");
    await run(page, /Failed/);
    // The failing request: its assert, in the theme's colors.
    await page.getByRole("region", { name: "Results" }).locator(".req-row").nth(1).click();
    await page.mouse.move(720, 450);
    await snap(page, `theme-${t.id}`);
  });
}
