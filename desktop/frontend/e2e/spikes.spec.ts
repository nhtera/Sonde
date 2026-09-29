// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Spike measurements (desktop/README.md §Spikes). Run with E2E_SPIKES=1.

import { expect, test } from "@playwright/test";

test.skip(!process.env.E2E_SPIKES, "set E2E_SPIKES=1 to run the spikes");

test("cancel reaches the Go context", async ({ page }) => {
  await page.goto("/");
  await page.waitForFunction(() => document.documentElement.dataset.harness === "ready");
  const r = (await page.evaluate(() => window.sondeHarness!.spike("cancel"))) as { state: string; ms: number };
  console.log(`[${test.info().project.name}] cancel`, JSON.stringify(r));
  expect(r.state).toBe("canceled");
});

test("50 MB body by URL, worker parse, mid-stream cancel, sandbox", async ({ page }) => {
  await page.goto("/");
  await page.waitForFunction(() => document.documentElement.dataset.harness === "ready");
  const r = (await page.evaluate(() => window.sondeHarness!.spike("bodies"))) as {
    cancel: { served: number; total: number; aborted: boolean };
    sandbox: Record<"sandboxed" | "control", { iframe: boolean; popup: boolean }>;
  };
  console.log(`[${test.info().project.name}] bodies`, JSON.stringify(r));
  expect(r.cancel.aborted).toBe(true);
  expect(r.cancel.served).toBeLessThan(r.cancel.total);
  expect(r.sandbox.sandboxed).toMatchObject({ iframe: false, popup: false });
  expect(r.sandbox.control.popup).toBe(true); // the check can see a script run
});
