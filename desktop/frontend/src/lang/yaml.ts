// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A small YAML highlighter for sonde.yaml and the project's other YAML
// files: keys, scalars (strings, numbers, booleans, null), list dashes,
// anchors, tags and comments. Highlighting only: sonde.yaml is checked by
// Go when it is loaded.

import { StreamLanguage, type StringStream } from "@codemirror/language";

interface State {
  /** Past the key's ":" (or a list item's dash): the rest is a value. */
  value: boolean;
}

const bools = /^(true|false|yes|no|on|off)$/i;
const nulls = /^(null|~)$/i;
const numbers = /^[-+]?(\d[\d_]*(\.\d*)?([eE][-+]?\d+)?|\.\d+|0x[\da-fA-F]+|0o[0-7]+|\.inf|\.nan)$/i;

/** A plain scalar: a boolean, null, number or string. */
function scalar(word: string): string {
  if (bools.test(word)) return "bool";
  if (nulls.test(word)) return "null";
  if (numbers.test(word)) return "number";
  return "string";
}

export function yamlToken(stream: StringStream, state: State): string | null {
  if (stream.sol()) state.value = false;
  if (stream.eatSpace()) return null;
  // A comment starts a line or follows a space.
  if (stream.peek() === "#") {
    stream.skipToEnd();
    return "lineComment";
  }
  if (stream.sol() && stream.match(/^(---|\.\.\.)(?=\s|$)/)) return "modifier";
  if (stream.match(/^-(?=\s|$)/)) return "modifier";
  if (stream.match(/^[&*][^\s,[\]{}]+/) || stream.match(/^![^\s]*/)) return "modifier";
  if (!state.value) {
    // key: (a plain or quoted key, then ":" and a space or the line's end)
    if (stream.match(/^(?:"(?:[^"\\]|\\.)*"|'(?:[^']|'')*'|[^\s#:"'][^#:]*?)(?=\s*:(?:\s|$))/)) return "propertyName";
    if (stream.match(/^:(?=\s|$)/)) {
      state.value = true;
      return null;
    }
    state.value = true;
  }
  if (stream.match(/^"(?:[^"\\]|\\.)*"?/) || stream.match(/^'(?:[^']|'')*'?/)) return "string";
  if (stream.match(/^[|>][-+]?\d*(?=\s|$)/)) return "modifier";
  if (stream.match(/^[[\]{},]/)) return null;
  // A plain scalar runs to a " #" comment, a flow indicator or the end.
  const word = stream.match(/^[^\s,[\]{}#][^,[\]{}]*?(?=\s+#|\s*$|\s*[,\]}])/) as RegExpMatchArray | null;
  if (word) return scalar(word[0].trim());
  stream.next();
  return null;
}

export const yamlLanguage = StreamLanguage.define<State>({
  name: "yaml",
  startState: () => ({ value: false }),
  token: yamlToken,
  languageData: { commentTokens: { line: "#" } },
});
