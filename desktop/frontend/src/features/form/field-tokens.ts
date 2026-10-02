// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The colors of a form field's text, as the editor colors the same text:
// {{variables}}, strings, numbers, a query's kind, a predicate.

export type TokenKind = "var" | "str" | "num" | "query" | "op" | "plain" | "muted";

export interface Token {
  kind: TokenKind;
  text: string;
}

const queries = new Set([
  "status", "version", "url", "ip", "header", "certificate", "cookie", "body", "bytes", "xpath", "jsonpath", "regex", "variable",
  "duration", "sha256", "md5", "redirects", "sondeGrpc",
]);
const predicates = new Set([
  "==", "!=", ">", ">=", "<", "<=", "startsWith", "endsWith", "contains", "matches", "exists", "isBoolean", "isCollection", "isDate",
  "isEmpty", "isFloat", "isInteger", "isIsoDate", "isNumber", "isString", "isIpv4", "isIpv6", "isUuid", "isList", "isObject", "not",
]);

// The last alternative takes any character the others leave (a "{{"
// closed by one brace only): the tokens always give the text back.
const part = /(\{\{[^}]*\}\}|\{\{[^}]*$)|("(?:[^"\\]|\\.)*"?)|(-?\d+(?:\.\d+)?\b)|((?:(?!\{\{)[^\s"])+)|(\s+)|([\s\S])/g;

/** The tokens of text; joined, they are text. */
export function tokens(text: string): Token[] {
  const out: Token[] = [];
  const push = (kind: TokenKind, t: string) => {
    const last = out.at(-1);
    if (last && last.kind === kind && kind === "plain") last.text += t;
    else out.push({ kind, text: t });
  };
  for (const m of text.matchAll(part)) {
    const [t, v, s, n, word] = m;
    if (v) push("var", t);
    else if (s) {
      // A string's {{variables}} keep their color.
      for (const sm of t.matchAll(/(\{\{[^}"]*(?:\}\})?)|([^{]+|\{)/g)) push(sm[1] ? "var" : "str", sm[0]);
    } else if (n) push("num", t);
    else if (word && (word === "true" || word === "false" || word === "null")) push("num", t);
    else if (word && queries.has(word)) push("query", t);
    else if (word && predicates.has(word)) push("op", t);
    else push("plain", t);
  }
  return out;
}
