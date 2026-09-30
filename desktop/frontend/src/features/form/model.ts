// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Form view's model of a file: its entries as Go reads them
// (editsvc.Model, the asserts split by editsvc.Checks), refreshed once
// typing pauses; while the text does not parse, the last model stays and
// the form says so. Plus what the form reads from an entry: its auth, its
// body kind, its options.

import { useEffect } from "react";
import { create } from "zustand";
import { appError, EditSvc, type Check, type EntryModel, type ModelRow } from "../../lib/api";
import { useTabs } from "../../state/tabs";

export type { Check, EntryModel, ModelRow };

/** Row sections, as Go names them. */
export type Sec = "headers" | "query" | "form" | "multipart" | "cookies" | "basic-auth" | "options" | "response-headers" | "captures" | "asserts" | "grpc";

export interface FileModel {
  /** The tab version the model was read from. */
  version: number;
  entries: EntryModel[];
  /** Each entry's asserts split for the grid, by entry index. */
  checks: Record<number, Check[]>;
  /** Why the text at the tab's version does not read (the model is then
   * the last one that did). */
  invalid?: string;
}

interface FormState {
  models: Record<string, FileModel>;
  /** The entry each file's form shows. */
  entry: Record<string, number>;
  tab: string;
  select(file: string, entry: number): void;
  setTab(tab: string): void;
}

export const useForm = create<FormState>((set) => ({
  models: {},
  entry: {},
  tab: "params",
  select: (file, entry) => set((s) => ({ entry: { ...s.entry, [file]: entry } })),
  setTab: (tab) => set({ tab }),
}));

const timers = new Map<string, ReturnType<typeof setTimeout>>();

/** Reads file's model now (the edit helpers wait for it). */
export async function loadModel(file: string): Promise<FileModel | null> {
  const tab = useTabs.getState().tabs.find((t) => t.path === file);
  if (!tab) return null;
  const b = { file, text: tab.text, version: tab.version };
  const had = useForm.getState().models[file];
  let next: FileModel;
  try {
    const [entries, checks] = await Promise.all([EditSvc.Model(b), EditSvc.Checks(b)]);
    const byEntry: Record<number, Check[]> = {};
    for (const c of checks ?? []) byEntry[c.entry] = c.asserts ?? [];
    next = { version: tab.version, entries: entries ?? [], checks: byEntry };
  } catch (err) {
    next = { ...(had ?? { entries: [], checks: {} }), version: tab.version, invalid: appError(err).message };
  }
  // A newer text reads on its own.
  const now = useTabs.getState().tabs.find((t) => t.path === file);
  if (now?.version !== tab.version) return null;
  useForm.setState((s) => ({ models: { ...s.models, [file]: next } }));
  return next;
}

/** Keeps file's model current while the form shows it (50 ms after the
 * last change). */
export function useFileModel(file: string): FileModel | undefined {
  const version = useTabs((s) => s.tabs.find((t) => t.path === file)?.version);
  useEffect(() => {
    if (version === undefined) return;
    clearTimeout(timers.get(file));
    timers.set(
      file,
      setTimeout(() => void loadModel(file), useForm.getState().models[file] ? 50 : 0),
    );
    return () => clearTimeout(timers.get(file));
  }, [file, version]);
  return useForm((s) => s.models[file]);
}

/** The entry the form shows for file: the one picked, else the one at
 * the Text cursor (1-based line), else the first; undefined when the
 * model has none. */
export function shownEntry(model: FileModel | undefined, picked: number | undefined, text: string, cursorLine?: number): EntryModel | undefined {
  const entries = model?.entries ?? [];
  const atCursor = cursorLine ? entries.find((e) => lineOf(text, e.Range.Start) <= cursorLine && cursorLine <= lineOf(text, e.Range.End)) : undefined;
  return entries.find((e) => e.Index === picked) ?? atCursor ?? entries[0];
}

/** The rows of a section ([] when it has none). */
export const rowsOf = (e: EntryModel | undefined, sec: Sec): ModelRow[] => ((e?.Rows as Record<string, ModelRow[] | null> | null)?.[sec] ?? []);

/** Enabled rows of a section: what its tab counts. */
export const countOf = (e: EntryModel | undefined, sec: Sec) => rowsOf(e, sec).filter((r) => !r.Disabled).length;

export type AuthKind = "none" | "bearer" | "basic" | "apikey" | "cert";

export interface Auth {
  kind: AuthKind;
  /** Where the auth is written: the section and row index. */
  sec?: Sec;
  index?: number;
  /** Row indices of a client certificate's options (cert, key). */
  rows?: number[];
}

const apiKeyName = /^x-api-key$|api[-_]?key/i;

/** The auth an entry writes, read back from its rows. */
export function authOf(e: EntryModel | undefined): Auth {
  const headers = rowsOf(e, "headers");
  const bearer = headers.findIndex((r) => !r.Disabled && /^authorization$/i.test(r.Key) && /^Bearer\s/.test(r.Value));
  if (bearer >= 0) return { kind: "bearer", sec: "headers", index: bearer };
  if (rowsOf(e, "basic-auth").length > 0) return { kind: "basic", sec: "basic-auth", index: 0 };
  const key = headers.findIndex((r) => !r.Disabled && apiKeyName.test(r.Key));
  if (key >= 0) return { kind: "apikey", sec: "headers", index: key };
  const q = rowsOf(e, "query").findIndex((r) => !r.Disabled && apiKeyName.test(r.Key));
  if (q >= 0) return { kind: "apikey", sec: "query", index: q };
  const certRows = rowsOf(e, "options")
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => r.Key === "cert" || r.Key === "key")
    .map(({ i }) => i);
  if (certRows.length > 0) return { kind: "cert", sec: "options", rows: certRows };
  return { kind: "none" };
}

export type BodyKind = "none" | "form-data" | "urlencoded" | "json" | "xml" | "text" | "binary" | "graphql" | "other";

/** How an entry's body is written. */
export function bodyKindOf(e: EntryModel | undefined): BodyKind {
  if (rowsOf(e, "multipart").length > 0) return "form-data";
  if (rowsOf(e, "form").length > 0) return "urlencoded";
  if (!e?.HasBody) return "none";
  const b = e.Body.trimStart();
  if (b.startsWith("```graphql")) return "graphql";
  if (/^(file|hex|base64),/.test(b)) return "binary";
  if (b.startsWith("{") || b.startsWith("[")) return "json";
  if (b.startsWith("<")) return "xml";
  // A ``` block with a language tag (```json…) is kept as written: it is
  // edited in Text.
  if (/^```[A-Za-z]/.test(b)) return "other";
  return "text";
}

/** The inside of a ``` multiline string (its body kind tag dropped), or
 * the text of a `…` string. */
export function multilineText(body: string): string {
  const m = /^```[a-z]*\n([\s\S]*?)\n?```$/.exec(body.trim());
  if (m) return m[1];
  const one = /^`(.*)`$/.exec(body.trim());
  return one ? one[1] : body;
}

/** A GraphQL body's query and its variables block ("" for none). */
export function graphqlParts(body: string): { query: string; variables: string } {
  const inner = multilineText(body);
  const at = inner.search(/(^|\n)variables\s*\{/);
  if (at < 0) return { query: inner, variables: "" };
  const start = inner.indexOf("{", at);
  return { query: inner.slice(0, at).replace(/\n$/, ""), variables: inner.slice(start) };
}

/** The GraphQL body of a query and variables. */
export const graphqlBody = (query: string, variables: string) =>
  "```graphql\n" + query.replace(/\n+$/, "") + (variables.trim() ? "\nvariables " + variables.trim() : "") + "\n```";

/** A `file,path; type` value: the path as written (escaped), the type. */
const fileRefRE = /^file,((?:\\.|[^;\\])*);\s*(.*)$/;

/** The file a `file,…;` value names (unescaped) and its content type;
 * null for another value. */
export function fileRef(value: string): { path: string; type: string } | null {
  const m = fileRefRE.exec(value.trim());
  return m ? { path: m[1].replace(/\\(.)/g, "$1"), type: m[2] } : null;
}

/** A path written as a file name of the file format (spaces, `;`, `#`,
 * braces escaped), by Go's writer. */
export async function escapeFilename(path: string): Promise<string> {
  return (await EditSvc.EscapeFilename(path)) ?? path;
}

/** A `file,path;` value (and its content type). */
export async function fileValue(path: string, type = ""): Promise<string> {
  return `file,${await escapeFilename(path)};${type ? ` ${type}` : ""}`;
}

/** The line of a UTF-16 offset of text (1-based). */
export function lineOf(text: string, offset: number): number {
  let line = 1;
  for (let i = 0; i < offset && i < text.length; i++) if (text.charCodeAt(i) === 10) line++;
  return line;
}
