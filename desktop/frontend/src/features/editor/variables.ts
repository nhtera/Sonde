// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What the editor says about a {{variable}}: its value (*** for a secret
// or a redacted capture), where it comes from and, for a capture, the
// request and line that set it. One source for the hover card and the
// completion list: the app's variables (Vars.For: the environment, the
// session overrides), the file's captures (the request model) and the
// file's last run (the captured values).

import { Vars, type ScopeVar } from "../../lib/api";
import { on } from "../../lib/events";
import { useEnv } from "../../state/env";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { lastParsed, modelOf } from "./entry-at";
import type { EntryShape } from "./results/marks";

export type VariableKind = "capture" | "override" | "secret" | "project" | "function";

export interface VariableInfo {
  name: string;
  kind: VariableKind;
  /** The value as text, *** for a secret; "" when not known yet. */
  value: string;
  /** Where it comes from, short: "capture · line 6", "sonde.yaml". */
  source: string;
  /** For a capture: the request (1-based) and line that set it. */
  setBy?: { entry: number; line: number };
  /** For a capture: whether the last run captured it. */
  captured?: boolean;
}

/** Template functions, listed after the variables. */
export const templateFunctions = ["newDate", "newUuid"];

const cache = new Map<string, Promise<ScopeVar[]>>();

/** The variables a file can use in an environment (cached until the
 * environments, the overrides or a project file change). */
function varsFor(file: string, env: string): Promise<ScopeVar[]> {
  const key = `${file}\n${env}`;
  let p = cache.get(key);
  if (!p) {
    p = Vars.For(file, env).then((v) => v ?? [], () => []);
    cache.set(key, p);
  }
  return p;
}

export function clearVarsCache() {
  cache.clear();
}
useEnv.subscribe((s, prev) => (s.project !== prev.project || s.overrides !== prev.overrides) && clearVarsCache());
on("ws:changed", clearVarsCache);

/** The 1-based line of a UTF-16 offset in text. */
export function lineOfOffset(text: string, offset: number): number {
  let line = 1;
  for (let i = 0; i < offset && i < text.length; i++) if (text.charCodeAt(i) === 10) line++;
  return line;
}

interface CaptureSite {
  entry: number;
  line: number;
}

/** The captures of the file's requests by name, each at the last request
 * before offset that captures it (all requests when offset is omitted). */
export function captureSites(model: EntryShape[], text: string, offset = Infinity): Map<string, CaptureSite> {
  const sites = new Map<string, CaptureSite>();
  for (const e of [...model].sort((a, b) => a.Index - b.Index)) {
    for (const row of e.Rows?.captures ?? []) {
      if (row.Range.Start >= offset) continue;
      sites.set(row.Key, { entry: e.Index, line: lineOfOffset(text, row.Range.Start) });
    }
  }
  return sites;
}

/** The value the file's last run captured for name at entry, as text. */
function capturedValue(file: string, entry: number, name: string): string | undefined {
  const c = useRuns.getState().runs[file]?.entries[entry]?.captures?.find((x) => x.name === name);
  if (!c) return undefined;
  return typeof c.value === "string" ? c.value : JSON.stringify(c.value);
}

function fromVar(v: ScopeVar): VariableInfo {
  if (v.source === "override") return { name: v.name, kind: "override", value: v.secret ? "***" : v.display, source: "session override" };
  if (v.source === "capture") return { name: v.name, kind: "capture", value: v.secret ? "***" : v.display, source: "capture", captured: true };
  const origin = v.origin || "sonde.yaml";
  return { name: v.name, kind: v.secret ? "secret" : "project", value: v.secret ? "***" : v.display, source: origin };
}

/** The variables file can use at offset (the whole file when omitted):
 * the captures of earlier requests first, nearest first, then the session
 * overrides and the environment's variables. */
export async function variablesAt(file: string, offset?: number): Promise<VariableInfo[]> {
  const tab = useTabs.getState().tabs.find((t) => t.path === file);
  const [vars, model] = await Promise.all([
    varsFor(file, useEnv.getState().current),
    tab ? modelOf({ file, text: tab.text, version: tab.version }) : Promise.resolve(null),
  ]);
  // A {{ being typed makes the file not parse: the captures before it
  // are those of the last text that did.
  const good = model && tab ? { model, text: tab.text } : lastParsed(file);
  const sites = good ? captureSites(good.model, good.text, offset) : new Map<string, CaptureSite>();
  const captures: VariableInfo[] = [...sites]
    .sort((a, b) => b[1].line - a[1].line)
    .map(([name, at]) => {
      const value = capturedValue(file, at.entry, name);
      return { name, kind: "capture", value: value ?? "", source: `capture · line ${at.line}`, setBy: at, captured: value !== undefined };
    });
  const rest = vars.filter((v) => v.source !== "capture" && !sites.has(v.name)).map(fromVar);
  return [...captures, ...rest.filter((v) => v.kind === "override"), ...rest.filter((v) => v.kind !== "override")];
}

/** What the editor says about name in file at offset: a capture is the
 * last one before it, as a run uses. */
export async function describeVariable(file: string, name: string, offset?: number): Promise<VariableInfo | null> {
  return (await variablesAt(file, offset)).find((v) => v.name === name) ?? null;
}
