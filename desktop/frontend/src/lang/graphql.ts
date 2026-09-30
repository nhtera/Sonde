// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A small GraphQL highlighter for ```graphql bodies: keywords, names,
// variables, strings, numbers, comments. (A full GraphQL language package
// would add its language service, far beyond the size budget.)

import { StreamLanguage, type StringStream } from "@codemirror/language";

const keywords = new Set(["query", "mutation", "subscription", "fragment", "on", "true", "false", "null", "variables"]);

interface State {
  /** Inside a """block string""". */
  block: boolean;
}

function token(stream: StringStream, state: State): string | null {
  if (state.block) {
    if (stream.skipTo('"""')) {
      stream.match('"""');
      state.block = false;
    } else stream.skipToEnd();
    return "string";
  }
  if (stream.eatSpace()) return null;
  if (stream.match("#")) {
    stream.skipToEnd();
    return "comment";
  }
  if (stream.match('"""')) {
    state.block = true;
    return token(stream, state) ?? "string";
  }
  if (stream.match(/^"(?:[^"\\]|\\.)*"?/)) return "string";
  if (stream.match(/^\$[A-Za-z_]\w*/)) return "variableName";
  if (stream.match(/^\{\{[^}]*\}\}/)) return "variableName.special";
  if (stream.match(/^-?\d+(\.\d+)?([eE][+-]?\d+)?/)) return "number";
  if (stream.match(/^@[A-Za-z_]\w*/)) return "meta";
  const word = stream.match(/^[A-Za-z_]\w*/) as RegExpMatchArray | null;
  if (word) return keywords.has(word[0]) ? "keyword" : "propertyName";
  stream.next();
  return "punctuation";
}

export const graphqlLanguage = StreamLanguage.define<State>({
  name: "graphql",
  startState: () => ({ block: false }),
  token,
  languageData: { commentTokens: { line: "#" } },
});
