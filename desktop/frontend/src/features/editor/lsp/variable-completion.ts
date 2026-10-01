// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Completion. Inside a {{…}} the list is the app's: the variables the
// request can use, each with where it comes from and its value (*** for a
// secret), the captures of earlier requests first, then the template
// functions; a footer says how to use it and how many match. Anywhere
// else it is the language server's.

import { autocompletion, currentCompletions, type Completion, type CompletionContext, type CompletionResult } from "@codemirror/autocomplete";
import { serverCompletionSource } from "@codemirror/lsp-client";
import { EditorView, ViewPlugin } from "@codemirror/view";
import { templateFunctions, variablesAt, type VariableInfo } from "../variables";

/** A {{ being typed, up to the cursor: the name typed so far. */
const openTemplate = /\{\{\s*([A-Za-z0-9_.-]*)$/;

interface VariableCompletion extends Completion {
  variable: VariableInfo;
}

const isVariable = (c: Completion): c is VariableCompletion => "variable" in c;

/** The variables offered at pos, as completions (functions last). */
export function variableOptions(vars: VariableInfo[], closed: boolean): VariableCompletion[] {
  const fns: VariableInfo[] = templateFunctions.map((name) => ({ name, kind: "function", value: "", source: "function" }));
  return [...vars, ...fns].map((v, i) => ({
    label: v.name,
    type: v.kind === "function" ? "function" : "variable",
    // The order is the list's: nearest capture first.
    boost: 99 - Math.min(i, 198),
    apply: closed ? v.name : `${v.name}}}`,
    variable: v,
  }));
}

/** How many variables the list had, for its footer. */
let listed = 0;

function sourceFor(file: string) {
  return async (ctx: CompletionContext): Promise<CompletionResult | null> => {
    const line = ctx.state.doc.lineAt(ctx.pos);
    const typed = openTemplate.exec(line.text.slice(0, ctx.pos - line.from));
    if (!typed) return serverCompletionSource(ctx);
    const vars = await variablesAt(file, ctx.pos);
    if (ctx.aborted) return null;
    const closed = /^\s*\}\}/.test(ctx.state.sliceDoc(ctx.pos, line.to));
    const options = variableOptions(vars, closed);
    listed = vars.length;
    return { from: ctx.pos - typed[1].length, options, validFor: /^[A-Za-z0-9_.-]*$/ };
  };
}

/** Where a variable comes from, right of its name (a secret's in amber). */
function renderSource(c: Completion): Node | null {
  if (!isVariable(c)) return null;
  return Object.assign(document.createElement("span"), { className: `cm-var-option-source k-${c.variable.kind}`, textContent: c.variable.source });
}

/** A variable's value under its name in the list. */
function renderValue(c: Completion): Node | null {
  if (!isVariable(c) || c.variable.kind === "function") return null;
  const v = c.variable;
  return Object.assign(document.createElement("div"), {
    className: `cm-var-option-value${v.value ? "" : " unset"}`,
    textContent: v.value || (v.kind === "capture" ? "not captured yet" : '""'),
  });
}

/** The footer's count: "5 of 9 variables". */
const footer = ViewPlugin.fromClass(
  class {
    update(u: { view: EditorView }) {
      const list = u.view.dom.querySelector<HTMLElement>(".cm-tooltip-autocomplete.cm-var-complete");
      if (!list) return;
      const shown = currentCompletions(u.view.state).filter((c) => isVariable(c) && c.variable.kind !== "function").length;
      list.dataset.count = `${shown} of ${listed} variable${listed === 1 ? "" : "s"}`;
    }
  },
);

export function sondeCompletion(file: string) {
  return [
    autocompletion({
      // Asks once typing pauses, not on every letter.
      activateOnTypingDelay: 250,
      override: [sourceFor(file)],
      addToOptions: [
        { render: renderSource, position: 70 },
        { render: renderValue, position: 90 },
      ],
      tooltipClass: (state) => (currentCompletions(state).some(isVariable) ? "cm-var-complete" : ""),
    }),
    footer,
  ];
}
