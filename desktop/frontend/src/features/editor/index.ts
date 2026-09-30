// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Text view: the editor, its commands (send at cursor, run to cursor,
// format, comment, go to line) and its toolbar buttons.

import { toggleComment } from "@codemirror/commands";
import { formatDocument } from "@codemirror/lsp-client";
import { EditorSelection } from "@codemirror/state";
import { registry } from "../../app/registry";
import { ask } from "../../components/ask";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { TextEditor } from "./editor-view";
import { entryAt } from "./entry-at";
import { FormatButton, RunToCursorButton } from "./toolbar";
import { activeView } from "./views";
import "./editor.css";

registry.editor({ id: "text", title: "Text", order: 0, render: TextEditor });

/** The request at the active editor's cursor, or 0 (with a hint). */
export async function requestAtCursor(): Promise<{ path: string; entry: number } | null> {
  const a = activeView();
  if (!a) return null;
  const tab = useTabs.getState().tabs.find((t) => t.path === a.path);
  if (!tab) return null;
  const entry = await entryAt({ file: a.path, text: tab.text, version: tab.version }, a.view.state.selection.main.head);
  if (!entry) {
    useUI.getState().toast({ kind: "info", text: "Put the cursor in a request first" });
    return null;
  }
  return { path: a.path, entry };
}

const hasEditor = () => activeView() !== null;
const idle = () => {
  const path = useTabs.getState().active;
  return !!path && !useRuns.getState().runs[path]?.running;
};

registry.command({
  id: "request.send",
  title: "Send request at cursor",
  hint: "reuses the earlier requests' captures",
  when: () => hasEditor() && idle(),
  run: async () => {
    const at = await requestAtCursor();
    if (at) await useRuns.getState().send(at.path, at.entry);
  },
});

registry.command({
  id: "request.runTo",
  title: "Run requests up to the cursor",
  when: () => hasEditor() && idle(),
  run: async () => {
    const at = await requestAtCursor();
    if (at) await useRuns.getState().run(at.path, at.entry);
  },
});

registry.command({
  id: "editor.format",
  title: "Format file",
  when: hasEditor,
  run: () => {
    const a = activeView();
    if (a) formatDocument(a.view);
  },
});

registry.command({
  id: "editor.toggleComment",
  title: "Toggle comment",
  hidden: true,
  // It edits where the cursor is: only while the editor has the focus.
  when: () => activeView()?.view.hasFocus ?? false,
  run: () => {
    const a = activeView();
    if (a) toggleComment(a.view);
  },
});

registry.command({
  id: "editor.gotoLine",
  title: "Go to line",
  when: hasEditor,
  run: async (arg) => {
    const a = activeView();
    if (!a) return;
    let line = typeof arg === "number" ? arg : 0;
    if (!line) {
      const answer = await ask({ title: "Go to line", label: "Line number", submit: "Go" });
      line = Number(answer);
    }
    if (!Number.isInteger(line) || line < 1) return;
    const doc = a.view.state.doc;
    const target = doc.line(Math.min(line, doc.lines));
    a.view.dispatch({ selection: EditorSelection.cursor(target.from), scrollIntoView: true });
    a.view.focus();
  },
});

registry.toolbarItem({ id: "editor.format", order: 40, render: FormatButton });
registry.toolbarItem({ id: "editor.runTo", order: 90, render: RunToCursorButton });
