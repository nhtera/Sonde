// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The import dialog's state: the request being shaped (kind, input,
// options), its live preview, then what was written and the optional
// suggestions of a Postman import. Go does the converting; the page never
// holds a lifted secret's value.

import { create } from "zustand";
import {
  appError,
  EditSvc,
  Imports,
  type ImportCandidate,
  type ImportPreview,
  type ImportRequest,
  type ImportSuggestion,
  type ImportWritten,
} from "../../lib/api";
import { applyEdit, type EditResult } from "../../state/edits";
import { useEnv } from "../../state/env";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { useWorkspace } from "../../state/workspace";

export type ImportKind = "curl" | "postman" | "opencollection" | "http" | "openapi";
export type Decision = "accepted" | "rejected";

export const kinds: { kind: ImportKind; title: string }[] = [
  { kind: "curl", title: "curl" },
  { kind: "postman", title: "Postman" },
  { kind: "opencollection", title: "Bruno" },
  { kind: "http", title: ".http" },
  { kind: "openapi", title: "OpenAPI" },
];

/** Names of the staged inputs, by ID (for the page to show). */
type Names = Record<string, string>;

interface ImportState {
  open: boolean;
  step: "form" | "result" | "suggestions";
  req: ImportRequest;
  names: Names;
  preview: ImportPreview | null;
  error: string;
  overwrite: string[];
  busy: boolean;
  written: ImportWritten | null;
  suggestions: ImportSuggestion[];
  decisions: Record<string, Decision>;
  /** Files whose suggestions are written (an Apply retried skips them). */
  applied: string[];
  show(kind: ImportKind): void;
  close(): void;
  update(patch: Partial<ImportRequest>, names?: Names): void;
  setLift(ids: number[]): void;
  setOverwrite(path: string, on: boolean): void;
  refresh(): Promise<void>;
  write(): Promise<void>;
  insert(file: string): Promise<void>;
  review(): void;
  decide(path: string, d: Decision | null): void;
  apply(): Promise<void>;
}

/** The env lifted secrets go to: the current one, else the first. */
function defaultEnv(): string {
  const { current, project } = useEnv.getState();
  return current || project?.envs?.[0]?.name || "";
}

export function freshRequest(kind: ImportKind): ImportRequest {
  return { kind, input: "", text: "", environments: [], group: "", baseUrlVar: "", ext: "hurl", folder: kind === "curl" ? "" : "imported", env: defaultEnv(), lift: null, target: "", targetText: "" };
}

/** Whether req has an input to preview. */
export function ready(req: ImportRequest): boolean {
  return req.input !== "" || req.text.trim() !== "";
}

/** The candidate IDs lifted: until the user picks (null), every one when
 * there is an env to write them to (Go does the same). */
export function liftOf(cands: ImportCandidate[], lift: number[] | null, env: string): number[] {
  if (lift === null) return env ? cands.map((c) => c.id) : [];
  return lift.filter((id) => cands.some((c) => c.id === id));
}

/** "3 accepted · 1 rejected · 2 pending". */
export function tally(sugg: ImportSuggestion[], decisions: Record<string, Decision>): { accepted: number; rejected: number; pending: number } {
  let accepted = 0;
  let rejected = 0;
  for (const s of sugg) {
    if (decisions[s.path] === "accepted") accepted++;
    else if (decisions[s.path] === "rejected") rejected++;
  }
  return { accepted, rejected, pending: sugg.length - accepted - rejected };
}

/** Whether a file can take pasted requests (a request file). */
export const requestFile = (path: string | null | undefined) => !!path && /\.(hurl|sonde)$/.test(path);

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

let seq = 0;

export const useImport = create<ImportState>((set, get) => ({
  open: false,
  step: "form",
  req: freshRequest("curl"),
  names: {},
  preview: null,
  error: "",
  overwrite: [],
  busy: false,
  written: null,
  suggestions: [],
  decisions: {},
  applied: [],
  show: (kind) =>
    set({ open: true, step: "form", req: freshRequest(kind), names: {}, preview: null, error: "", overwrite: [], written: null, suggestions: [], decisions: {}, applied: [] }),
  close: () => set({ open: false }),
  update: (patch, names) => {
    const req = { ...get().req, ...patch };
    // Another kind starts over.
    if (patch.kind && patch.kind !== get().req.kind) {
      set({ req: { ...freshRequest(patch.kind as ImportKind), ...patch }, names: {}, preview: null, error: "", overwrite: [] });
    } else {
      // Another input: its candidates are other values, picked afresh.
      if ("text" in patch || "input" in patch) req.lift = null;
      set({ req, names: { ...get().names, ...names } });
    }
    void get().refresh();
  },
  setLift: (lift) => {
    set({ req: { ...get().req, lift } });
    void get().refresh();
  },
  setOverwrite: (path, on) => set((s) => ({ overwrite: on ? [...s.overwrite, path] : s.overwrite.filter((p) => p !== path) })),
  refresh: async () => {
    const req = get().req;
    if (!ready(req)) {
      set({ preview: null, error: "" });
      return;
    }
    const n = ++seq;
    try {
      const preview = await Imports.Preview(req);
      if (n === seq) set({ preview, error: "" });
    } catch (err) {
      if (n === seq) set({ preview: null, error: appError(err).message });
    }
  },
  write: async () => {
    const { req, overwrite } = get();
    set({ busy: true });
    try {
      const written = await Imports.Write(req, overwrite);
      const suggestions = req.kind === "postman" ? ((await Imports.Suggestions(req)) ?? []) : [];
      set({ written, suggestions, step: "result" });
      await useWorkspace.getState().refresh();
    } catch (err) {
      fail(err);
    } finally {
      set({ busy: false });
    }
  },
  insert: async (file) => {
    const tab = useTabs.getState().tabs.find((t) => t.path === file);
    if (!tab) return;
    // The requests in the file's dialect, avoiding the names it captures.
    const req = { ...get().req, target: file, targetText: tab.text };
    set({ busy: true });
    try {
      const text = await Imports.CurlText(req);
      const res = (await EditSvc.Apply(
        { file, text: tab.text, version: tab.version },
        { kind: "appendEntries", entry: 0, section: "", index: 0, key: "", value: text },
      )) as EditResult | null;
      if (!res || !applyEdit(file, res)) {
        fail(new Error("The file changed: try again"));
        return;
      }
      // The secrets once the requests naming them are in.
      const saved = (await Imports.SaveSecrets(req)) ?? [];
      const where = saved.length ? `; ${saved.join(", ")} in the secrets file` : "";
      useUI.getState().toast({ kind: "success", text: `Added to ${file.split("/").pop()}${where}` });
      set({ open: false });
    } catch (err) {
      fail(err);
    } finally {
      set({ busy: false });
    }
  },
  review: () => set({ step: "suggestions" }),
  decide: (path, d) =>
    set((s) => {
      const decisions = { ...s.decisions };
      if (d) decisions[path] = d;
      else delete decisions[path];
      return { decisions };
    }),
  apply: async () => {
    const { req, suggestions, decisions } = get();
    set({ busy: true });
    try {
      for (const s of suggestions) {
        if (decisions[s.path] !== "accepted" || get().applied.includes(s.path)) continue;
        await Imports.Accept(req, s.path);
        set((st) => ({ applied: [...st.applied, s.path] }));
      }
      const done = get().applied.length;
      useUI.getState().toast({ kind: "success", text: `Applied the suggestions to ${done} file${done === 1 ? "" : "s"}` });
      set({ open: false });
    } catch (err) {
      fail(err);
    } finally {
      set({ busy: false });
    }
  },
}));
