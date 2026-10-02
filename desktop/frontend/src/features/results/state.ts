// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Results panel's own state: the request each file's run shows, the
// detail tab and body mode picked, and the interactive session open.

import { create } from "zustand";
import type { FileRun } from "../../state/run-model";
import type { Entry } from "../../lib/view";
import { cardOf } from "./model";

export type TabId = "error" | "stream" | "body" | "headers" | "asserts" | "captures" | "cookies" | "timeline" | "request";
export type BodyMode = "pretty" | "raw" | "preview";

interface Choice<T> {
  runId: string;
  value: T;
}

interface ResultsState {
  /** The request picked in each file's run (for that run only). */
  picked: Record<string, Choice<number>>;
  /** The tab picked (for that run only: a new run shows its failure first). */
  tab: Choice<TabId> | null;
  bodyMode: BodyMode;
  /** The interactive WebSocket session shown instead of the results. */
  session: { file: string; entry: number } | null;
  /** The body the Body tab shows (for Save response). */
  activeBody: { id: string; contentType: string } | null;
  pick(file: string, runId: string, entry: number): void;
  setTab(runId: string, tab: TabId): void;
  setBodyMode(mode: BodyMode): void;
  openSession(file: string, entry: number): void;
  closeSession(): void;
}

export const useResults = create<ResultsState>((set) => ({
  picked: {},
  tab: null,
  bodyMode: "pretty",
  session: null,
  activeBody: null,
  pick: (file, runId, entry) => set((s) => ({ picked: { ...s.picked, [file]: { runId, value: entry } } })),
  setTab: (runId, value) => set({ tab: { runId, value } }),
  setBodyMode: (bodyMode) => set({ bodyMode }),
  openSession: (file, entry) => set({ session: { file, entry } }),
  closeSession: () => set({ session: null }),
}));

/** The request a run shows first: the one picked, else its first
 * failure, the one sent, the one running, or the last finished. */
export function shownEntry(run: FileRun, picked: Choice<number> | undefined): number {
  if (picked && picked.runId === run.runId) return picked.value;
  const done = Object.values(run.entries).sort((a, b) => a.index - b.index);
  const failed = done.find((e) => !e.success && (run.kind !== "send" || e.index === run.sent));
  if (failed) return failed.index;
  if (run.kind === "send" && run.sent) return run.sent;
  if (run.running && run.current) return run.current;
  return done.at(-1)?.index ?? run.current ?? 0;
}

/** The detail tabs of an entry, in order. */
export function tabsOf(e: Entry | undefined): TabId[] {
  if (!e) return ["timeline", "request"];
  const hasResponse = (e.calls?.length ?? 0) > 0;
  if (!hasResponse) return cardOf(e.errors?.[0]) ? ["error", "timeline", "request"] : ["asserts", "timeline", "request"];
  // A stream's captures and cookies show only when it has some.
  if (e.sonde?.stream) {
    const call = e.calls?.at(-1);
    const cookies = (call?.request.cookies?.length ?? 0) + (call?.response.cookies?.length ?? 0);
    return ["stream", "headers", "asserts", ...(e.captures?.length ? (["captures"] as const) : []), ...(cookies ? (["cookies"] as const) : []), "timeline", "request"];
  }
  return ["body", "headers", "asserts", "captures", "cookies", "timeline", "request"];
}

/** The tab an entry shows: the one picked for this run when it has it,
 * else its failure (an error card, a failed assert) or its content. */
export function shownTab(e: Entry | undefined, runId: string, picked: Choice<TabId> | null): TabId {
  const tabs = tabsOf(e);
  if (picked && picked.runId === runId && tabs.includes(picked.value)) return picked.value;
  if (tabs[0] === "error") return "error";
  if (e && !e.success && (e.errors?.[0]?.assert || e.errors?.[0]?.kind === "assert-status")) return "asserts";
  return tabs[0];
}
