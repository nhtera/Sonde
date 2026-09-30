// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A body's text as received (read-only editor): highlighted as JSON or
// XML, with the panel's search marking every match.

import { SearchCursor } from "@codemirror/search";
import { EditorState, StateEffect, StateField, type Extension } from "@codemirror/state";
import { Decoration, EditorView, lineNumbers, type DecorationSet } from "@codemirror/view";
import { useEffect, useRef } from "react";
import { jsonBodyLanguage, xmlBodyLanguage } from "../../../../lang";
import { editorHighlight, editorTheme } from "../../../editor/theme";
import type { BodyKind } from "../../model";
import { MAX_MATCHES } from "./body-search";

const setMatches = StateEffect.define<{ ranges: { from: number; to: number }[]; current: number }>();
const matchMark = Decoration.mark({ class: "cm-body-match" });
const currentMark = Decoration.mark({ class: "cm-body-match cm-body-current" });

const matchField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) {
      if (e.is(setMatches)) {
        deco = Decoration.set(e.value.ranges.map((r, i) => (i === e.value.current ? currentMark : matchMark).range(r.from, r.to)));
      }
    }
    return deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

function language(kind: BodyKind): Extension {
  if (kind === "json") return jsonBodyLanguage;
  if (kind === "xml" || kind === "html") return xmlBodyLanguage;
  return [];
}

export interface RawViewProps {
  text: string;
  kind: BodyKind;
  query: string;
  nav: { n: number; step: number };
  onMatches(current: number, total: number): void;
}

export function RawView({ text, kind, query, nav, onMatches }: RawViewProps) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const found = useRef<{ from: number; to: number }[]>([]);
  const current = useRef(-1);
  const handled = useRef(nav.n);

  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: text,
        extensions: [
          EditorState.readOnly.of(true),
          lineNumbers(),
          language(kind),
          editorTheme,
          editorHighlight,
          matchField,
          EditorView.contentAttributes.of({ "aria-label": "Raw body" }),
        ],
      }),
    });
    view.current = v;
    return () => {
      v.destroy();
      view.current = null;
    };
  }, [text, kind]);

  const show = (i: number) => {
    const v = view.current;
    if (!v) return;
    current.current = i;
    const r = found.current[i];
    v.dispatch({
      effects: [setMatches.of({ ranges: found.current, current: i }), ...(r ? [EditorView.scrollIntoView(r.from, { y: "center" })] : [])],
    });
    onMatches(i, found.current.length);
  };

  useEffect(() => {
    const v = view.current;
    if (!v) return;
    const ranges: { from: number; to: number }[] = [];
    if (query) {
      const cursor = new SearchCursor(v.state.doc, query, 0, v.state.doc.length, (s) => s.toLowerCase());
      while (!cursor.next().done && ranges.length < MAX_MATCHES) ranges.push({ from: cursor.value.from, to: cursor.value.to });
    }
    found.current = ranges;
    show(ranges.length ? 0 : -1);
    // show only reads refs and the latest onMatches.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, text, kind]);

  useEffect(() => {
    const n = found.current.length;
    // Each Enter moves once (not on mount, not when the matches change).
    if (nav.n === handled.current || n === 0) return;
    handled.current = nav.n;
    show((current.current + nav.step + n) % n);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nav]);

  return <div className="raw-view" ref={host} />;
}
