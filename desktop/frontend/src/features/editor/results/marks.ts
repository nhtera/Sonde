// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What a run shows in the editor, by line: a gutter mark (▶ request, ✓, ✗,
// ◆ capture) and a ghost text at the line's end (status and time, a
// captured value, what a failed check got). Pure: the editor turns the
// marks into decorations.

import { failureOf, transportTitles } from "../../../components/run/model";
import type { FileRun } from "../../../state/run-model";

export type GutterKind = "run" | "pass" | "fail" | "capture" | "skip";

export interface LineMark {
  /** 1-based line. */
  line: number;
  gutter: GutterKind;
  /** The request's entry, on its request line (▶ runs to it). */
  entry?: number;
  ghost?: string;
  ghostKind?: "muted" | "fail" | "capture";
  /** From the run a Send reused: shown dimmed. */
  dim?: boolean;
}

/** The parts of an entry model (Go's editsvc.Model) the marks use. */
export interface EntryShape {
  Index: number;
  Range: { Start: number; End: number };
  Rows: { [section: string]: { Key: string; Range: { Start: number; End: number } }[] | null | undefined } | null;
}

const priority: Record<GutterKind, number> = { fail: 4, capture: 3, pass: 2, skip: 1, run: 0 };

const MAX_GHOST = 60;

function short(s: string): string {
  return s.length > MAX_GHOST ? s.slice(0, MAX_GHOST - 1) + "…" : s;
}

/** The ghost of a captured value (redacted values arrive as ***). */
function captureGhost(value: unknown): string {
  return "= " + short(typeof value === "string" ? JSON.stringify(value) : JSON.stringify(value) ?? String(value));
}

/** "at 14:32" (a clock time stays true while the marks stay). */
export function runTime(iso: string | null | undefined): string {
  if (!iso) return "earlier";
  const d = new Date(iso);
  return `at ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** ▶ on every request line (runs requests 1…n). lineOf turns a UTF-16
 * offset of the document into its line. */
export function requestMarks(model: EntryShape[], lineOf: (offset: number) => number): LineMark[] {
  return model.map((e) => ({ line: lineOf(e.Range.Start), gutter: "run" as const, entry: e.Index }));
}

/**
 * A run's results by line, for the document that ran: status and time on
 * request lines, ✓ and ✕ on checks, ◆ and the value on captures.
 */
export function runMarks(model: EntryShape[], lineOf: (offset: number) => number, run: FileRun): LineMark[] {
  const byLine = new Map<number, LineMark>();
  const put = (m: LineMark) => {
    const had = byLine.get(m.line);
    if (!had || priority[m.gutter] > priority[had.gutter]) byLine.set(m.line, { ...had, ...m, ghost: m.ghost ?? had?.ghost, ghostKind: m.ghost ? m.ghostKind : had?.ghostKind });
    else if (!had.ghost && m.ghost) byLine.set(m.line, { ...had, ghost: m.ghost, ghostKind: m.ghostKind });
  };
  const requestLine = new Map<number, number>();
  for (const e of model) requestLine.set(e.Index, lineOf(e.Range.Start));

  // After a Send, every entry but the one sent comes from the run it reused.
  const sent = run.kind === "send" && run.sent ? run.sent : 0;
  const from = `from the run ${runTime(run.summary?.baseRunAt)}`;
  for (const e of Object.values(run.entries)) {
    const dim = (sent > 0 && e.index !== sent) || undefined;
    const call = e.calls?.at(-1);
    const request = requestLine.get(e.index) ?? e.line;
    const status = call?.response.status;
    const timing = [status, `${e.time} ms`].filter((x) => x !== undefined).join(" · ");
    put({ line: request, gutter: "run", entry: e.index, ghost: dim ? `${timing} · ${from}` : timing, ghostKind: "muted", dim });
    for (const a of e.asserts ?? []) if (a.success) put({ line: a.line, gutter: "pass", dim });
    const shape = model.find((m) => m.Index === e.index);
    for (const c of e.captures ?? []) {
      const row = shape?.Rows?.captures?.find((r) => r.Key === c.name);
      if (row) put({ line: lineOf(row.Range.Start), gutter: "capture", ghost: captureGhost(c.value), ghostKind: "capture", dim });
    }
    for (const err of e.errors ?? []) {
      const f = failureOf({ ...e, errors: [err] });
      const why = (err.transport && transportTitles[err.transport]?.toLowerCase()) || err.description || err.message;
      const ghost = f?.actual !== undefined ? `✗ got ${short(f.actual)}` : `✗ ${short(why)}`;
      put({ line: err.line || request, gutter: "fail", ghost, ghostKind: "fail", dim });
    }
  }
  for (const [entry, reason] of Object.entries(run.skipped)) {
    const line = requestLine.get(Number(entry));
    if (line) put({ line, gutter: "skip", entry: Number(entry), ghost: reason === "option" ? "skipped" : `skipped · ${reason}`, ghostKind: "muted" });
  }
  return [...byLine.values()].sort((a, b) => a.line - b.line);
}
