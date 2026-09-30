// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// One hover: the language server's, and for a {{variable}} its value (***
// for a secret) and where it comes from, as the app resolves it
// (Vars.For, the last run's captures).

import { syntaxTree } from "@codemirror/language";
import type { SyntaxNode } from "@lezer/common";
import { LSPPlugin } from "@codemirror/lsp-client";
import { hoverTooltip, type EditorView, type Tooltip } from "@codemirror/view";
import type * as lsp from "vscode-languageserver-protocol";
import { Vars, type ScopeVar } from "../../../lib/api";
import { useEnv } from "../../../state/env";
import { on } from "../../../lib/events";
import { useRuns } from "../../../state/run";
import { lspReady } from "./client";

/** The variable name under pos, when pos is inside a {{…}}. */
export function variableAt(view: EditorView, pos: number): string | null {
  for (let n: SyntaxNode | null = syntaxTree(view.state).resolveInner(pos, 1); n; n = n.parent) {
    if (n.name === "VariableName") return view.state.sliceDoc(n.from, n.to).trim();
    if (n.name === "UrlTemplate" || n.name === "Template" || n.name === "StringTemplate" || n.name === "BacktickTemplate") {
      const text = view.state.sliceDoc(n.from, n.to);
      const m = /^\{\{\s*([^\s}]+)\s*\}\}$/.exec(text);
      return m ? m[1] : null;
    }
  }
  return null;
}

/** How long a hover waits for the language server. */
const HOVER_WAIT_MS = 300;

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

/** The value and source of name for file, as the app resolves it. */
export async function describeVariable(file: string, name: string): Promise<{ value: string; source: string } | null> {
  // A capture of the file's last run.
  const run = useRuns.getState().runs[file];
  for (const e of Object.values(run?.entries ?? {}).reverse()) {
    const c = e.captures?.find((x) => x.name === name);
    if (c) return { value: typeof c.value === "string" ? c.value : JSON.stringify(c.value), source: `captured by request ${e.index}` };
  }
  const v = (await varsFor(file, useEnv.getState().current)).find((x) => x.name === name);
  if (!v) return null;
  return { value: v.secret ? "***" : v.display, source: v.origin ? `${v.source} · ${v.origin}` : v.source };
}

export function sondeHover(file: string) {
  return hoverTooltip(async (view, pos): Promise<Tooltip | null> => {
    const plugin = LSPPlugin.get(view);
    let html = "";
    let range: { from: number; to: number } | null = null;
    if (plugin && lspReady()) {
      plugin.client.sync();
      try {
        // The value shows even when the server is slow or restarting.
        const res = await Promise.race([
          plugin.client.request<lsp.HoverParams, lsp.Hover | null>("textDocument/hover", {
            textDocument: { uri: plugin.uri },
            position: plugin.toPosition(pos),
          }),
          new Promise<null>((resolve) => setTimeout(() => resolve(null), HOVER_WAIT_MS)),
        ]);
        if (res) {
          const contents = Array.isArray(res.contents) ? res.contents : [res.contents];
          html = contents.map((c) => plugin.docToHTML(typeof c === "string" || "kind" in c ? c : c.value)).join("");
          if (res.range) range = { from: plugin.fromPosition(res.range.start, plugin.syncedDoc), to: plugin.fromPosition(res.range.end, plugin.syncedDoc) };
        }
      } catch {
        // no server answer: the value alone
      }
    }
    const name = variableAt(view, pos);
    const value = name ? await describeVariable(file, name) : null;
    if (!html && !value) return null;
    return {
      pos: range?.from ?? pos,
      end: range?.to ?? pos,
      above: true,
      create() {
        const dom = document.createElement("div");
        dom.className = "cm-sonde-hover";
        if (html) {
          const doc = document.createElement("div");
          doc.className = "cm-sonde-hover-doc";
          doc.innerHTML = html; // sanitized by the client (sanitize.ts)
          dom.append(doc);
        }
        if (value) {
          const row = document.createElement("div");
          row.className = "cm-sonde-hover-value";
          const code = document.createElement("code");
          code.textContent = value.value || '""';
          const source = document.createElement("span");
          source.textContent = value.source;
          row.append(code, source);
          dom.append(row);
        }
        return { dom };
      },
    };
  });
}
