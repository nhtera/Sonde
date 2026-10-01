// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { FileRun } from "../../state/run-model";
import { clockOf, copyItems, shellOf } from "./model";

const run = (over: Partial<FileRun>): FileRun =>
  ({ runId: "r", kind: "run", running: false, source: "", entries: {}, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null, ...over }) as FileRun;

describe("copyItems", () => {
  const base = { file: "a.hurl", source: "GET x", env: "local", shell: "posix" };

  it("offers the request at the cursor, the file, and the commands", () => {
    const items = copyItems({ ...base, entry: 3 });
    expect(items.map((i) => i.id)).toEqual(["curl.request", "curl.file", "sonde.file", "sonde.to", "sonde.test"]);
    expect(items[0].req).toMatchObject({ entry: 3, kind: "" });
    expect(items[3].req).toMatchObject({ kind: "send", entry: 3 });
    expect(items[4].req).toMatchObject({ kind: "test", files: ["a.hurl"] });
  });

  it("without a request at the cursor, only the file's", () => {
    expect(copyItems({ ...base, entry: 0 }).map((i) => i.id)).toEqual(["curl.file", "sonde.file", "sonde.test"]);
  });

  it("after a Send, the command up to the request sent, with the run's time", () => {
    const at = new Date(2026, 9, 1, 9, 5).toISOString();
    const items = copyItems({ ...base, entry: 0, run: run({ kind: "send", sent: 2, summary: { baseRunAt: at } as FileRun["summary"] }) });
    const send = items.find((i) => i.id === "sonde.send")!;
    expect(send.req).toMatchObject({ kind: "send", entry: 2, clock: "09:05" });
    expect(copyItems({ ...base, entry: 0, run: run({ kind: "run" }) }).some((i) => i.id === "sonde.send")).toBe(false);
  });
});

describe("clockOf and shellOf", () => {
  it("formats HH:MM, nothing for a bad time", () => {
    expect(clockOf(new Date(2026, 0, 2, 14, 7).toISOString())).toBe("14:07");
    expect(clockOf("not a time")).toBe("");
  });

  it("quotes for PowerShell on Windows only", () => {
    expect(shellOf("Win32")).toBe("powershell");
    expect(shellOf("MacIntel")).toBe("posix");
  });
});
