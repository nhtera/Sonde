// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A file's run as its events build it (no store: the run components and
// their tests use it too).

import type { Summary } from "../lib/api";
import type { RunItem } from "../lib/run-bridge";
import type { Entry, ReportRequest, RunEvent, StreamMessage } from "../lib/view";

export interface LogLine {
  entry: number;
  level: string;
  text: string;
}

/** One row of a data-driven run: its number, what names it, its run. */
export interface DataRowRun {
  row: number;
  label?: string;
  run: FileRun;
}

/** A data-driven run: the data file and each row's run; the file's run
 * shows the picked row (the first failed one, until one is picked). */
export interface DataRun {
  file: string;
  rows: DataRowRun[];
  /** The row shown. */
  row: number;
  /** Whether the user picked it (a later failure does not move it). */
  picked: boolean;
}

export interface FileRun {
  runId: string;
  kind: "run" | "send" | "data";
  /** For a Send, the entry it sent (the entries before it come from the
   * run it reused). */
  sent?: number;
  running: boolean;
  /** The text that ran, to tell when the file was edited since. */
  source: string;
  /** Finished entries by 1-based index (the last attempt). */
  entries: Record<number, Entry>;
  /** Requests in flight (sent, no response yet), by entry. */
  sending: Record<number, ReportRequest>;
  skipped: Record<number, string>;
  logs: LogLine[];
  messages: Record<number, StreamMessage[]>;
  /** The entry that ran last (for progress). */
  current: number;
  last: number;
  dropped: number;
  summary: Summary | null;
  error: string | null;
  data?: DataRun;
  /** A file of a test run: its outcome there, from its units (it has no
   * summary of its own). */
  outcome?: "passed" | "failed" | "canceled" | "error" | "interrupted";
}

/** A test run file's outcome from its units (rows, for a data run);
 * undefined when it has none. */
export function unitsOutcome(units: Summary["units"], file: string): FileRun["outcome"] {
  const mine = (units ?? []).filter((u) => u.file === file);
  if (mine.length === 0) return undefined;
  if (mine.some((u) => u.canceled)) return "canceled";
  if (mine.some((u) => u.interrupted)) return "interrupted";
  if (mine.some((u) => u.error || u.parseError)) return "error";
  return mine.every((u) => u.success) ? "passed" : "failed";
}

/** Whether a data row failed: a request did, or the row could not run. */
export const rowFailed = (r: FileRun) => !!r.error || Object.values(r.entries).some((e) => !e.success);

/** The row a data run shows: the picked one, else the first that failed,
 * else the last that started. */
export function shownRow(d: DataRun): number {
  if (d.picked) return d.row;
  return d.rows.find((r) => rowFailed(r.run))?.row ?? d.rows.at(-1)?.row ?? 0;
}

/** Applies a data run's items (in sequence order): unitStarted opens a
 * row, the others go to their unit's row; the file's run shows one row. */
export function applyData(r: FileRun, items: RunItem[], dropped: number, units: Map<number, number>): FileRun {
  const data: DataRun = r.data ? { ...r.data, rows: r.data.rows.slice() } : { file: "", rows: [], row: 0, picked: false };
  const byRow = new Map<number, RunItem[]>();
  for (const it of items) {
    const ev = it.event as RunEvent;
    if (ev.type === "unitStarted") {
      const row = ev.row ?? units.size + 1;
      units.set(it.unit, row);
      data.rows.push({ row, label: ev.label, run: { ...r, data: undefined, entries: {}, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } });
      continue;
    }
    const row = units.get(it.unit);
    if (row === undefined) continue;
    const list = byRow.get(row);
    if (list) list.push(it);
    else byRow.set(row, [it]);
  }
  for (const [row, its] of byRow) {
    const i = data.rows.findIndex((x) => x.row === row);
    if (i >= 0) data.rows[i] = { ...data.rows[i], run: apply(data.rows[i].run, its, 0) };
  }
  if (items.some((it) => (it.event as RunEvent).type === "unitStarted")) data.rows.sort((a, b) => a.row - b.row);
  return showRow({ ...r, dropped: r.dropped + dropped }, data, shownRow(data));
}

/** The file's run showing a data run's row. */
export function showRow(r: FileRun, data: DataRun, row: number): FileRun {
  const shown = data.rows.find((x) => x.row === row)?.run;
  const d = { ...data, row };
  if (!shown) return { ...r, data: d };
  return { ...r, entries: shown.entries, sending: shown.sending, skipped: shown.skipped, logs: shown.logs, messages: shown.messages, current: shown.current, last: shown.last, data: d };
}

/** Applies run items (in sequence order) to a file's run. */
export function apply(r: FileRun, items: RunItem[], dropped: number): FileRun {
  const next: FileRun = {
    ...r,
    entries: { ...r.entries },
    sending: { ...r.sending },
    skipped: { ...r.skipped },
    logs: r.logs.slice(),
    messages: { ...r.messages },
    dropped: r.dropped + dropped,
  };
  for (const it of items) {
    const ev = it.event as RunEvent;
    switch (ev.type) {
      case "entryStarted":
        next.current = ev.entry;
        next.last = ev.last;
        break;
      case "requestSent":
        next.sending[ev.entry] = ev.request;
        break;
      case "entrySkipped":
        next.skipped[ev.entry] = ev.reason;
        break;
      case "log":
        next.logs.push({ entry: ev.entry, level: ev.level, text: ev.text });
        break;
      case "message":
        next.messages[ev.entry] = [...(next.messages[ev.entry] ?? []), ev.message];
        break;
      case "entryFinished":
        next.entries[ev.entry.index] = ev.entry;
        delete next.sending[ev.entry.index];
        break;
    }
  }
  return next;
}
