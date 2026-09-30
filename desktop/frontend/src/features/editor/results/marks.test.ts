// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import type { RunItem } from "../../../lib/run-bridge";
import { apply, type FileRun } from "../../../state/run-model";
import checkout from "../../../components/run/testdata/checkout.json";
import { requestMarks, runTime, runMarks, type EntryShape } from "./marks";

const source = readFileSync(fileURLToPath(new URL("../../../../../testdata/shop-api/checkout.hurl", import.meta.url)), "utf8");
const lineStarts = [0];
for (let i = 0; i < source.length; i++) if (source[i] === "\n") lineStarts.push(i + 1);
const lineOf = (offset: number) => {
  let line = 0;
  while (line + 1 < lineStarts.length && lineStarts[line + 1] <= offset) line++;
  return line + 1;
};

/** A model like Go's (editsvc.Model): request lines and capture rows. */
function modelOf(src: string): EntryShape[] {
  const out: EntryShape[] = [];
  let section = "";
  src.split("\n").forEach((text, i) => {
    const start = lineStarts[i];
    if (/^[A-Z]+ /.test(text) && !text.startsWith("HTTP")) {
      out.push({ Index: out.length + 1, Range: { Start: start, End: start + text.length }, Rows: {} });
      section = "";
    } else if (/^\[(\w+)\]/.test(text)) section = text.slice(1, -1).toLowerCase();
    else if (section === "captures" && /^\w+:/.test(text)) {
      const rows = out.at(-1)!.Rows!;
      (rows.captures ??= []).push({ Key: text.split(":")[0], Range: { Start: start, End: start + text.length } });
    }
  });
  return out;
}

function replay(kind: "run" | "send" = "run", sent?: number): FileRun {
  let r: FileRun = {
    runId: "checkout", kind, sent, running: false, source, entries: {}, sending: {}, skipped: {},
    logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null,
  };
  for (const e of checkout as { data: { items?: RunItem[]; summary?: never } }[]) {
    if (e.data.items) r = apply(r, e.data.items, 0);
    if (e.data.summary) r = { ...r, summary: e.data.summary };
  }
  return r;
}

const model = modelOf(source);

describe("run marks", () => {
  it("puts ▶ on every request line", () => {
    expect(requestMarks(model, lineOf).map((m) => [m.line, m.entry])).toEqual([
      [2, 1],
      [10, 2],
      [15, 3],
      [22, 4],
      [28, 5],
    ]);
  });

  const marks = runMarks(model, lineOf, replay());
  const at = (line: number) => marks.find((m) => m.line === line);

  it("shows status and time on request lines", () => {
    expect(at(2)?.ghost).toMatch(/^200 · \d+ ms$/);
    expect(at(2)?.entry).toBe(1);
  });

  it("marks captures with their value, a redacted one as ***", () => {
    expect(at(6)).toMatchObject({ gutter: "capture", ghost: '= "***"' });
    expect(at(19)).toMatchObject({ gutter: "capture", ghost: '= "c1"' });
  });

  it("shows what a failed assert got", () => {
    expect(at(32)).toMatchObject({ gutter: "fail", ghost: '✗ got "pending"', ghostKind: "fail" });
    expect(at(30)?.gutter).toBe("pass");
  });

  it("dims every entry but the one a Send sent, saying which run they come from", () => {
    const send = replay("send", 3);
    const base = new Date(2026, 8, 30, 14, 32).toISOString();
    send.summary = { ...send.summary!, baseRunAt: base };
    const m = runMarks(model, lineOf, send);
    expect(m.find((x) => x.line === 2)).toMatchObject({ dim: true, ghost: expect.stringMatching(/from the run at 14:32$/) });
    expect(m.find((x) => x.line === 15)?.dim).toBeUndefined();
    // After the sent entry too: they come from the earlier run.
    expect(m.find((x) => x.line === 28)?.dim).toBe(true);
  });

  it("says when", () => {
    expect(runTime(new Date(2026, 8, 30, 9, 5).toISOString())).toBe("at 09:05");
    expect(runTime(null)).toBe("earlier");
  });
});
