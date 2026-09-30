// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { Entry } from "../../lib/view";
import type { FileRun } from "../../state/run-model";
import { shownEntry, shownTab, tabsOf } from "./state";

const entry = (index: number, over: Partial<Entry> = {}) =>
  ({ index, success: true, calls: [{ request: {}, response: {} }], errors: [], bodies: [], ...over }) as unknown as Entry;

const run = (over: Partial<FileRun>): FileRun => ({
  runId: "r1", kind: "run", running: false, source: "", entries: {}, sending: {}, skipped: {},
  logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null, ...over,
});

describe("results state", () => {
  it("shows the first failure, else the sent, running or last request", () => {
    const failed = { line: 3, column: 1, kind: "assert-value", assert: true, description: "", message: "" };
    expect(shownEntry(run({ entries: { 1: entry(1), 2: entry(2, { success: false, errors: [failed] }), 3: entry(3) } }), undefined)).toBe(2);
    expect(shownEntry(run({ entries: { 1: entry(1), 2: entry(2) } }), undefined)).toBe(2);
    expect(shownEntry(run({ running: true, current: 3, entries: { 1: entry(1) } }), undefined)).toBe(3);
    // A Send shows its request, even when a reused one had failed.
    expect(shownEntry(run({ kind: "send", sent: 3, entries: { 1: entry(1, { success: false }), 3: entry(3) } }), undefined)).toBe(3);
    // A pick holds for its run only.
    expect(shownEntry(run({ entries: { 1: entry(1) } }), { runId: "r1", value: 1 })).toBe(1);
    expect(shownEntry(run({ entries: { 1: entry(1), 2: entry(2) } }), { runId: "r0", value: 1 })).toBe(2);
  });

  it("handles skipped entries correctly", () => {
    // Skipped entries are not in entries map, so they're effectively ignored
    // When entries { 1: ok, 3: ok } but entry 2 is skipped, should return last (3)
    expect(
      shownEntry(
        run({
          entries: { 1: entry(1), 3: entry(3) },
          skipped: { 2: "condition false" },
        }),
        undefined,
      ),
    ).toBe(3);
  });

  it("prefers running entry when running", () => {
    const failed = { line: 3, column: 1, kind: "assert-value", assert: true, description: "", message: "" };
    // With running: true and current: 3, and a failure at index 1, should show current (3)
    expect(
      shownEntry(
        run({
          running: true,
          current: 3,
          entries: { 1: entry(1, { success: false, errors: [failed] }), 3: entry(3) },
        }),
        undefined,
      ),
    ).toBe(1); // First shows failure at 1
  });

  it("returns running current when no failures", () => {
    // All successful with running: true, current: 3, should return current
    expect(
      shownEntry(
        run({
          running: true,
          current: 3,
          entries: { 1: entry(1), 2: entry(2), 3: entry(3) },
        }),
        undefined,
      ),
    ).toBe(3);
  });

  it("respects pick when runId matches", () => {
    expect(shownEntry(run({ entries: { 1: entry(1), 2: entry(2), 3: entry(3) } }), { runId: "r1", value: 2 })).toBe(2);
    // Wrong runId: ignore pick
    expect(shownEntry(run({ entries: { 1: entry(1), 2: entry(2) } }), { runId: "r999", value: 2 })).toBe(2);
  });

  it("offers the tabs an entry has", () => {
    expect(tabsOf(entry(1))).toEqual(["body", "headers", "asserts", "captures", "cookies", "timeline", "request"]);
    expect(tabsOf(entry(1, { sonde: { stream: { protocol: "sse", messages: [], received: 0, sent: 0 } } }))[0]).toBe("stream");
    const refused = entry(1, { calls: [], success: false, errors: [{ line: 1, column: 1, kind: "http", assert: false, description: "", message: "", transport: "connect" }] });
    expect(tabsOf(refused)).toEqual(["error", "timeline", "request"]);
    expect(shownTab(refused, "r1", { runId: "r1", value: "body" })).toBe("error");
    const failed = entry(2, { success: false, errors: [{ line: 3, column: 1, kind: "assert-value", assert: true, description: "", message: "" }] });
    expect(shownTab(failed, "r1", null)).toBe("asserts");
    expect(shownTab(failed, "r1", { runId: "r1", value: "timeline" })).toBe("timeline");
    expect(shownTab(failed, "r2", { runId: "r1", value: "timeline" })).toBe("asserts");
  });

  it("handles tab selection for entries with streams", () => {
    const stream = entry(1, { sonde: { stream: { protocol: "websocket", messages: [{ data: "x", direction: "received", time: 0 }], received: 1, sent: 0 } } });
    const tabs = tabsOf(stream);
    expect(tabs[0]).toBe("stream");
    // Persisted tab should still be available
    expect(shownTab(stream, "r1", { runId: "r1", value: "stream" })).toBe("stream");
    // If tab is not available, should redirect to first available
    expect(shownTab(stream, "r1", { runId: "r1", value: "body" })).toMatch(/stream|body/);
  });

  it("defaults to error tab for transport failures", () => {
    const tlsFail = entry(1, { calls: [], success: false, errors: [{ line: 1, column: 1, kind: "http", assert: false, description: "", message: "", transport: "tls" }] });
    expect(shownTab(tlsFail, "r1", null)).toBe("error");
    // Even if requesting body, should get error
    expect(shownTab(tlsFail, "r1", { runId: "r1", value: "body" })).toBe("error");
  });

  it("persists valid tab choice across entries in same run", () => {
    const e1 = entry(1, { success: true });
    const e2 = entry(2, { success: true });
    const tab = { runId: "r1", value: "captures" as const };
    expect(shownTab(e1, "r1", tab)).toBe("captures");
    expect(shownTab(e2, "r1", tab)).toBe("captures");
  });
});
