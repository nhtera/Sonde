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

export interface FileRun {
  runId: string;
  kind: "run" | "send";
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
