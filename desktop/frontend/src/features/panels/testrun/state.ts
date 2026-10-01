// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Test run: the files picked, the last test run (its summary, and
// each file's run built from its events, as the Results panel builds a
// file's), and running one.

import { create } from "zustand";
import { appError, Runs, Workspace, type Summary } from "../../../lib/api";
import { startRun } from "../../../lib/run-bridge";
import type { RunEvent } from "../../../lib/view";
import { useEnv } from "../../../state/env";
import { apply, type FileRun } from "../../../state/run-model";
import { useTabs } from "../../../state/tabs";
import { useUI } from "../../../state/ui";
import type { Node } from "../../../lib/api";

interface TestRunState {
  /** Files left out of the run (every request file is in by default). */
  excluded: string[];
  runId: string | null;
  running: boolean;
  summary: Summary | null;
  startedAt: number;
  /** Each file's run, by project path. */
  files: Record<string, FileRun>;
  toggle(file: string): void;
  setAll(on: boolean, files: string[]): void;
  /** Runs files as a test run; with dataHandle (a data file picked in a
   * dialog), runs the one file once per row instead. */
  start(files: string[], dataHandle?: string): Promise<void>;
  cancel(): void;
}

const fresh = (runId: string): FileRun => ({
  runId, kind: "run", running: true, source: "", entries: {}, sending: {}, skipped: {},
  logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null,
});

let cancelRun: (() => void) | null = null;

export const useTestRun = create<TestRunState>((set, get) => ({
  excluded: [],
  runId: null,
  running: false,
  summary: null,
  startedAt: 0,
  files: {},
  toggle: (file) => set((s) => ({ excluded: s.excluded.includes(file) ? s.excluded.filter((f) => f !== file) : [...s.excluded, file] })),
  setAll: (on, files) => set({ excluded: on ? [] : files }),
  start: async (files, dataHandle) => {
    if (get().running || files.length === 0 || (dataHandle && files.length !== 1)) return;
    // Open tabs run as the user sees them.
    const sources: Record<string, string> = {};
    for (const t of useTabs.getState().tabs) if (files.includes(t.path)) sources[t.path] = t.text;
    const units: Record<number, string> = {};
    const handle = startRun<Summary>(
      (runId) => {
        set({ runId, running: true, summary: null, files: {}, startedAt: Date.now() });
        const env = useEnv.getState().current;
        if (dataHandle) {
          const file = files[0];
          const source = file in sources ? Promise.resolve(sources[file]) : Workspace.Read(file).then((f) => f?.text ?? "");
          return source.then((text) => Runs.RunData({ runId, file, source: text, env, dataHandle, rows: [], secrets: [] })) as Promise<Summary>;
        }
        return Runs.RunTest({ runId, files, sources, env }) as Promise<Summary>;
      },
      {
        onItems: (items) =>
          set((s) => {
            const next = { ...s.files };
            // Items in sequence order; a unit names its file first.
            const byFile = new Map<string, typeof items>();
            for (const it of items) {
              const ev = it.event as RunEvent;
              if (ev.type === "unitStarted") {
                units[it.unit] = ev.file;
                next[ev.file] = fresh(handle.runId);
                continue;
              }
              const f = units[it.unit];
              if (f) byFile.set(f, [...(byFile.get(f) ?? []), it]);
            }
            for (const [f, its] of byFile) next[f] = apply(next[f] ?? fresh(handle.runId), its, 0);
            return { files: next };
          }),
        onDone: (summary) =>
          set((s) => {
            const files = { ...s.files };
            for (const f of Object.keys(files)) files[f] = { ...files[f], running: false };
            return { running: false, summary, files };
          }),
      },
      (runId) => Runs.Cancel(runId),
    );
    cancelRun = handle.cancel;
    try {
      await handle.result;
    } catch (err) {
      set({ running: false });
      useUI.getState().toast({ kind: "error", text: appError(err).message });
    }
  },
  cancel: () => cancelRun?.(),
}));

/** The request files of a tree (.hurl, .sonde), in tree order. */
export function requestFiles(tree: Node | null): string[] {
  const out: string[] = [];
  const walk = (n: Node | null | undefined) => {
    if (!n) return;
    if (n.kind === "request") out.push(n.path);
    n.children?.forEach(walk);
  };
  walk(tree);
  return out;
}

/** Whether file passed: by its units once the run is done (every data
 * row, and a unit stopped or refused counts as failed), else by its
 * entries so far; undefined while nothing is known. */
export function fileSucceeded(file: string, summary: Summary | null, run?: FileRun): boolean | undefined {
  const units = (summary?.units ?? []).filter((u) => u.file === file);
  if (units.length > 0) return units.every((u) => u.success && !u.error && !u.parseError);
  const entries = Object.values(run?.entries ?? {});
  if (entries.length === 0) return undefined;
  return entries.every((e) => e.success);
}
