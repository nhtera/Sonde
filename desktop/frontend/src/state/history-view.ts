// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A run from the history, shown read-only in the Results panel in place of
// the file's last run until closed, or until the file runs again.

import { create } from "zustand";
import type { HistoryRecord } from "../lib/api";
import type { Entry } from "../lib/view";
import { on } from "../lib/events";
import type { FileRun } from "./run-model";
import { useRuns } from "./run";

export interface HistoryView {
  id: string;
  file: string;
  /** When it ran. */
  at: string;
  run: FileRun;
  /** The request opened (its entry), shown first. */
  entry?: number;
}

export const useHistoryView = create<{ view: HistoryView | null; show(v: HistoryView | null): void }>((set) => ({
  view: null,
  show: (view) => set({ view }),
}));

/** The run of file in a history record, as the Results panel shows a run
 * (its entries as the report kept them, redacted; no bodies). */
export function historyRun(record: HistoryRecord, file: string): FileRun | null {
  const res = record.results?.find((r) => r.filename === file || r.filename.endsWith(`/${file}`));
  if (!res) return null;
  const entries: Record<number, Entry> = {};
  for (const e of res.entries ?? []) {
    entries[e.index] = { ...e, bodies: [], errors: [], success: (e.asserts ?? []).every((a) => a.success), retried: false } as Entry;
  }
  // The record keeps asserts, not errors: a file that failed with every
  // assert passing stopped at an error in its last entry.
  const list = Object.values(entries);
  if (!res.success && list.length > 0 && list.every((e) => e.success)) {
    const last = Math.max(...list.map((e) => e.index));
    entries[last] = { ...entries[last], success: false };
  }
  return {
    runId: `history:${record.id}`, kind: "run", running: false, source: "", entries, sending: {}, skipped: {},
    logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: record.summary ?? null, error: null,
  };
}

// A new run of the file, or another project, ends the history view.
useRuns.subscribe((s, prev) => {
  const view = useHistoryView.getState().view;
  if (view && s.runs[view.file] !== prev.runs[view.file] && s.runs[view.file]?.running) useHistoryView.getState().show(null);
});
on("ws:opened", () => useHistoryView.getState().show(null));
