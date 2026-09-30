// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The .hurl/.sonde language: the file grammar (sonde.grammar) with each
// body parsed by its own language (JSON with templates, XML, GraphQL),
// highlight tags, and folding per entry, per section and per body.

import { foldNodeProp, LanguageSupport, LRLanguage } from "@codemirror/language";
import type { EditorState } from "@codemirror/state";
import { parseMixed, type SyntaxNode } from "@lezer/common";
import { styleTags, tags as t } from "@lezer/highlight";
import { parser as xmlParser } from "@lezer/xml";
import { graphqlLanguage } from "./graphql";
import { parser as jsonParser } from "./hurl-json.grammar";
import { parser as fileParser } from "./sonde.grammar";

const template = t.special(t.variableName);

/** The JSON body parser with its highlighting and folding. */
const jsonBodyParser = jsonParser.configure({
  props: [
      styleTags({
        PropertyName: t.propertyName,
        String: t.string,
        Number: t.number,
        "True False": t.bool,
        Null: t.null,
        Escape: t.escape,
        "Template/... StringTemplate/...": template,
        "{ }": t.brace,
        "[ ]": t.squareBracket,
        ", :": t.separator,
      }),
      foldNodeProp.add({ "Object Array": (node) => inside(node) }),
  ],
});

/** JSON bodies: plain JSON, a template for any value or inside a string. */
export const jsonBodyLanguage = LRLanguage.define({
  name: "hurl-json",
  parser: jsonBodyParser,
  languageData: { closeBrackets: { brackets: ["{", "[", '"'] } },
});

/** The folded range of a delimited node: between its first and last
 * characters (the editor offers only ranges that span lines). */
function inside(node: SyntaxNode): { from: number; to: number } | null {
  return node.to - node.from > 2 ? { from: node.from + 1, to: node.to - 1 } : null;
}

/** An entry or section folds from the end of its first line to the end of
 * its last line that is not blank or a comment (a comment before the next
 * request belongs to it). */
function afterFirstLine(node: SyntaxNode, state: EditorState): { from: number; to: number } | null {
  const first = state.doc.lineAt(node.from);
  let last = state.doc.lineAt(Math.max(node.from, node.to - 1));
  while (last.number > first.number && /^\s*(#.*)?$/.test(last.text)) last = state.doc.line(last.number - 1);
  return last.number > first.number ? { from: first.to, to: last.to } : null;
}

/** A body spanning lines folds between its first and last lines. */
function betweenLines(node: SyntaxNode, state: EditorState): { from: number; to: number } | null {
  const first = state.doc.lineAt(node.from);
  const last = state.doc.lineAt(node.to);
  return last.number > first.number + 1 ? { from: first.to, to: last.from - 1 } : null;
}

/** A ```lang body: its language and the range after the first line. */
function fenced(node: SyntaxNode, read: (from: number, to: number) => string) {
  const head = read(node.from, Math.min(node.to, node.from + 64));
  const nl = head.indexOf("\n");
  if (nl < 0 || node.to - node.from < 6) return null;
  const lang = head.slice(3, nl).trim().replace(/,$/, "");
  return { lang, from: node.from + nl + 1, to: node.to - 3 };
}

const fileLanguageParser = fileParser.configure({
  props: [
    styleTags({
      Method: t.keyword,
      UrlText: t.url,
      "UrlTemplate Template/...": template,
      "VariableName FunctionName": template,
      HttpVersion: t.keyword,
      SectionHeader: t.heading,
      Key: t.propertyName,
      QueryName: t.function(t.variableName),
      FilterName: t.function(t.propertyName),
      Predicate: t.operatorKeyword,
      Modifier: t.modifier,
      Bool: t.bool,
      Null: t.null,
      Number: t.number,
      "String/...": t.string,
      "Backtick/...": t.special(t.string),
      Escape: t.escape,
      "StringTemplate/... BacktickTemplate/...": template,
      Regex: t.regexp,
      Comment: t.lineComment,
      Operator: t.compareOperator,
      Punct: t.punctuation,
      ":": t.separator,
      "MultilineString LiteralBody": t.string,
    }),
    foldNodeProp.add({
      "Entry Section": afterFirstLine,
      "JsonBody XmlBody": betweenLines,
      MultilineString: (node, state) => afterFirstLine(node, state),
    }),
  ],
  wrap: parseMixed((node, input) => {
    switch (node.name) {
      case "JsonBody":
        return { parser: jsonBodyParser };
      case "XmlBody":
        return { parser: xmlParser };
      case "MultilineString": {
        const f = fenced(node.node, (a, b) => input.read(a, b));
        if (!f || f.to <= f.from) return null;
        const parser = f.lang === "json" ? jsonBodyParser : f.lang === "xml" ? xmlParser : f.lang === "graphql" ? graphqlLanguage.parser : null;
        return parser ? { parser, overlay: [{ from: f.from, to: f.to }] } : null;
      }
    }
    return null;
  }),
});

/** The file language (highlighting and folding only). */
export const sondeFileLanguage = LRLanguage.define({
  name: "sonde",
  parser: fileLanguageParser,
  languageData: { commentTokens: { line: "#" } },
});

/** The file language as an editor extension. */
export function sondeLanguage(): LanguageSupport {
  return new LanguageSupport(sondeFileLanguage);
}
