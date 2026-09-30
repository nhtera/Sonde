// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The tree on a project of 1000 request files (playwright.config.ts
// generates it): scrolling runs no task over 50 ms, and the filter answers
// in under 100 ms.

import { expect, test } from "@playwright/test";

test("scrolls 1000 files without long tasks", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  const tree = page.getByRole("tree", { name: "Project files" });
  await expect(tree).toContainText("request01.hurl");
  await page.evaluate(() => {
    const w = window as unknown as { longTasks: number[] };
    w.longTasks = [];
    new PerformanceObserver((list) => list.getEntries().forEach((e) => w.longTasks.push(e.duration))).observe({ type: "longtask" });
  });
  await tree.hover();
  for (let i = 0; i < 60; i++) {
    await page.mouse.wheel(0, 900);
    await page.waitForTimeout(16);
  }
  // At the bottom: the last folder's last file.
  await expect(tree).toContainText("request50.hurl");
  expect(await tree.evaluate((el) => el.scrollTop + el.clientHeight >= el.scrollHeight - 2)).toBe(true);
  const long = await page.evaluate(() => (window as unknown as { longTasks: number[] }).longTasks);
  expect(long.filter((d) => d > 50), `long tasks: ${long.join(", ")} ms`).toEqual([]);
});

test("filters 1000 files in under 100 ms", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("request01.hurl");
  const filter = page.getByLabel("Filter requests in all files");
  // Measured in the page: from the input event to the painted result.
  const ms = await filter.evaluate(async (input: HTMLInputElement) => {
    const start = performance.now();
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
    setter.call(input, "items/07/");
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise<void>((resolve) => {
      const check = () => (document.body.textContent?.includes("50 requests in 50 files") ? resolve() : requestAnimationFrame(check));
      check();
    });
    return performance.now() - start;
  });
  expect(ms).toBeLessThan(100);
});
