// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Server mode's own sign-in, in a real browser: the launch page is opened
// as a file (as --open does), its cross-site navigation trades the nonce
// for the SameSite=Strict cookie, and the page's runtime calls carry the
// token. Runs against the server build (E2E_SERVER_BIN).

import { type ChildProcess, spawn } from "node:child_process";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
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
  await expect(page.getByRole("button", { name: "Search files and commands" })).toBeVisible();
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

/** Signs in through the launch file; returns the server's origin and the
 * launch page's URL (with its nonce). */
async function signIn(page: import("@playwright/test").Page, port: number) {
  const launch = await start(port);
  const nonceURL = /https?:\/\/[^"'<\s]+/.exec(readFileSync(launch, "utf8"))![0];
  const origin = `http://127.0.0.1:${port}`;
  await page.goto(pathToFileURL(launch).href);
  await page.waitForURL(`${origin}/`);
  return { origin, nonceURL };
}

test("the launch nonce works once", async ({ page, browser }, info) => {
  const { nonceURL } = await signIn(page, basePort + 20 + info.parallelIndex);
  const other = await browser.newContext();
  const res = await other.request.get(nonceURL, { maxRedirects: 0 });
  expect([401, 403]).toContain(res.status());
  await other.close();
});

test("requests from elsewhere are refused: Host, Origin, framing, bodies", async ({ page, browser }, info) => {
  const port = basePort + 30 + info.parallelIndex;
  const { origin } = await signIn(page, port);
  // A name other than the loopback address (DNS rebinding).
  const host = await page.request.get(`${origin}/`, { headers: { Host: `evil.test:${port}` } });
  expect(host.status()).toBe(403);
  // A call without the page's Origin, the cookie sent (refused: the
  // Origin rule alone is checked by the Go tests, TestOriginChecked).
  const noOrigin = await page.request.post(`${origin}/wails/runtime`, { data: {} });
  expect([401, 403]).toContain(noOrigin.status());
  // The app may not be framed by another site.
  const app = await page.request.get(`${origin}/`);
  expect(app.headers()["x-frame-options"]).toBe("DENY");
  expect(app.headers()["content-security-policy"]).toContain("frame-ancestors 'none'");
  // A stored body without the session cookie.
  const fresh = await browser.newContext();
  const body = await fresh.request.get(`${origin}/_sonde/body/0123456789abcdef`);
  expect(body.status()).toBe(401);
  await fresh.close();
});

test("a page on another port can not call the app", async ({ page }, info) => {
  const port = basePort + 40 + info.parallelIndex;
  const { origin } = await signIn(page, port);
  // A page on another loopback port: the same site, so the SameSite=Strict
  // cookie goes with its calls; it has no token, and CORS keeps it from
  // reading an answer.
  const other = createServer((_, res) => res.end("<!doctype html><title>elsewhere</title>"));
  await new Promise<void>((r) => other.listen(0, "127.0.0.1", r));
  const otherURL = `http://127.0.0.1:${(other.address() as AddressInfo).port}/`;
  try {
    const tab = await page.context().newPage();
    await tab.goto(otherURL);
    const outcome = await tab.evaluate(async (o) => {
      try {
        const r = await fetch(`${o}/wails/runtime`, { method: "POST", credentials: "include", body: "{}" });
        return `read ${r.status}`;
      } catch {
        return "blocked";
      }
    }, origin);
    expect(outcome).toBe("blocked");
    // The same call as the server gets it: the session cookie, that page's
    // Origin, no token: refused.
    const call = await page.context().request.post(`${origin}/wails/runtime`, { headers: { Origin: otherURL.slice(0, -1) }, data: {} });
    expect([401, 403]).toContain(call.status());
    await tab.close();
  } finally {
    other.close();
  }
});
