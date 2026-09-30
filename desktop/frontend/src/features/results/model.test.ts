// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { Entry } from "../../lib/view";
import { ago, bodyKind, cardOf, formatBytes, formatMs, sentCookies, statusText, stopText, tabCounts, waterfall } from "./model";

describe("results model", () => {
  it("formats the meta line", () => {
    expect(statusText(200)).toBe("200 OK");
    expect(statusText(101)).toBe("101 Switching Protocols");
    expect(statusText(299)).toBe("299");
    expect(formatBytes(193)).toBe("193 B");
    expect(formatBytes(3174)).toBe("3.1 kB");
    expect(formatBytes(48 << 20)).toBe("48.0 MB");
    expect(formatMs(28)).toBe("28 ms");
    expect(formatMs(1840)).toBe("1.84 s");
    const now = new Date("2026-10-01T10:00:00Z");
    expect(ago(new Date("2026-10-01T09:59:58Z"), now)).toBe("just now");
    expect(ago(new Date("2026-10-01T09:59:48Z"), now)).toBe("12s ago");
    expect(ago(new Date("2026-10-01T09:57:00Z"), now)).toBe("3m ago");
  });

  it("picks a body view from the content type", () => {
    expect(bodyKind("application/json; charset=utf-8")).toBe("json");
    expect(bodyKind("application/problem+json")).toBe("json");
    expect(bodyKind("application/x-ndjson")).toBe("text");
    expect(bodyKind("text/html")).toBe("html");
    expect(bodyKind("image/svg+xml")).toBe("image");
    expect(bodyKind("application/atom+xml")).toBe("xml");
    expect(bodyKind("application/pdf")).toBe("pdf");
    expect(bodyKind("application/octet-stream")).toBe("binary");
    expect(bodyKind("")).toBe("text");
  });

  it("lays out the waterfall from microseconds", () => {
    const t = { name_lookup: 400, connect: 1300, app_connect: 0, pre_transfer: 1300, start_transfer: 25400, total: 28000 };
    const { segments, total } = waterfall(t, false);
    expect(total).toBe(28);
    expect(segments.map((s) => [s.name, +s.ms.toFixed(1), s.skipped])).toEqual([
      ["DNS", 0.4, undefined],
      ["Connect", 0.9, undefined],
      ["TLS", 0, "http"],
      ["Waiting", 24.1, undefined],
      ["Download", 2.6, undefined],
    ]);
    expect(waterfall({ ...t, app_connect: 5000, pre_transfer: 5000 }, true).segments[2]).toMatchObject({ name: "TLS", ms: 3.7 });
  });

  it("handles zero timings and redirects", () => {
    // All zeros: should not crash
    const t = { name_lookup: 0, connect: 0, app_connect: 0, pre_transfer: 0, start_transfer: 0, total: 0 };
    const { segments, total } = waterfall(t, false);
    expect(total).toBe(0);
    expect(segments.every((s) => s.ms === 0)).toBe(true);

    // Redirect case: pre_transfer may exceed app_connect
    const redirect = { name_lookup: 100, connect: 1000, app_connect: 0, pre_transfer: 2000, start_transfer: 3000, total: 4000 };
    const { segments: rsegs } = waterfall(redirect, false);
    expect(rsegs[2].skipped).toBe("http");
    expect(rsegs[3].ms).toBeGreaterThan(0); // Waiting should be positive
  });

  it("preserves timing phase order even with negative/jumbled values", () => {
    // Negative microsecond values should be treated as 0
    const t = { name_lookup: -100, connect: 500, app_connect: -50, pre_transfer: 600, start_transfer: 1000, total: 1500 };
    const { segments } = waterfall(t, false);
    // All negative values become 0, so DNS phase should be 0
    expect(segments[0].ms).toBe(0);
    expect(segments[0].start).toBe(0);
  });

  it("says why a stream stopped", () => {
    const m = { data: "", direction: "received", time: 0 };
    expect(stopText({ protocol: "sse", stop_reason: "count", messages: Array(6).fill(m), received: 6, sent: 0 })).toBe("count 6 reached");
    expect(stopText({ protocol: "websocket", stop_reason: "script", messages: [], received: 4, sent: 1 })).toBe("last step done");
    expect(stopText({ protocol: "sse", messages: null, received: 0, sent: 0 })).toBe("the stream failed");
  });

  it("has a card per transport failure", () => {
    const err = (transport?: string) => ({ line: 1, column: 1, kind: "http", assert: false, description: "", message: "", transport });
    expect(cardOf(err("connect"))?.code).toBe("ECONNREFUSED");
    expect(cardOf(err("resolve"))?.code).toBe("DNS");
    expect(cardOf(err("tls"))?.code).toBe("TLS");
    expect(cardOf(err("timeout"))?.code).toBe("TIMEOUT");
    expect(cardOf(err("host-denied"))?.code).toBe("HOST");
    expect(cardOf(err("canceled"))?.neutral).toBe(true);
    expect(cardOf(err(undefined))).toBeNull();
    expect(cardOf(undefined)).toBeNull();
  });

  it("finds the request that set each cookie sent", () => {
    const entry = (index: number, sent: string[], set: string[]) =>
      ({
        index,
        calls: [{ request: { cookies: sent.map((name) => ({ name, value: "v" })) }, response: { cookies: set.map((name) => ({ name, value: "v" })) } }],
      }) as unknown as Entry;
    const entries = { 1: entry(1, [], ["sid"]), 2: entry(2, ["sid"], ["cart"]), 3: entry(3, ["sid", "cart", "jar"], []) };
    expect(sentCookies(entries, 3).map((c) => [c.name, c.setBy])).toEqual([
      ["sid", 1],
      ["cart", 2],
      ["jar", 0],
    ]);
  });

  it("handles cookies set and sent in same request", () => {
    const entry = (index: number, sent: string[], set: string[]) =>
      ({
        index,
        calls: [{ request: { cookies: sent.map((name) => ({ name, value: "v" })) }, response: { cookies: set.map((name) => ({ name, value: "v" })) } }],
      }) as unknown as Entry;
    // Request 2 sets "auth" and sends it in the same request (should find it in request 2 itself)
    const entries = { 1: entry(1, [], []), 2: entry(2, ["auth"], ["auth"]) };
    const result = sentCookies(entries, 2);
    // "auth" was sent by request 2, but set in request 2 itself (or later), so setBy should be 0
    expect(result.find((c) => c.name === "auth")?.setBy).toBe(0);
  });

  it("handles no cookies and missing entries", () => {
    const entry = (index: number) =>
      ({
        index,
        calls: [{ request: { cookies: [] }, response: { cookies: [] } }],
      }) as unknown as Entry;
    const entries = { 1: entry(1), 2: entry(2) };
    expect(sentCookies(entries, 2)).toEqual([]);
    expect(sentCookies({}, 1)).toEqual([]);
  });

  it("finds the most recent request that set a cookie when multiple did", () => {
    const entry = (index: number, sent: string[], set: string[]) =>
      ({
        index,
        calls: [{ request: { cookies: sent.map((name) => ({ name, value: "v" })) }, response: { cookies: set.map((name) => ({ name, value: "v" })) } }],
      }) as unknown as Entry;
    // Request 1 sets "x", request 2 overwrites it
    const entries = {
      1: entry(1, [], ["x"]),
      2: entry(2, ["x"], ["x"]),
      3: entry(3, ["x"], []),
    };
    const result = sentCookies(entries, 3);
    // Should find the most recent setter (request 2, not request 1)
    expect(result.find((c) => c.name === "x")?.setBy).toBe(2);
  });

  it("counts the tabs", () => {
    const e = {
      index: 1,
      asserts: [{ line: 3, success: true }, { line: 4, success: false }],
      captures: [{ name: "a", value: 1 }],
      calls: [{ request: { cookies: [{ name: "sid", value: "x" }] }, response: { headers: [{ name: "a", value: "b" }], cookies: [] } }],
      sonde: { contract: { violations: [{ kind: "k", message: "m" }, { kind: "k", message: "m", warning: true }] } },
    } as unknown as Entry;
    expect(tabCounts(e)).toEqual({ headers: 1, asserts: { total: 4, failed: 2 }, captures: 1, cookies: 1, stream: 0 });
  });
});
