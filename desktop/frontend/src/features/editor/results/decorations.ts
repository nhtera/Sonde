// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The run's marks in the editor: a gutter column before the line numbers
// and ghost texts at the ends of lines, set by an effect and moved with
// the edits made since. The language server's warnings add ⚠ and their
// message the same way.

import { forEachDiagnostic } from "@codemirror/lint";
import { RangeSet, StateEffect, StateField, type EditorState, type Extension } from "@codemirror/state";
import { Decoration, EditorView, gutter, GutterMarker, WidgetType, type DecorationSet } from "@codemirror/view";
import type { LineMark } from "./marks";

const symbols = { run: "▸", pass: "✓", fail: "✕", capture: "◆", skip: "–", warn: "⚠" } as const;
type Symbol = keyof typeof symbols;

class Mark extends GutterMarker {
  constructor(
    readonly kind: Symbol,
    readonly entry: number | undefined,
    readonly dim: boolean,
  ) {
    super();
  }
  override eq(other: Mark) {
    return other.kind === this.kind && other.entry === this.entry && other.dim === this.dim;
  }
  override toDOM() {
    const el = document.createElement("span");
    el.className = `cm-run-mark cm-run-${this.kind}${this.dim ? " cm-run-dim" : ""}`;
    el.textContent = symbols[this.kind];
    if (this.kind === "run") el.title = `Run requests 1–${this.entry}`;
    return el;
  }
}

class Ghost extends WidgetType {
  constructor(
    readonly text: string,
    readonly kind: string,
    readonly dim: boolean,
  ) {
    super();
  }
  override eq(other: Ghost) {
    return other.text === this.text && other.kind === this.kind && other.dim === this.dim;
  }
  toDOM() {
    const el = document.createElement("span");
    el.className = `cm-ghost cm-ghost-${this.kind}${this.dim ? " cm-run-dim" : ""}`;
    el.textContent = this.text;
    return el;
  }
  override ignoreEvent() {
    return true;
  }
}

interface Marks {
  gutter: RangeSet<Mark>;
  ghosts: DecorationSet;
  lines: DecorationSet;
}

/** A warning's text, short enough for the end of a line: a long one
 * keeps its headline (before the first ": "); hovering shows it all. */
export function shorten(s: string, max = 60): string {
  if (s.length > max && s.indexOf(": ") > 0) s = s.slice(0, s.indexOf(": "));
  return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

/** Replaces the run's results (lines of the current document). */
export const setRunMarks = StateEffect.define<LineMark[]>();

/** Replaces the ▶ marks of the request lines. */
export const setRequestMarks = StateEffect.define<LineMark[]>();

function build(state: EditorState, marks: LineMark[]): Marks {
  const gutter: { from: number; value: Mark }[] = [];
  const ghosts: { from: number; value: Decoration }[] = [];
  const lines: { from: number; value: Decoration }[] = [];
  for (const m of marks) {
    if (m.line < 1 || m.line > state.doc.lines) continue;
    const line = state.doc.line(m.line);
    gutter.push({ from: line.from, value: new Mark(m.gutter, m.entry, !!m.dim) });
    // The ghost belongs to its line (drawn after it by CSS): Enter at the
    // line's end leaves it there.
    const classes = [m.ghost ? `cm-ghosted cm-ghost-${m.ghostKind ?? "muted"}` : "", m.dim ? "cm-ghost-dim" : "", m.gutter === "fail" && !m.dim ? "cm-fail-line" : ""].filter(Boolean);
    if (classes.length) {
      const spec = m.ghost ? { class: classes.join(" "), attributes: { "data-ghost": m.ghost } } : { class: classes.join(" ") };
      (m.ghost ? ghosts : lines).push({ from: line.from, value: Decoration.line(spec) });
    }
  }
  return {
    gutter: RangeSet.of(gutter.map((g) => g.value.range(g.from)), true),
    ghosts: Decoration.set(ghosts.map((g) => g.value.range(g.from)), true),
    lines: Decoration.set(lines.map((l) => l.value.range(l.from)), true),
  };
}

/** Marks set by effect, then moved with each edit. */
function marksField(effect: typeof setRunMarks) {
  return StateField.define<Marks>({
    create: () => ({ gutter: RangeSet.empty, ghosts: Decoration.none, lines: Decoration.none }),
    update(value, tr) {
      for (const e of tr.effects) if (e.is(effect)) return build(tr.state, e.value);
      if (!tr.docChanged) return value;
      return { gutter: value.gutter.map(tr.changes), ghosts: value.ghosts.map(tr.changes), lines: value.lines.map(tr.changes) };
    },
    provide: (f) => [EditorView.decorations.from(f, (m) => m.ghosts), EditorView.decorations.from(f, (m) => m.lines)],
  });
}

const runMarksField = marksField(setRunMarks);
const requestMarksField = marksField(setRequestMarks);

/** Whether the cursor sits in a {{…}} still being typed on its line: its
 * parse error is not news yet. */
export function typingTemplate(state: EditorState): number | null {
  const pos = state.selection.main.head;
  const line = state.doc.lineAt(pos);
  const before = line.text.slice(0, pos - line.from);
  const open = before.lastIndexOf("{{");
  if (open < 0 || before.indexOf("}}", open) >= 0) return null;
  return /^[\s\w.-]*(\}\}|$)/.test(line.text.slice(pos - line.from)) ? line.number : null;
}

/** The language server's warnings and errors: ⚠ in the gutter and the
 * first message of each line as a ghost (not on a line whose {{…}} is
 * being typed). */
const diagnosticMarks = StateField.define<{ gutter: RangeSet<Mark>; ghosts: DecorationSet }>({
  create: () => ({ gutter: RangeSet.empty, ghosts: Decoration.none }),
  update(value, tr) {
    if (!tr.docChanged && tr.effects.length === 0 && !tr.selection) return value;
    const typing = typingTemplate(tr.state);
    const seen = new Set<number>();
    const gutter: { from: number; value: Mark }[] = [];
    const ghosts: { from: number; value: Decoration }[] = [];
    forEachDiagnostic(tr.state, (d, from) => {
      if (d.severity !== "warning" && d.severity !== "error") return;
      const line = tr.state.doc.lineAt(from);
      if (seen.has(line.number) || line.number === typing) return;
      seen.add(line.number);
      gutter.push({ from: line.from, value: new Mark("warn", undefined, false) });
      ghosts.push({ from: line.to, value: Decoration.widget({ widget: new Ghost(`⚠ ${shorten(d.message)}`, "warn", false), side: 2 }) });
    });
    // Nor its squiggle (editor.css).
    if (typing) ghosts.push({ from: tr.state.doc.line(typing).from, value: Decoration.line({ class: "cm-typing-template" }) });
    gutter.sort((a, b) => a.from - b.from);
    ghosts.sort((a, b) => a.from - b.from);
    return { gutter: RangeSet.of(gutter.map((g) => g.value.range(g.from))), ghosts: Decoration.set(ghosts.map((g) => g.value.range(g.from)), true) };
  },
  provide: (f) => EditorView.decorations.from(f, (m) => m.ghosts),
});

/** The marks gutter; clicking ▶ on a request line calls runTo(entry). */
export function runGutter(runTo: (entry: number) => void): Extension {
  return [
    runMarksField,
    requestMarksField,
    diagnosticMarks,
    gutter({
      class: "cm-run-gutter",
      // One mark shows per line: a result, then a warning, then ▶.
      markers: (view) =>
        RangeSet.join([view.state.field(runMarksField).gutter, view.state.field(diagnosticMarks).gutter, view.state.field(requestMarksField).gutter]),
      domEventHandlers: {
        mousedown(view, line, event) {
          if ((event as MouseEvent).button !== 0) return false;
          let entry: number | undefined;
          for (const f of [runMarksField, requestMarksField]) {
            view.state.field(f).gutter.between(line.from, line.from, (_f, _t, m) => {
              entry = m.entry ?? entry;
            });
          }
          if (!entry) return false;
          runTo(entry);
          return true;
        },
      },
    }),
    EditorView.theme({
      ".cm-run-gutter .cm-gutterElement": { width: "18px", textAlign: "center", fontSize: "10px", cursor: "default" },
      ".cm-run-gutter .cm-gutterElement > :not(:first-child)": { display: "none" },
      ".cm-run-run": { color: "var(--faint)", cursor: "pointer" },
      ".cm-run-run:hover": { color: "var(--accent)" },
      ".cm-run-pass": { color: "var(--pass)" },
      ".cm-run-fail": { color: "var(--fail)" },
      ".cm-run-capture": { color: "var(--t-var)" },
      ".cm-run-skip": { color: "var(--faint)" },
      ".cm-run-warn": { color: "var(--warn)" },
      ".cm-run-dim": { opacity: "0.45" },
      ".cm-ghost": { marginLeft: "16px", fontSize: "12px", pointerEvents: "none", whiteSpace: "pre" },
      ".cm-ghost-warn": { color: "var(--warn)" },
      ".cm-ghosted::after": { content: "attr(data-ghost)", marginLeft: "16px", fontSize: "12px", pointerEvents: "none", whiteSpace: "pre" },
      ".cm-ghost-muted::after": { color: "var(--faint)" },
      ".cm-ghost-capture::after": { color: "var(--t-str)", opacity: "0.8" },
      ".cm-ghost-fail::after": { color: "var(--fail)" },
      ".cm-ghost-dim::after": { opacity: "0.45" },
      ".cm-fail-line": { backgroundColor: "var(--fail-row)", boxShadow: "inset 2px 0 0 var(--fail)" },
    }),
  ];
}
