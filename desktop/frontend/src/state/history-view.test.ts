// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A run opened from the history: its entries' outcomes, and when it ends.

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { HistoryRecord } from "../lib/api";

vi.mock("../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), Runs: { Run: vi.fn(), Send: vi.fn(), Cancel: vi.fn() } }));

const { historyRun, useHistoryView } = await import("./history-view");
const { useRuns } = await import("./run");

const record = (success: boolean, asserts: boolean[]): HistoryRecord =>
  ({
    id: "h1",
    results: [
      {
        filename: "/p/a.hurl",
        success,
        entries: [
          { index: 1, asserts: [{ success: true }] },
          { index: 2, asserts: asserts.map((ok) => ({ success: ok })) },
        ],
      },
    ],
  }) as unknown as HistoryRecord;

describe("historyRun", () => {
  it("fails the last entry of a failed file whose asserts all passed", () => {
    const run = historyRun(record(false, [true]), "a.hurl")!;
    expect(run.entries[1].success).toBe(true);
    expect(run.entries[2].success).toBe(false);
  });

  it("keeps a passed file's entries passed", () => {
    const run = historyRun(record(true, [true]), "a.hurl")!;
    expect(run.entries[2].success).toBe(true);
  });

  it("fails only the entry whose assert failed", () => {
    const run = historyRun(record(false, [false]), "a.hurl")!;
    expect([run.entries[1].success, run.entries[2].success]).toEqual([true, false]);
  });
});

describe("the history view", () => {
  beforeEach(() => useHistoryView.getState().show({ id: "h1", file: "a.hurl", at: "", run: historyRun(record(true, [true]), "a.hurl")! }));

  it("ends when its file runs again", () => {
    useRuns.setState((s) => ({ runs: { ...s.runs, "b.hurl": { ...useHistoryView.getState().view!.run, running: true } } }));
    expect(useHistoryView.getState().view).not.toBeNull();
    useRuns.setState((s) => ({ runs: { ...s.runs, "a.hurl": { ...useHistoryView.getState().view!.run, running: true } } }));
    expect(useHistoryView.getState().view).toBeNull();
  });
});
