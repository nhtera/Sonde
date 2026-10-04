// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Runs. The page picks the run id, subscribes to the run's events first,
// then starts the run: events of a run that finishes before its first
// frame are not lost. Batches are applied in sequence order, and Done only
// once its last sequence number has been seen. A run call rejected before
// the run starts (busy file, bad entry) sends no Done: its rejection ends
// the run.

import { on } from "./events";

/** One event of a run (a view DTO), with its job. */
export interface RunItem {
  seq: number;
  unit: number;
  event: unknown;
}

interface Batch {
  runId: string;
  items: RunItem[];
  dropped: number;
}

interface Done<S> {
  runId: string;
  lastSeq: number;
  summary: S;
}

export interface RunHandlers<S> {
  /** Items in sequence order, as they arrive. */
  onItems(items: RunItem[], dropped: number): void;
  /** The run ended (its summary), once every item was applied. */
  onDone(summary: S): void;
}

/** A started run. */
export interface RunHandle<S> {
  runId: string;
  /** Resolves with the summary, or rejects when the run could not start. */
  result: Promise<S>;
  cancel(): void;
}

let counter = 0;

/** How long Done may trail the run call's answer before the call's
 * summary ends the run. */
const DONE_GRACE_MS = 1000;

/** A fresh run id. */
export function newRunId(): string {
  const rand = typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : String(Math.random()).slice(2);
  return `${rand}-${++counter}`;
}

/**
 * Starts a run: subscribes to its events, then calls start with the run
 * id. start is a run binding (runsvc.Run, Send, …) bound to its request.
 */
export function startRun<S>(
  start: (runId: string) => Promise<S> & { cancel?: () => void },
  handlers: RunHandlers<S>,
  cancelBinding?: (runId: string) => unknown,
): RunHandle<S> {
  const runId = newRunId();
  let next = 1;
  let dropped = 0;
  const pending = new Map<number, RunItem>();
  let done: Done<S> | undefined;
  let finished = false;
  let resolveResult!: (s: S) => void;
  let rejectResult!: (e: unknown) => void;
  const result = new Promise<S>((res, rej) => {
    resolveResult = res;
    rejectResult = rej;
  });

  const flush = () => {
    const ready: RunItem[] = [];
    while (pending.has(next)) {
      ready.push(pending.get(next)!);
      pending.delete(next);
      next++;
    }
    if (ready.length > 0 || dropped > 0) {
      handlers.onItems(ready, dropped);
      dropped = 0;
    }
    if (done && next > done.lastSeq && !finished) {
      finished = true;
      cleanup();
      handlers.onDone(done.summary);
      resolveResult(done.summary);
    }
  };

  const offItems = on(`run:${runId}`, (data) => {
    const b = data as Batch;
    for (const it of b.items ?? []) pending.set(it.seq, it);
    dropped += b.dropped ?? 0;
    flush();
  });
  const offDone = on(`run:${runId}:done`, (data) => {
    done = data as Done<S>;
    flush();
  });
  const cleanup = () => {
    offItems();
    offDone();
  };

  // Subscribed: now start the run.
  const call = start(runId);
  // The call returns once Done was sent. Should Done be lost (the event
  // stream reconnected meanwhile), the run ends from the call's summary.
  call.then(
    (summary) =>
      setTimeout(() => {
        if (finished) return;
        finished = true;
        cleanup();
        if (pending.size > 0) handlers.onItems([...pending.values()].sort((a, b) => a.seq - b.seq), dropped);
        handlers.onDone(summary);
        resolveResult(summary);
      }, DONE_GRACE_MS),
    () => undefined,
  );
  call.catch((err: unknown) => {
    // A call rejected before the run started sends no Done.
    setTimeout(() => {
      if (!finished && !done) {
        finished = true;
        cleanup();
        rejectResult(err);
      }
    }, 0);
  });

  return {
    runId,
    result,
    // The binding stops the run, which ends as canceled with its Done.
    // Canceling the call would reject it at once, before that summary.
    cancel: () => {
      if (cancelBinding) void cancelBinding(runId);
      else call.cancel?.();
    },
  };
}
