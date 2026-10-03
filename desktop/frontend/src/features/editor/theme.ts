// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The editor's look, from the design tokens (both themes follow the page's
// CSS variables, so a theme switch needs no reconfiguration).

import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";

export const editorTheme = EditorView.theme({
  "&": { height: "100%", backgroundColor: "var(--bg)", color: "var(--text)", fontSize: "var(--code-size, 13px)" },
  ".cm-scroller": { fontFamily: "var(--font-code)", lineHeight: "calc(var(--code-size, 13px) + 7px)" },
  ".cm-content": { padding: "8px 0", caretColor: "var(--accent)" },
  "&.cm-focused": { outline: "none" },
  ".cm-cursor": { borderLeftColor: "var(--accent)" },
  ".cm-activeLine": { backgroundColor: "var(--curline)" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection": { backgroundColor: "var(--accent-soft) !important" },
  ".cm-gutters": { backgroundColor: "var(--bg)", color: "var(--faint)", border: "none" },
  ".cm-lineNumbers .cm-gutterElement": { padding: "0 14px 0 6px", minWidth: "32px" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--muted)" },
  ".cm-foldGutter .cm-gutterElement": { color: "var(--faint)" },
  ".cm-tooltip": { backgroundColor: "var(--raised)", border: "none", borderRadius: "10px", boxShadow: "var(--pop), inset 0 0 0 1px var(--line)", color: "var(--text)" },
  ".cm-tooltip-autocomplete > ul > li[aria-selected]": { backgroundColor: "var(--accent-soft)", color: "var(--text)" },
  ".cm-diagnostic-warning": { borderLeftColor: "var(--warn)" },
  ".cm-diagnostic-error": { borderLeftColor: "var(--fail)" },
  ".cm-lintRange-warning": { backgroundImage: "none", textDecoration: "underline wavy var(--warn-line)", textUnderlineOffset: "3px" },
  ".cm-lintRange-error": { backgroundImage: "none", textDecoration: "underline wavy var(--fail-line)", textUnderlineOffset: "3px" },
  ".cm-foldPlaceholder": { backgroundColor: "var(--raised)", border: "none", color: "var(--muted)" },
  // CodeMirror's own parts have light colors of their own: the tokens'
  // instead, in every theme.
  ".cm-tooltip-section:not(:first-child)": { borderTopColor: "var(--line)" },
  ".cm-tooltip .cm-tooltip-arrow:before": { borderTopColor: "var(--line)", borderBottomColor: "var(--line)" },
  ".cm-tooltip .cm-tooltip-arrow:after": { borderTopColor: "var(--raised)", borderBottomColor: "var(--raised)" },
  ".cm-panels": { backgroundColor: "var(--panel)", color: "var(--text)" },
  ".cm-panels-top": { borderBottom: "1px solid var(--line)" },
  ".cm-panels-bottom": { borderTop: "1px solid var(--line)" },
  ".cm-panel.cm-search label": { color: "var(--muted)" },
  ".cm-button": { backgroundImage: "none", backgroundColor: "var(--raised)", border: "1px solid var(--line)", borderRadius: "var(--r-chip)", color: "var(--text)" },
  ".cm-textfield": { backgroundColor: "var(--bg)", border: "1px solid var(--line)", borderRadius: "var(--r-chip)", color: "var(--text)" },
  ".cm-searchMatch": { backgroundColor: "var(--warn-soft)", outline: "1px solid var(--warn-line)" },
  ".cm-searchMatch-selected": { backgroundColor: "var(--accent-soft)", outline: "1px solid var(--accent-line)" },
  ".cm-panel.cm-lint": { backgroundColor: "var(--panel)", color: "var(--text)" },
  ".cm-panel.cm-lint ul [aria-selected]": { backgroundColor: "var(--accent-soft)" },
  ".cm-diagnostic": { color: "var(--text)" },
  ".cm-snippetField": { backgroundColor: "var(--accent-soft)" },
  ".cm-specialChar": { color: "var(--fail)" },
});

/** Syntax colors: {{variables}} stand out, the rest stays calm. */
export const editorHighlight = syntaxHighlighting(
  HighlightStyle.define([
    { tag: t.keyword, color: "var(--t-http)" },
    { tag: t.url, color: "var(--text)" },
    { tag: t.special(t.variableName), color: "var(--t-var)", backgroundColor: "var(--accent-soft)", borderRadius: "3px" },
    { tag: t.heading, color: "var(--t-sec)" },
    { tag: t.propertyName, color: "var(--t-key)" },
    { tag: t.function(t.variableName), color: "var(--t-q)" },
    { tag: t.function(t.propertyName), color: "var(--t-f)" },
    { tag: [t.operatorKeyword, t.compareOperator], color: "var(--t-op)" },
    { tag: t.modifier, color: "var(--t-op)" },
    { tag: [t.string, t.special(t.string)], color: "var(--t-str)" },
    { tag: t.regexp, color: "var(--t-str)" },
    { tag: t.escape, color: "var(--t-num)" },
    { tag: [t.number, t.bool, t.null], color: "var(--t-num)" },
    { tag: t.lineComment, color: "var(--t-com)", fontStyle: "italic" },
    { tag: [t.tagName, t.angleBracket], color: "var(--t-key)" },
    { tag: t.attributeName, color: "var(--t-f)" },
    { tag: t.attributeValue, color: "var(--t-str)" },
  ]),
);
