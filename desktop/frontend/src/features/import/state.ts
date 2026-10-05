// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The import dialog's state: the request being shaped (kind, input,
// options), its live preview, then what was written and the optional
// suggestions of a Postman import. Go does the converting; the page never
// holds a lifted secret's value.

import { create } from "zustand";
import { isRequestPath } from "../../lib/files";
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
  /** A preview is on its way (typed text not previewed yet): Import
   * waits for it, so what is written is what was shown. */
  pending: boolean;
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
  /** Decides a change (key: changeKey), or undoes the decision. */
  decide(key: string, d: Decision | null): void;
  /** Decides every change of a file. */
  decideFile(path: string, d: Decision): void;
  apply(): Promise<void>;
}

/** Whether a collection file is picked and reads: the dialog shows its
 * preview without the source choices. A file that does not read (its
 * error shown) leaves them, to pick another. */
export function collectionPicked(s: { req: ImportRequest; error: string }): boolean {
  return s.req.kind === "postman" && s.req.input !== "" && !s.error;
}

/** The env lifted secrets go to: the current one, else the first. */
function defaultEnv(): string {
  const { current, project } = useEnv.getState();
  return current || project?.envs?.[0]?.name || "";
}

/** The mapping into the project's environments with a default for each
 * collection environment that has none; null when none is missing. */
export function defaultInto(envs: string[], into: Record<string, string | undefined>): Record<string, string> | null {
  const { current, project } = useEnv.getState();
  const names = (project?.envs ?? []).map((e) => e.name);
  if (names.length === 0) return null;
  const missing = envs.filter((n) => !(n in into));
  if (missing.length === 0) return null;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(into)) if (v !== undefined) out[k] = v;
  for (const n of missing) out[n] = names.includes(n) ? n : current || names[0];
  return out;
}

export function freshRequest(kind: ImportKind): ImportRequest {
  return { kind, input: "", text: "", environments: [], group: "", baseUrlVar: "", ext: "hurl", folder: kind === "curl" ? "" : "imported", name: "", env: defaultEnv(), lift: null, into: {}, target: "", targetText: "" };
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

/** A change's key in the decisions: its file and index. */
export const changeKey = (path: string, index: number) => `${path}#${index}`;

/** The changes a decision can be made on: those that apply. */
export const changesOf = (s: ImportSuggestion) => (s.changes ?? []).filter((c) => !c.error);

/** "3 accepted · 1 rejected · 2 pending", counting changes. */
export function tally(sugg: ImportSuggestion[], decisions: Record<string, Decision>): { accepted: number; rejected: number; pending: number } {
  let accepted = 0;
  let rejected = 0;
  let all = 0;
  for (const s of sugg) {
    for (const c of changesOf(s)) {
      all++;
      const d = decisions[changeKey(s.path, c.index)];
      if (d === "accepted") accepted++;
      else if (d === "rejected") rejected++;
    }
  }
  return { accepted, rejected, pending: all - accepted - rejected };
}

/** A file's state in the list: every change decided one way, some of
 * them, or none. */
export function fileState(s: ImportSuggestion, decisions: Record<string, Decision>): "accepted" | "rejected" | "mixed" | "pending" | "error" {
  const cs = changesOf(s);
  if (cs.length === 0) return "error";
  const ds = cs.map((c) => decisions[changeKey(s.path, c.index)]);
  if (ds.every((d) => d === "accepted")) return "accepted";
  if (ds.every((d) => d === "rejected")) return "rejected";
  return ds.some((d) => d) ? "mixed" : "pending";
}

/** Whether a file can take pasted requests (a request file). */
export const requestFile = isRequestPath;

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

let seq = 0;

export const useImport = create<ImportState>((set, get) => ({
  open: false,
  step: "form",
  req: freshRequest("curl"),
  names: {},
  preview: null,
  pending: false,
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
      set({ preview: null, error: "", pending: false });
      return;
    }
    const n = ++seq;
    set({ pending: true });
    try {
      const preview = await Imports.Preview(req);
      if (n !== seq) return;
      set({ preview, error: "", pending: false });
      // The collection's environments go into the project's: each new one
      // to the one of its name, else the current one (shown, changeable).
      const into = defaultInto(preview?.importEnvs ?? [], req.into ?? {});
      if (into) {
        set({ req: { ...get().req, into } });
        void get().refresh();
      }
    } catch (err) {
      if (n === seq) set({ preview: null, error: appError(err).message, pending: false });
    }
  },
  write: async () => {
    const { req, overwrite } = get();
    if (get().pending) return;
    set({ busy: true });
    try {
      const written = await Imports.Write(req, overwrite);
      const suggestions = req.kind === "postman" || req.kind === "opencollection" ? ((await Imports.Suggestions(req)) ?? []) : [];
      // The environments the import wrote, to tell what is left to define.
      await useEnv.getState().load();
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
  decide: (key, d) =>
    set((s) => {
      const decisions = { ...s.decisions };
      if (d) decisions[key] = d;
      else delete decisions[key];
      return { decisions };
    }),
  decideFile: (path, d) =>
    set((s) => {
      const sg = s.suggestions.find((x) => x.path === path);
      if (!sg) return s;
      const decisions = { ...s.decisions };
      for (const c of changesOf(sg)) decisions[changeKey(path, c.index)] = d;
      return { decisions };
    }),
  apply: async () => {
    const { req, suggestions, decisions } = get();
    set({ busy: true });
    try {
      for (const s of suggestions) {
        // Only the accepted changes; a file none of whose changes is
        // accepted stays as imported.
        const picked = changesOf(s).filter((c) => decisions[changeKey(s.path, c.index)] === "accepted").map((c) => c.index);
        if (picked.length === 0 || get().applied.includes(s.path)) continue;
        await Imports.Accept(req, s.path, picked);
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
