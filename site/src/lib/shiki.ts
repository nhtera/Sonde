// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { LanguageRegistration, ThemeRegistration } from "shiki";
import sondeGrammar from "../../../editors/vscode/syntaxes/sonde.tmLanguage.json" with { type: "json" };

// Code colors are CSS variables from the desktop tokens (--t-*), so one
// theme serves dark and light and follows the toggle with no re-render.
// The scope → token mapping mirrors desktop/frontend/src/features/editor/theme.ts.
const v = (name: string) => `var(--${name})`;

export const sondeTheme: ThemeRegistration = {
  name: "sonde",
  type: "dark",
  colors: {
    "editor.foreground": v("text"),
    "editor.background": v("bg"),
  },
  fg: v("text"),
  bg: v("bg"),
  tokenColors: [
    { scope: ["comment", "punctuation.definition.comment"], settings: { foreground: v("t-com"), fontStyle: "italic" } },
    { scope: ["string", "string.regexp", "markup.inline.raw"], settings: { foreground: v("t-str") } },
    { scope: ["constant.numeric", "constant.language", "constant.character", "constant.other"], settings: { foreground: v("t-num") } },
    // Hurl: methods, HTTP version and status line.
    { scope: ["keyword.control.method", "keyword.control.version", "constant.numeric.http-version", "constant.numeric.status"], settings: { foreground: v("t-http") } },
    { scope: ["entity.name.section", "punctuation.definition.section"], settings: { foreground: v("t-sec") } },
    { scope: ["entity.name.tag", "support.type.property-name", "meta.object-literal.key", "variable.other.property"], settings: { foreground: v("t-key") } },
    { scope: ["meta.embedded.line.placeholder", "variable.other.readwrite.sonde", "punctuation.definition.template-expression"], settings: { foreground: v("t-var") } },
    { scope: ["support.function.query"], settings: { foreground: v("t-q") } },
    { scope: ["support.function", "entity.name.function", "entity.other.attribute-name"], settings: { foreground: v("t-f") } },
    { scope: ["keyword.operator.predicate", "keyword.other.modifier", "keyword.operator"], settings: { foreground: v("t-op") } },
    { scope: ["keyword", "storage", "storage.type"], settings: { foreground: v("t-sec") } },
    { scope: ["punctuation", "meta.brace"], settings: { foreground: v("muted") } },
  ],
};

// The repository's own grammar (the VS Code extension's), so docs code
// highlights the way the editor and the desktop app do.
export const hurlLanguage = {
  ...(sondeGrammar as unknown as LanguageRegistration),
  name: "hurl",
  aliases: ["sonde"],
} as LanguageRegistration;
