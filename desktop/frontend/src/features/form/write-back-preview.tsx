// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The source the form writes for the current request, as a unified diff
// against the request as it was when the form first showed it: exactly
// the lines that will be saved, comments kept.

import { unifiedMergeView } from "@codemirror/merge";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { useEffect, useRef, useState } from "react";
import { sondeLanguage } from "../../lang";
import { useTabs } from "../../state/tabs";
import { editorHighlight, editorTheme } from "../editor/theme";
import { lineOf, type EntryModel } from "./model";

/** Each request's text when the form first showed it, by file and
 * entry, and how many requests the file had then (a request added or
 * removed moves the others: the baselines start again). */
const originals = new Map<string, { count: number; texts: Map<number, string> }>();

export function WriteBackPreview({ file, entry, count, version }: { file: string; entry: EntryModel; count: number; version: number }) {
  const tab = useTabs((s) => s.tabs.find((t) => t.path === file));
  const text = tab?.text ?? "";
  // The model's ranges hold for the text it was read from only: until it
  // catches up, the preview keeps what it showed.
  const [shown, setShown] = useState({ current: "", original: "" });
  if (tab?.version === version) {
    let o = originals.get(file);
    if (!o || o.count !== count) {
      o = { count, texts: new Map() };
      originals.set(file, o);
    }
    // Offsets are UTF-16, as JS strings.
    const current = text.slice(entry.Range.Start, entry.Range.End);
    if (!o.texts.has(entry.Index)) o.texts.set(entry.Index, current);
    const original = o.texts.get(entry.Index)!;
    if (shown.current !== current || shown.original !== original) setShown({ current, original });
  }
  const { current, original } = shown;
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: current,
        extensions: [
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          sondeLanguage(),
          editorTheme,
          editorHighlight,
          unifiedMergeView({ original, mergeControls: false, gutter: true }),
          EditorView.contentAttributes.of({ "aria-label": "Lines written to the file" }),
        ],
      }),
    });
    return () => v.destroy();
  }, [current, original]);
  const from = lineOf(text, entry.Range.Start);
  const to = lineOf(text, entry.Range.End);
  return (
    <section className="write-back" aria-label="Write-back preview">
      <div className="section-note">
        <span>Writes to {file}</span>
        <span>
          lines {from}–{to} · comments kept
        </span>
      </div>
      <div ref={host} />
    </section>
  );
}

// A saved or reloaded file starts the diffs again.
useTabs.subscribe((s, prev) => {
  for (const t of s.tabs) {
    const before = prev.tabs.find((p) => p.path === t.path);
    if (before && before.savedText !== t.savedText) originals.delete(t.path);
  }
});
