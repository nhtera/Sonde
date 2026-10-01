// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// One hover. A {{variable}} the app knows gets a card: its name and
// kind, its value (*** for a secret or a redacted capture), where it
// comes from and, for a capture, a link to the line that sets it. Any
// other hover is the language server's.

import { syntaxTree } from "@codemirror/language";
import type { SyntaxNode } from "@lezer/common";
import { LSPPlugin } from "@codemirror/lsp-client";
import { EditorView, hoverTooltip, type Tooltip } from "@codemirror/view";
import type * as lsp from "vscode-languageserver-protocol";
import { useEnv } from "../../../state/env";
import { describeVariable, type VariableInfo } from "../variables";
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
    const info = name ? await describeVariable(file, name, pos) : null;
    if (!html && !info) return null;
    return {
      pos: range?.from ?? pos,
      end: range?.to ?? pos,
      above: true,
      create(view) {
        const dom = document.createElement("div");
        dom.className = "cm-sonde-hover";
        if (info) dom.append(variableCard(view, info));
        else {
          const doc = document.createElement("div");
          doc.className = "cm-sonde-hover-doc";
          doc.innerHTML = html; // sanitized by the client (sanitize.ts)
          dom.append(doc);
        }
        return { dom };
      },
    };
  });
}

const el = (tag: string, className: string, text?: string) => Object.assign(document.createElement(tag), { className, textContent: text ?? "" });

/** Where a variable comes from, in a sentence. */
function origin(info: VariableInfo): string {
  if (info.setBy) return `Set by request ${info.setBy.entry} · line ${info.setBy.line} · ${info.captured ? "last run" : "not run yet"}`;
  if (info.kind === "override") return "Session override, used as --variable";
  const env = useEnv.getState().current;
  return `${env ? `Environment ${env} · ` : ""}${info.source}`;
}

/** The card of a variable: name and kind, value, origin and a link. */
export function variableCard(view: EditorView, info: VariableInfo): HTMLElement {
  const card = el("div", "cm-var-card");
  const head = el("div", "cm-var-head");
  head.append(el("code", "cm-var-name", `{{${info.name}}}`), el("span", `cm-var-kind k-${info.kind}`, info.kind === "project" ? "environment" : info.kind));
  const value = el("code", "cm-var-value", info.value || (info.kind === "capture" ? "not captured yet" : '""'));
  if (!info.value) value.classList.add("unset");
  const foot = el("div", "cm-var-foot");
  foot.append(el("span", "", origin(info)));
  if (info.setBy) {
    const line = info.setBy.line;
    const go = el("button", "cm-var-link", "Go to capture") as HTMLButtonElement;
    go.type = "button";
    go.onmousedown = (e) => e.preventDefault();
    go.onclick = () => {
      const at = view.state.doc.line(Math.min(line, view.state.doc.lines));
      view.dispatch({ selection: { anchor: at.from }, effects: EditorView.scrollIntoView(at.from, { y: "center" }) });
      view.focus();
    };
    foot.append(go);
  }
  card.append(head, value, foot);
  return card;
}
