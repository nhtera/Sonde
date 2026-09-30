// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Each request's method in its own color (GET green, POST amber…), from
// the syntax tree of the visible lines.

import { syntaxTree } from "@codemirror/language";
import { RangeSetBuilder } from "@codemirror/state";
import { Decoration, ViewPlugin, type DecorationSet, type EditorView, type ViewUpdate } from "@codemirror/view";

function build(view: EditorView): DecorationSet {
  const b = new RangeSetBuilder<Decoration>();
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(view.state).iterate({
      from,
      to,
      enter: (n) => {
        if (n.name === "Method") b.add(n.from, n.to, Decoration.mark({ class: `cm-method m-${view.state.sliceDoc(n.from, n.to)}` }));
        // Nothing to color inside a body.
        return n.name !== "JsonBody" && n.name !== "XmlBody" && n.name !== "MultilineString";
      },
    });
  }
  return b.finish();
}

export const methodColors = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = build(view);
    }
    update(u: ViewUpdate) {
      if (u.docChanged || u.viewportChanged || syntaxTree(u.startState) !== syntaxTree(u.state)) this.decorations = build(u.view);
    }
  },
  { decorations: (p) => p.decorations },
);
