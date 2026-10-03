// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The tree on a project of 1000 request files (playwright.config.ts
// generates it): scrolling runs no task over 50 ms, and the filter answers
// in under 100 ms. The editor's diagnostics, on the shell's harness, come
// under 150 ms after the idle that follows typing.

import { expect, test } from "@playwright/test";

import { shellURL } from "../playwright.config";

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

test("types in a 5,000-line file: each keystroke's work well under a frame (p95)", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  // Opened from the palette: the tree lists 1,000 files before it.
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("folder01");
  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type("big.hurl");
  await page.keyboard.press("Enter");
  await expect(page.locator(".cm-content")).toBeVisible();
  // The middle of the file.
  await page.keyboard.press("ControlOrMeta+KeyG");
  await page.getByLabel("Line number").fill("2500");
  await page.keyboard.press("Enter");
  await expect(page.locator(".statusbar")).toContainText("Ln 2500, Col 1");
  // Event Timing, per keystroke (its keydown, keypress and input entries
  // share a start): the app's work (processing), and the time until the
  // next paint (rounded to 8 ms, frame-bound: 16 at 60 Hz). The paint time
  // also carries the headless browser's frame scheduling; the native check
  // is in the release checks.
  await page.evaluate(() => {
    const w = window as unknown as { keys: Map<number, { work: number; paint: number }> };
    w.keys = new Map();
    new PerformanceObserver((list) => {
      for (const e of list.getEntries() as PerformanceEventTiming[]) {
        if (!["keydown", "keypress", "beforeinput", "input", "keyup"].includes(e.name)) continue;
        const k = Math.round(e.startTime);
        const had = w.keys.get(k) ?? { work: 0, paint: 0 };
        w.keys.set(k, { work: had.work + (e.processingEnd - e.processingStart), paint: Math.max(had.paint, e.duration) });
      }
    }).observe({ type: "event", durationThreshold: 16, buffered: false } as PerformanceObserverInit);
  });
  const typed = "X-Trace: {{trace_id}} typed in the middle of a big file, twice as long as a line";
  await page.keyboard.type(typed, { delay: 40 });
  await page.waitForTimeout(300);
  const keys = await page.evaluate(() => [...(window as unknown as { keys: Map<number, { work: number; paint: number }> }).keys.values()]);
  // Only keystrokes over 16 ms are reported; the rest are faster.
  const work = [...keys.map((k) => k.work), ...Array(Math.max(0, typed.length - keys.length)).fill(0)].sort((a, b) => a - b);
  const p95 = work[Math.floor(work.length * 0.95)];
  const overFrame = keys.filter((k) => k.paint > 16).length;
  console.log(`keystrokes: ${typed.length}; work p95 ${p95.toFixed(1)} ms; painted after more than a frame: ${overFrame}`);
  expect(p95).toBeLessThan(16);
});

test("reports diagnostics soon after typing stops", async ({ page }) => {
  await page.goto(`${shellURL}/`);
  await page.locator(".tree-row", { hasText: "users.hurl" }).first().click();
  // The language server has the file once ▸ marks show.
  await expect(page.locator(".cm-run-run").first()).toBeVisible();
  const times: number[] = [];
  for (let i = 0; i < 10; i++) {
    await page.locator(".cm-line", { hasText: "POST {{base_url}}/users" }).first().click();
    await page.keyboard.press("End");
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
