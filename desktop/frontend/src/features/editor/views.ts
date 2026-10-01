// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// One editor per open tab, alive while the tab is open (its undo history,
// folds and scroll stay). The tab's text is the truth for runs: every edit
// goes to the tab store, and a text changed there (a reload from disk, an
// edit from Go) replaces the editor's.

import { closeBrackets } from "@codemirror/autocomplete";
import { history, undo } from "@codemirror/commands";
import { bracketMatching, foldGutter, indentUnit } from "@codemirror/language";
import { highlightSelectionMatches } from "@codemirror/search";
import { EditorState } from "@codemirror/state";
import { drawSelection, EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from "@codemirror/view";
import { sondeLanguage } from "../../lang";
import { registerEditTarget } from "../../state/edits";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { pasteAsCurl } from "./paste-curl";
import { dropModel, modelOf } from "./entry-at";
import { editorKeymap, ownKeys } from "./keymap";
import { fileURI, lspClient } from "./lsp/client";
import { sondeHover } from "./lsp/hover-decorate";
import { sondeCompletion } from "./lsp/variable-completion";
import { lspSync } from "./lsp/sync";
import { methodColors } from "./methods";
import { runGutter, setRequestMarks, setRunMarks } from "./results/decorations";
import { requestMarks, runMarks, type EntryShape } from "./results/marks";
import { editorHighlight, editorTheme } from "./theme";

const views = new Map<string, EditorView>();
/** Unregisters each editor as the target of Go's edits of its tab. */
const unregister = new Map<string, () => void>();
/** The text each editor last exchanged with the tab store. */
const synced = new WeakMap<EditorView, string>();

export function viewOf(path: string): EditorView | undefined {
  return views.get(path);
}

/** The editor of the active tab, if it is open. */
export function activeView(): { path: string; view: EditorView } | null {
  const path = useTabs.getState().active;
  const view = path ? views.get(path) : undefined;
  return path && view ? { path, view } : null;
}

function publishCursor(view: EditorView) {
  const head = view.state.selection.main.head;
  const line = view.state.doc.lineAt(head);
  const col = head - line.from + 1;
  const had = useUI.getState().cursor;
  if (had?.line !== line.number || had.col !== col) useUI.setState({ cursor: { line: line.number, col } });
}

/** Replaces an editor's text with next, changing only the part that
 * differs (the cursor and marks outside it stay). */
function replaceText(view: EditorView, next: string, userEvent?: string) {
  const cur = view.state.doc.toString();
  let from = 0;
  const max = Math.min(cur.length, next.length);
  while (from < max && cur.charCodeAt(from) === next.charCodeAt(from)) from++;
  let end = 0;
  while (end < max - from && cur.charCodeAt(cur.length - 1 - end) === next.charCodeAt(next.length - 1 - end)) end++;
  view.dispatch({ changes: { from, to: cur.length - end, insert: next.slice(from, next.length - end) }, userEvent, scrollIntoView: !!userEvent });
}

function extensions(path: string) {
  return [
    runGutter((entry) => void useRuns.getState().run(path, entry)),
    lineNumbers(),
    foldGutter({ markerDOM: (open) => Object.assign(document.createElement("span"), { textContent: open ? "▾" : "▸", className: "cm-fold-mark" }) }),
    highlightActiveLine(),
    highlightActiveLineGutter(),
    history(),
    drawSelection(),
    bracketMatching(),
    closeBrackets(),
    highlightSelectionMatches(),
    indentUnit.of("  "),
    sondeLanguage(),
    editorTheme,
    editorHighlight,
    methodColors,
    ownKeys,
    keymap.of(editorKeymap),
    lspClient().plugin(fileURI(path), "sonde"),
    // Completion: the app's variables inside {{…}}, the server's elsewhere.
    sondeCompletion(path),
    sondeHover(path),
    lspSync,
    EditorView.contentAttributes.of({ "aria-label": `${path} text` }),
    // A curl command pasted into an empty file goes through the import,
    // which writes it as a request.
    EditorView.domEventHandlers({
      paste: (e, view) => {
        const text = e.clipboardData?.getData("text/plain") ?? "";
        if (view.state.doc.toString().trim() !== "" || !/^\s*curl\s/.test(text)) return false;
        e.preventDefault();
        void pasteAsCurl(text);
        return true;
      },
    }),
  ];
}

/** The editor of a tab, created on first use. */
export function openView(path: string): EditorView {
  const had = views.get(path);
  if (had) return had;
  const text = useTabs.getState().tabs.find((t) => t.path === path)?.text ?? "";
  const view = new EditorView({
    state: EditorState.create({ doc: text, extensions: extensions(path) }),
    dispatch: (tr, v) => {
      v.update([tr]);
      if (tr.docChanged) {
        const next = v.state.doc.toString();
        synced.set(v, next);
        useTabs.getState().setText(path, next);
        scheduleRequestMarks(path);
      }
      if ((tr.docChanged || tr.selection) && useTabs.getState().active === path) publishCursor(v);
    },
  });
  synced.set(view, text);
  views.set(path, view);
  // Go's edits (an assert from the results…) are one change of the
  // editor: ⌘Z undoes them.
  unregister.set(
    path,
    registerEditTarget(path, {
      // The editor's line breaks are "\n" (see state/edits).
      apply: (text) => replaceText(view, text.replace(/\r\n?/g, "\n"), "input.edit"),
      undo: () => void undo(view),
    }),
  );
  scheduleRequestMarks(path, 0);
  refreshRunMarks(path);
  return view;
}

// The ▶ marks follow the requests as the text changes (Go's model, after
// a pause in typing).
const pending = new Map<string, ReturnType<typeof setTimeout>>();

function scheduleRequestMarks(path: string, delay = 300) {
  clearTimeout(pending.get(path));
  pending.set(
    path,
    setTimeout(async () => {
      pending.delete(path);
      const tab = useTabs.getState().tabs.find((t) => t.path === path);
      const view = views.get(path);
      if (!tab || !view) return;
      const model = await modelOf({ file: path, text: tab.text, version: tab.version });
      // A newer edit schedules its own refresh; a text that does not parse
      // keeps the marks set earlier.
      if (!model || views.get(path) !== view || view.state.doc.toString() !== tab.text) return;
      const doc = view.state.doc;
      view.dispatch({ effects: setRequestMarks.of(requestMarks(model, (o) => doc.lineAt(Math.min(o, doc.length)).number)) });
      refreshRunMarks(path, model);
    }, delay),
  );
}

/** Sets a file's run results, when its editor holds the text that ran;
 * otherwise the marks set earlier stay, moved with the edits. */
async function refreshRunMarks(path: string, known?: EntryShape[]) {
  const view = views.get(path);
  const run = useRuns.getState().runs[path];
  if (!view) return;
  const text = view.state.doc.toString();
  if (!run) {
    view.dispatch({ effects: setRunMarks.of([]) });
    return;
  }
  if (run.source !== text) return;
  const tab = useTabs.getState().tabs.find((t) => t.path === path);
  const model = known ?? (tab ? await modelOf({ file: path, text, version: tab.version }) : null);
  if (!model || views.get(path) !== view || view.state.doc.toString() !== text) return;
  const doc = view.state.doc;
  view.dispatch({ effects: setRunMarks.of(runMarks(model, (o) => doc.lineAt(Math.min(o, doc.length)).number, run)) });
}

// Runs: results arrive as events; one refresh per frame.
let frame = 0;
const dirtyRuns = new Set<string>();
useRuns.subscribe((s, prev) => {
  for (const path of views.keys()) if (s.runs[path] !== prev.runs[path]) dirtyRuns.add(path);
  if (dirtyRuns.size === 0 || frame) return;
  frame = requestAnimationFrame(() => {
    frame = 0;
    for (const path of dirtyRuns) void refreshRunMarks(path);
    dirtyRuns.clear();
  });
});

// Tabs: a text changed outside the editor replaces it; a closed tab's
// editor goes away.
useTabs.subscribe((s) => {
  for (const [path, view] of views) {
    const tab = s.tabs.find((t) => t.path === path);
    if (!tab) {
      view.destroy();
      views.delete(path);
      unregister.get(path)?.();
      unregister.delete(path);
      dropModel(path);
      continue;
    }
    if (tab.text !== synced.get(view)) {
      synced.set(view, tab.text);
      replaceText(view, tab.text);
    }
  }
  const active = s.active ? views.get(s.active) : undefined;
  if (active) publishCursor(active);
  else if (useUI.getState().cursor) useUI.setState({ cursor: null });
});
