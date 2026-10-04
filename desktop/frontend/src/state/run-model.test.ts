// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { RunItem } from "../lib/run-bridge";
import type { Summary } from "../lib/api";
import { applyData, shownRow, showRow, unitsOutcome, type DataRun, type FileRun } from "./run-model";

describe("run-model: data-driven runs", () => {
  function freshRun(): FileRun {
    return {
      runId: "r1",
      kind: "data",
      running: true,
      source: "GET /login\nGET /checkout",
      entries: {},
      sending: {},
      skipped: {},
      logs: [],
      messages: {},
      current: 0,
      last: 2,
      dropped: 0,
      summary: null,
      error: null,
    };
  }

  it("applyData: unitStarted events create data rows", () => {
    const run = freshRun();
    const items: RunItem[] = [
      { seq: 1, unit: 0, event: { type: "unitStarted", entry: 1, last: 2, row: 1, label: "Row 1" } as never },
      { seq: 2, unit: 1, event: { type: "unitStarted", entry: 1, last: 2, row: 2, label: "Row 2" } as never },
    ];
    const units = new Map<number, number>();
    const result = applyData(run, items, 0, units);

    expect(result.data).toBeDefined();
    expect(result.data!.rows).toHaveLength(2);
    expect(result.data!.rows[0].row).toBe(1);
    expect(result.data!.rows[0].label).toBe("Row 1");
    expect(result.data!.rows[1].row).toBe(2);
    expect(result.data!.rows[1].label).toBe("Row 2");
  });

  it("applyData: items go to their unit's row", () => {
    const run = freshRun();
    const items: RunItem[] = [
      { seq: 1, unit: 0, event: { type: "unitStarted", entry: 1, last: 2, row: 1 } as never },
      { seq: 2, unit: 1, event: { type: "unitStarted", entry: 1, last: 2, row: 2 } as never },
      { seq: 3, unit: 0, event: { type: "entryStarted", entry: 1, last: 2 } as never },
      { seq: 4, unit: 1, event: { type: "entryStarted", entry: 1, last: 2 } as never },
    ];
    const units = new Map<number, number>();
    const result = applyData(run, items, 0, units);

    // Row 1 (unit 0) has current: 1 from entryStarted
    expect(result.data!.rows[0].run.current).toBe(1);
    // Row 2 (unit 1) has current: 1 from entryStarted
    expect(result.data!.rows[1].run.current).toBe(1);
  });

  it("shownRow: picked row is always shown", () => {
    const data: DataRun = {
      file: "data.csv",
      rows: [
        { row: 1, run: { runId: "r1", kind: "data", running: false, source: "", entries: {}, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
        { row: 2, run: { runId: "r2", kind: "data", running: false, source: "", entries: { 1: { index: 1, line: 1, success: false, errors: [{ kind: "test" } as never] } as never }, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
      ],
      row: 2,
      picked: true,
    };

    expect(shownRow(data)).toBe(2);
  });

  it("shownRow: first failed row is shown when not picked", () => {
    const data: DataRun = {
      file: "data.csv",
      rows: [
        { row: 1, run: { runId: "r1", kind: "data", running: false, source: "", entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never }, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
        { row: 2, run: { runId: "r2", kind: "data", running: false, source: "", entries: { 1: { index: 1, line: 1, success: false, errors: [{ kind: "test" } as never] } as never }, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
        { row: 3, run: { runId: "r3", kind: "data", running: false, source: "", entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never }, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
      ],
      row: 1,
      picked: false,
    };

    expect(shownRow(data)).toBe(2);
  });

  it("shownRow: last row is shown when none failed", () => {
    const data: DataRun = {
      file: "data.csv",
      rows: [
        { row: 1, run: { runId: "r1", kind: "data", running: false, source: "", entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never }, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
        { row: 2, run: { runId: "r2", kind: "data", running: false, source: "", entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never }, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
      ],
      row: 1,
      picked: false,
    };

    expect(shownRow(data)).toBe(2);
  });

  it("showRow: displays the picked row's entries in the file run", () => {
    const run = freshRun();
    const data: DataRun = {
      file: "data.csv",
      rows: [
        { row: 1, run: { ...run, entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never } } },
        { row: 2, run: { ...run, entries: { 1: { index: 1, line: 1, success: false, errors: [{ kind: "test" } as never] } as never } } },
      ],
      row: 2,
      picked: false,
    };

    const result = showRow(run, data, 2);

    expect(result.entries[1]?.index).toBe(1);
    expect(result.data?.row).toBe(2);
    expect(result.data?.picked).toBe(false);
  });

  it("showRow: row not found returns data without merging entries", () => {
    const run = freshRun();
    const data: DataRun = {
      file: "data.csv",
      rows: [{ row: 1, run: { ...run, entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never } } }],
      row: 1,
      picked: false,
    };

    const result = showRow(run, data, 99);

    expect(result.entries).toEqual({});
    expect(result.data?.row).toBe(99);
  });
});

describe("unitsOutcome", () => {
  const u = (file: string, o: Partial<{ success: boolean; canceled: boolean; error: string }> = {}) =>
    ({ file, success: true, interrupted: false, canceled: false, requests: 1, durationMs: 1, ...o }) as NonNullable<Summary["units"]>[number];
  it("is the file's own, from its units or rows", () => {
    const units = [u("a.hurl"), u("a.hurl"), u("b.hurl", { success: false }), u("c.hurl", { canceled: true, success: false })];
    expect(unitsOutcome(units, "a.hurl")).toBe("passed");
    expect(unitsOutcome(units, "b.hurl")).toBe("failed");
    expect(unitsOutcome(units, "c.hurl")).toBe("canceled");
    expect(unitsOutcome([...units, u("a.hurl", { success: false })], "a.hurl")).toBe("failed");
    expect(unitsOutcome(units, "d.hurl")).toBeUndefined();
    expect(unitsOutcome([u("e.hurl", { success: false, error: "unreadable" })], "e.hurl")).toBe("error");
  });
});
