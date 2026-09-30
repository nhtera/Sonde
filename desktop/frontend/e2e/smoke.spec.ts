// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, test } from "@playwright/test";

test("loads the app, calls a binding, receives an event", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("button", { name: "Search files and commands" })).toBeVisible();
  await page.waitForFunction(() => document.documentElement.dataset.harness === "ready");

  const pong = await page.evaluate(() => window.sondeHarness!.call("Ping", "e2e"));
  expect(pong).toBe("pong e2e");

  const event = await page.evaluate(async () => {
    const h = window.sondeHarness!;
    const next = h.nextEvent("harness:event");
    await h.call("Emit", "hello");
    return next;
  });
  expect(event).toBe("hello");
});

test("a binding call without the token is refused", async ({ request }) => {
  const res = await request.post("/wails/runtime", {
    headers: { Origin: new URL(test.info().project.use.baseURL!).origin },
    data: {},
  });
  expect(res.status()).toBe(401);
});
