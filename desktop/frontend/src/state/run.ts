// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The last run of each file, built from the run's events: entries as they
// finish, the requests being sent, logs and stream messages, the summary.

import { create } from "zustand";
import { Runs, appError, type Summary } from "../lib/api";
import { startRun } from "../lib/run-bridge";
import { apply, type FileRun } from "./run-model";
import { useEnv } from "./env";
import { useTabs } from "./tabs";
import { useUI } from "./ui";

export { apply, type FileRun, type LogLine } from "./run-model";

interface RunState {
  runs: Record<string, FileRun>;
  run(file: string, to?: number): Promise<void>;
  send(file: string, entry: number): Promise<void>;
  cancel(file: string): void;
  /** Forgets every run (another project was opened). */
  reset(): void;
}

const handles = new Map<string, () => void>();

export const useRuns = create<RunState>((set, get) => {
  // Applies fn to file's run when it is still run runId: a late event of
  // an earlier or canceled run changes nothing.
  const update = (file: string, runId: string, fn: (r: FileRun) => FileRun) =>
    set((s) => (s.runs[file]?.runId === runId ? { runs: { ...s.runs, [file]: fn(s.runs[file]) } } : s));

  const begin = (file: string, kind: FileRun["kind"], start: (runId: string) => Promise<Summary | null>) => {
    // One run of a file at a time from this page.
    if (get().runs[file]?.running) return Promise.resolve();
    const tab = useTabs.getState().tabs.find((t) => t.path === file);
    const source = tab?.text ?? "";
    const prev = get().runs[file];
    const handle = startRun<Summary>(
      (runId) => {
        const fresh: FileRun = {
          runId, kind, running: true, source,
          // A Send keeps the earlier entries of the run it reuses.
          entries: kind === "send" && prev ? prev.entries : {},
          sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null,
        };
        set((s) => ({ runs: { ...s.runs, [file]: fresh } }));
        return start(runId) as Promise<Summary>;
      },
      {
        onItems: (items, dropped) => update(file, handle.runId, (r) => apply(r, items, dropped)),
        onDone: (summary) => update(file, handle.runId, (r) => ({ ...r, running: false, summary, error: summary.error ?? null })),
      },
      (runId) => Runs.Cancel(runId),
    );
    handles.set(file, handle.cancel);
    return handle.result
      .then(
        () => undefined,
        (err) => {
          const e = appError(err);
          if (e.code === "busy" && get().runs[file]?.runId === handle.runId) {
            // Another page or agent is running the file: keep the last run.
            set((s) => {
              const runs = { ...s.runs };
              if (prev) runs[file] = prev;
              else delete runs[file];
              return { runs };
            });
          } else {
            update(file, handle.runId, (r) => ({ ...r, running: false, error: e.message }));
          }
          useUI.getState().toast({ kind: e.code === "busy" ? "warn" : "error", text: e.message });
        },
      )
      .finally(() => {
        if (handles.get(file) === handle.cancel) handles.delete(file);
      });
  };

  return {
    runs: {},
    run: (file, to = 0) => {
      const text = useTabs.getState().tabs.find((t) => t.path === file)?.text ?? "";
      return begin(file, "run", (runId) =>
        Runs.Run({ runId, file, source: text, env: useEnv.getState().current, to }),
      );
    },
    send: (file, entry) => {
      const text = useTabs.getState().tabs.find((t) => t.path === file)?.text ?? "";
      return begin(file, "send", (runId) =>
        Runs.Send({ runId, file, source: text, env: useEnv.getState().current, entry }),
      );
    },
    cancel: (file) => handles.get(file)?.(),
    reset: () => {
      handles.forEach((cancel) => cancel());
      handles.clear();
      set({ runs: {} });
    },
  };
});
