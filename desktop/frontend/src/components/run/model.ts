// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What the run components show, built from a run's entries: one row per
// request of the file, and the first failure of an entry.

import type { Request } from "../../lib/api";
import type { Entry, EntryError, ReportRequest } from "../../lib/view";

export type RowState = "passed" | "failed" | "running" | "skipped" | "pending";

export interface RequestRow {
  entry: number;
  method: string;
  path: string;
  line: number;
  status?: number;
  ms?: number;
  state: RowState;
  /** Variables this request captured (→ name). */
  captured: string[];
  /** Variables this request uses (← name), when known. */
  used?: string[];
  skipReason?: string;
}

/** The parts of a run the rows are built from. */
export interface RunView {
  entries: Record<number, Entry>;
  sending: Record<number, ReportRequest>;
  skipped: Record<number, string>;
  running: boolean;
}

/** The path (and query) of url, or url without its leading {{variable}}. */
export function displayPath(url: string): string {
  try {
    const u = new URL(url);
    return u.pathname + u.search;
  } catch {
    return url.replace(/^\{\{[^}]*\}\}/, "") || url;
  }
}

/** A row per request of the file, with its run state. */
export function requestRows(requests: Request[], run: RunView | undefined): RequestRow[] {
  return requests.map((r) => {
    const e = run?.entries[r.entry];
    const sent = e?.calls?.at(-1);
    const row: RequestRow = {
      entry: r.entry,
      method: r.method,
      path: displayPath(sent?.request.url ?? run?.sending[r.entry]?.url ?? r.url),
      line: r.line,
      state: "pending",
      captured: (e?.captures ?? []).map((c) => c.name),
    };
    if (e) {
      row.state = e.success ? "passed" : "failed";
      row.status = sent?.response.status;
      row.ms = e.time;
    } else if (run?.skipped[r.entry]) {
      row.state = "skipped";
      row.skipReason = run.skipped[r.entry];
    } else if (run?.running && run.sending[r.entry]) {
      row.state = "running";
    }
    return row;
  });
}

export interface Failure {
  title: string;
  line: number;
  /** The failing line of the file, when the source is known. */
  code?: string;
  expected?: string;
  actual?: string;
  message: string;
}

const value = (m: string, key: string) => {
  const hit = new RegExp(`${key}:\\s+(?:(\\w+) )?<(.*)>`).exec(m);
  if (!hit) return undefined;
  return hit[1] === "string" ? JSON.stringify(hit[2]) : hit[2];
};

/** The first failure of entry, explained; lines is the file's text by line. */
export function failureOf(entry: Entry, lines?: string[]): Failure | null {
  const err: EntryError | undefined = entry.errors?.[0];
  if (!err) return null;
  const f: Failure = {
    title: err.kind === "assert-status" ? "Status failed" : err.assert ? "Assert failed" : err.description,
    line: err.line,
    code: lines?.[err.line - 1]?.trim(),
    message: err.message,
  };
  f.expected = value(err.message, "expected");
  f.actual = value(err.message, "actual");
  return f;
}
