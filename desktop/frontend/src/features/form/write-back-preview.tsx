// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The source the form writes for the current request, as a unified diff
// against the request as it was when the form first showed it: exactly
// the lines that will be saved, comments kept.

import { unifiedMergeView } from "@codemirror/merge";
import { EditorState, RangeSetBuilder } from "@codemirror/state";
import { Decoration, EditorView } from "@codemirror/view";
import { useEffect, useRef, useState } from "react";
import { sondeLanguage } from "../../lang";
import { useTabs } from "../../state/tabs";
import { methodColors } from "../editor/methods";
import { editorHighlight, editorTheme } from "../editor/theme";
import { lineOf, type EntryModel } from "./model";

/** Each request's text when the form first showed it, by file and
 * entry, and how many requests the file had then (a request added or
 * removed moves the others: the baselines start again). */
const originals = new Map<string, { count: number; texts: Map<number, string> }>();

/** What the preview shows for a tab: its title and note, the part of
 * the request the tab writes, and the lines it marks. */
export interface WriteBackFocus {
  title?: string;
  sub?: string;
  /** "response": from the HTTP line on; "request": the lines before it. */
  part?: "request" | "response";
  mark?: (line: string) => boolean;
}

/** The part of a request's text the preview shows, and where it starts
 * (in characters). */
export function partOf(text: string, part: WriteBackFocus["part"]): { text: string; at: number } {
  if (!part) return { text, at: 0 };
  // The response line: HTTP, a version maybe, a status or *.
  const m = /^HTTP(\/[\d.]+)?[ \t]+(\d{3}|\*)[ \t]*$/m.exec(text);
  const cut = m ? m.index : text.length;
  return part === "request" ? { text: text.slice(0, cut), at: 0 } : { text: text.slice(cut), at: cut };
}

const marked = Decoration.line({ class: "wb-mark" });

export function WriteBackPreview({ file, entry, count, version, focus = {} }: { file: string; entry: EntryModel; count: number; version: number; focus?: WriteBackFocus }) {
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
  const cur = partOf(shown.current, focus.part);
  const current = cur.text;
  const original = partOf(shown.original, focus.part).text;
  const { mark } = focus;
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
          methodColors,
          unifiedMergeView({ original, mergeControls: false, gutter: false }),
          EditorView.contentAttributes.of({ "aria-label": "Lines written to the file" }),
          EditorView.decorations.of((view) => {
            const b = new RangeSetBuilder<Decoration>();
            if (mark) {
              for (let n = 1; n <= view.state.doc.lines; n++) {
                const line = view.state.doc.line(n);
                if (mark(line.text)) b.add(line.from, line.from, marked);
              }
            }
            return b.finish();
          }),
        ],
      }),
    });
    return () => v.destroy();
  }, [current, original, mark]);
  const start = entry.Range.Start + cur.at;
  const from = lineOf(text, start);
  const to = focus.part ? lineOf(text, start + current.replace(/\n+$/, "").length) : lineOf(text, entry.Range.End);
  return (
    <section className="write-back" aria-label="Write-back preview">
      <div className="section-note">
        <span>{focus.title ?? `Writes to ${file}`}</span>
        <span className="sub">{focus.sub ?? `lines ${from}–${to} · comments kept`}</span>
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
