// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Server mode's own sign-in, in a real browser: the launch page is opened
// as a file (as --open does), its cross-site navigation trades the nonce
// for the SameSite=Strict cookie, and the page's runtime calls carry the
// token. Runs against the server build (E2E_SERVER_BIN).

import { type ChildProcess, spawn } from "node:child_process";
import { mkdtempSync, rmSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { expect, test } from "@playwright/test";

const bin = process.env.E2E_SERVER_BIN ?? "../bin/sonde-desktop-server";
const basePort = Number(process.env.E2E_SERVER_PORT) || 34300;

let proc: ChildProcess | undefined;
let data = "";

test.afterEach(() => {
  proc?.kill();
  if (data) rmSync(data, { recursive: true, force: true });
});

/** Starts the server and returns the launch file path it prints. */
async function start(port: number): Promise<string> {
  data = mkdtempSync(join(tmpdir(), "sonde-server-e2e-"));
  proc = spawn(bin, ["--root", "e2e/fixture", "--port", String(port), "--data", data], {
    stdio: ["pipe", "pipe", "ignore"],
  });
  return await new Promise<string>((resolve, reject) => {
    let out = "";
    const timer = setTimeout(() => reject(new Error(`no launch link in: ${out}`)), 20_000);
    proc!.stdout!.on("data", (chunk: Buffer) => {
      out += chunk.toString();
      const m = /Open (.+\.html) in a browser/.exec(out);
      if (m) {
        clearTimeout(timer);
        resolve(m[1]);
      }
    });
    proc!.on("exit", (code) => reject(new Error(`server exited (${code}): ${out}`)));
  });
}

test("launch file, cookie and token, runtime call", async ({ page }, info) => {
  const port = basePort + info.parallelIndex;
  const launch = await start(port);
  const origin = `http://127.0.0.1:${port}`;

  await page.goto(pathToFileURL(launch).href);
  await page.waitForURL(`${origin}/`);
  await expect(page.getByRole("heading", { name: "Sonde" })).toBeVisible();
  expect(existsSync(launch)).toBe(false); // used once, then removed

  const result = await page.evaluate(async () => {
    const call = await fetch("/wails/runtime", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ object: 0, method: 0, args: {} }),
    });
    return { status: call.status, cookie: document.cookie };
  });
  expect(result.status).not.toBe(401);
  expect(result.status).not.toBe(403);
  expect(result.cookie).not.toContain("sonde_session"); // HttpOnly

  // Without the page's token the same call is refused, cookie or not.
  const bare = await page.request.post(`${origin}/wails/runtime`, {
    headers: { Origin: origin },
    data: {},
  });
  expect(bare.status()).toBe(401);
});

test("a fresh browser without the link is refused", async ({ browser }, info) => {
  const port = basePort + 10 + info.parallelIndex;
  await start(port);
  const context = await browser.newContext();
  const page = await context.newPage();
  const res = await page.goto(`http://127.0.0.1:${port}/`);
  expect(res?.status()).toBe(401);
  await context.close();
});
