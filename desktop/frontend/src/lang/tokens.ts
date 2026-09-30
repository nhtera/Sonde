// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The tokens of sonde.grammar that depend on where a line starts or on
// text ahead: line starts (a method, HTTP, a section header, a key), the
// URL's pieces, whole bodies (JSON, XML, ```…```, base64,…;), regexes,
// strings and template openings. Each is emitted only when the parser can
// take it (stack.canShift), so an unexpected one never blocks the plain
// tokens.

import { ExternalTokenizer, type InputStream } from "@lezer/lr";
import {
  backtickContent,
  backtickEnd,
  backtickStart,
  eofEnd,
  Escape,
  HttpVersion,
  JsonBody,
  Key,
  LiteralBody,
  Method,
  MultilineString,
  Regex,
  SectionHeader,
  stringContent,
  stringEnd,
  stringStart,
  templateOpen,
  UrlTemplate,
  UrlText,
  XmlBody,
} from "./sonde.grammar.terms";

type Stack = { canShift(term: number): boolean };

const NL = 10, SPACE = 32, TAB = 9, CR = 13;
const QUOTE = 34, APOSTROPHE = 39, HASH = 35, COLON = 58, SLASH = 47, BACKSLASH = 92, BACKTICK = 96;
const LBRACE = 123, RBRACE = 125, LBRACKET = 91, RBRACKET = 93, LT = 60, GT = 62;

/** How far back a line is read (for the regex and message contexts). */
const MAX_LOOKBEHIND = 256;

const isSpace = (c: number) => c === SPACE || c === TAB || c === CR;
const isUpper = (c: number) => c >= 65 && c <= 90;
const isLetter = (c: number) => isUpper(c) || (c >= 97 && c <= 122);

/** The end of the current line's text before the stream's position (at
 * most MAX_LOOKBEHIND characters). */
function lineBefore(input: InputStream): string {
  let s = "";
  for (let k = -1; k >= -MAX_LOOKBEHIND; k--) {
    const c = input.peek(k);
    if (c === NL || c === -1) break;
    s = String.fromCharCode(c) + s;
  }
  return s;
}

/** Whether only spaces precede offset i on its line. */
function lineStartsAt(input: InputStream, i: number): boolean {
  for (let k = i - 1; ; k--) {
    const c = input.peek(k);
    if (c === NL || c === -1) return true;
    if (!isSpace(c)) return false;
  }
}

/** Whether input at offset i starts with s. */
function startsWith(input: InputStream, i: number, s: string): boolean {
  for (let j = 0; j < s.length; j++) if (input.peek(i + j) !== s.charCodeAt(j)) return false;
  return true;
}

/** The offset after s found from offset i, or -1 (up to the end, or the
 * line's end when sameLine). */
function after(input: InputStream, i: number, s: string, sameLine = false): number {
  for (;;) {
    const c = input.peek(i);
    if (c === -1 || (sameLine && c === NL)) return -1;
    if (startsWith(input, i, s)) return i + s.length;
    i++;
  }
}

/** Whether the line starting at offset i (after its spaces) begins the
 * next request or response: a body never holds such a line. */
function entryLineAt(input: InputStream, i: number): boolean {
  while (isSpace(input.peek(i))) i++;
  if (startsWith(input, i, "HTTP")) {
    const n = input.peek(i + 4);
    if (n === SLASH || isSpace(n)) return true;
  }
  let j = i;
  while (isUpper(input.peek(j))) j++;
  if (j === i || !isSpace(input.peek(j))) return false;
  while (isSpace(input.peek(j))) j++;
  const n = input.peek(j);
  return n !== -1 && n !== NL && n !== HASH && n !== COLON;
}

/** The offset after the string (or backtick string) opening at i, on the
 * same line, or -1. */
function skipString(input: InputStream, i: number, quote = QUOTE): number {
  for (i++; ; i++) {
    const c = input.peek(i);
    if (c === -1 || c === NL) return -1;
    if (c === BACKSLASH) {
      if (input.peek(i + 1) === NL || input.peek(i + 1) === -1) return -1;
      i++;
    } else if (c === quote) return i + 1;
  }
}

/** The offset after the JSON object or array opening at i, or -1 when it
 * does not close before the next request or response. Templates count as
 * values; strings stay on their line. */
function skipJson(input: InputStream, i: number): number {
  let depth = 0;
  for (;;) {
    const c = input.peek(i);
    if (c === -1) return -1;
    if (c === NL && entryLineAt(input, i + 1)) return -1;
    if (c === QUOTE) {
      i = skipString(input, i);
      if (i < 0) return -1;
      continue;
    }
    if (c === LBRACE && input.peek(i + 1) === LBRACE) {
      i = after(input, i + 2, "}}", true);
      if (i < 0) return -1;
      continue;
    }
    if (c === LBRACE || c === LBRACKET) depth++;
    else if (c === RBRACE || c === RBRACKET) {
      depth--;
      if (depth === 0) return i + 1;
    }
    i++;
  }
}

/** The offset after the XML document starting at i (its root element),
 * or -1 when it does not close before the next request or response. */
function skipXml(input: InputStream, i: number): number {
  let depth = 0;
  for (;;) {
    const c = input.peek(i);
    if (c === -1) return -1;
    if (c !== LT) {
      if (c === NL && entryLineAt(input, i + 1)) return -1;
      i++;
      continue;
    }
    if (startsWith(input, i, "<?")) i = after(input, i + 2, "?>");
    else if (startsWith(input, i, "<!--")) i = after(input, i + 4, "-->");
    else if (startsWith(input, i, "<![CDATA[")) i = after(input, i + 9, "]]>");
    else if (startsWith(input, i, "<!")) i = after(input, i + 2, ">");
    else {
      const closing = input.peek(i + 1) === SLASH;
      // The tag's end, past quoted attribute values.
      let j = i + 1;
      for (;;) {
        const t = input.peek(j);
        if (t === -1) return -1;
        if (t === QUOTE || t === APOSTROPHE) {
          j = after(input, j + 1, String.fromCharCode(t));
          if (j < 0) return -1;
          continue;
        }
        if (t === GT) break;
        j++;
      }
      const selfClosing = input.peek(j - 1) === SLASH;
      i = j + 1;
      if (closing) depth--;
      else if (!selfClosing) depth++;
      if (depth <= 0) return i;
      continue;
    }
    if (i < 0) return -1;
  }
}

/** The end of a `base64,…;`, `hex,…;` or `file,…;` at i, or -1. */
function skipLiteral(input: InputStream, i: number): number {
  for (const kind of ["base64,", "hex,", "file,"]) {
    if (startsWith(input, i, kind)) return after(input, i + kind.length, ";", true);
  }
  return -1;
}

/** The end of a key at i (a key is followed by `:`), or -1. */
function keyEnd(input: InputStream, i: number): number {
  const start = i;
  for (;;) {
    const c = input.peek(i);
    if (c === LBRACE && input.peek(i + 1) === LBRACE) {
      i = after(input, i + 2, "}}", true);
      if (i < 0) return -1;
    } else if (c === BACKSLASH && input.peek(i + 1) !== NL && input.peek(i + 1) !== -1) {
      i += 2;
    } else if (c === -1 || c === NL || c === COLON || c === HASH || c === QUOTE || c === BACKSLASH || isSpace(c)) {
      break;
    } else {
      i++;
    }
  }
  if (i === start) return -1;
  let j = i;
  while (isSpace(input.peek(j))) j++;
  return input.peek(j) === COLON ? i : -1;
}

/** Whether the rest of the line from i is blank or a comment. */
function restBlank(input: InputStream, i: number): boolean {
  for (;;) {
    const c = input.peek(i);
    if (c === -1 || c === NL || c === HASH) return true;
    if (!isSpace(c)) return false;
    i++;
  }
}

/** Accepts the token when the parser can take it. */
function take(input: InputStream, stack: Stack, term: number, end: number): boolean {
  if (end <= 0 || !stack.canShift(term)) return false;
  input.acceptToken(term, end);
  return true;
}

// Line starts: a method then a URL, HTTP, a section header, a key, a body.
function lineStart(input: InputStream, stack: Stack): boolean {
  const c = input.next;
  if (isUpper(c)) {
    let i = 0;
    while (isLetter(input.peek(i))) i++;
    const word = i;
    let upper = true;
    for (let k = 0; k < word; k++) if (!isUpper(input.peek(k))) upper = false;
    if (word === 4 && startsWith(input, 0, "HTTP")) {
      let v = 4;
      if (input.peek(v) === SLASH) {
        v++;
        while ((input.peek(v) >= 48 && input.peek(v) <= 57) || input.peek(v) === 46) v++;
      }
      const n = input.peek(v);
      if ((v > 4 || isSpace(n) || n === NL || n === -1) && take(input, stack, HttpVersion, v)) return true;
    }
    if (upper && entryLineAt(input, 0) && take(input, stack, Method, word)) return true;
  }
  // A section header: [Name], Name capitalized ([true] is a JSON body).
  if (c === LBRACKET && isUpper(input.peek(1))) {
    let i = 1;
    while (isLetter(input.peek(i))) i++;
    if (input.peek(i) === RBRACKET && restBlank(input, i + 1) && take(input, stack, SectionHeader, i + 1)) return true;
  }
  if (startsWith(input, 0, "```") && take(input, stack, MultilineString, after(input, 3, "```"))) return true;
  if ((c === LBRACE && input.peek(1) !== LBRACE) || c === LBRACKET) {
    if (take(input, stack, JsonBody, skipJson(input, 0))) return true;
  }
  if (c === QUOTE && take(input, stack, JsonBody, skipString(input, 0))) return true;
  if (c === LT && take(input, stack, XmlBody, skipXml(input, 0))) return true;
  if (take(input, stack, LiteralBody, skipLiteral(input, 0))) return true;
  if (c !== LBRACKET && c !== LT && c !== BACKTICK && take(input, stack, Key, keyEnd(input, 0))) return true;
  return false;
}

/** Inside a string or backtick string: its content, escapes, templates and
 * closing mark. */
function stringPart(input: InputStream, stack: Stack, quote: number, content: number, end: number): void {
  const c = input.next;
  if (c === quote) {
    take(input, stack, end, 1);
    return;
  }
  if (c === BACKSLASH) {
    take(input, stack, Escape, input.peek(1) === NL || input.peek(1) === -1 ? 1 : 2);
    return;
  }
  if (c === LBRACE && input.peek(1) === LBRACE && after(input, 2, "}}", true) > 0 && take(input, stack, templateOpen, 2)) return;
  let i = 0;
  for (;;) {
    const s = input.peek(i);
    if (s === -1 || s === NL || s === quote || s === BACKSLASH) break;
    if (i > 0 && s === LBRACE && input.peek(i + 1) === LBRACE && after(input, i + 2, "}}", true) > 0) break;
    i++;
  }
  take(input, stack, content, i);
}

// The words after which `/…/` is a regex.
const regexBefore = /(?:^|\s)(?:matches|regex|replaceRegex|replace|split)\s*$/;

export const lineTokens = new ExternalTokenizer(
  (input, stack) => {
    const c = input.next;
    if (c === -1) {
      // An empty token ends the last line when it has no newline.
      if (stack.canShift(eofEnd)) input.acceptToken(eofEnd, 0);
      return;
    }
    // Strings keep their spaces.
    if (stack.canShift(stringEnd)) return stringPart(input, stack, QUOTE, stringContent, stringEnd);
    if (stack.canShift(backtickEnd)) return stringPart(input, stack, BACKTICK, backtickContent, backtickEnd);
    if (c === NL || isSpace(c)) return;
    if (lineStartsAt(input, 0) && lineStart(input, stack)) return;

    // The URL: its first piece follows the method, the next ones follow
    // the previous piece with no space between.
    if (stack.canShift(UrlText) || stack.canShift(UrlTemplate)) {
      const prev = input.peek(-1);
      const first = isSpace(prev) && /^\s*[A-Z]+[ \t]+$/.test(lineBefore(input));
      if (first || (!isSpace(prev) && prev !== NL && prev !== -1)) {
        if (c === LBRACE && input.peek(1) === LBRACE) {
          let end = after(input, 2, "}}", true);
          if (end < 0) end = 2;
          if (take(input, stack, UrlTemplate, end)) return;
        } else {
          let i = 0;
          for (;;) {
            const u = input.peek(i);
            if (u === -1 || u === NL || isSpace(u) || (u === LBRACE && input.peek(i + 1) === LBRACE)) break;
            i++;
          }
          if (take(input, stack, UrlText, i)) return;
        }
      }
    }

    if (c === LBRACE && input.peek(1) === LBRACE) {
      if (after(input, 2, "}}", true) > 0) take(input, stack, templateOpen, 2);
      return;
    }
    if (c === BACKTICK) {
      if (startsWith(input, 0, "```")) {
        take(input, stack, MultilineString, after(input, 3, "```"));
        return;
      }
      take(input, stack, backtickStart, skipString(input, 0, BACKTICK) > 0 ? 1 : -1);
      return;
    }
    if (c === QUOTE) {
      take(input, stack, stringStart, skipString(input, 0) > 0 ? 1 : -1);
      return;
    }
    // A value that is a body: `file,…;` and friends after a space or `:`.
    if (c === 98 || c === 104 || c === 102) {
      const prev = input.peek(-1);
      if ((isSpace(prev) || prev === COLON) && take(input, stack, LiteralBody, skipLiteral(input, 0))) return;
      return;
    }
    // A message step's JSON (`send: {…}` may span lines).
    if (c === LBRACE || c === LBRACKET) {
      if (/^\s*send\s*:\s*$/.test(lineBefore(input))) take(input, stack, JsonBody, skipJson(input, 0));
      return;
    }
    if (c === SLASH && regexBefore.test(lineBefore(input))) {
      let i = 1;
      for (;;) {
        const r = input.peek(i);
        if (r === -1 || r === NL || (r === BACKSLASH && (input.peek(i + 1) === NL || input.peek(i + 1) === -1))) {
          i = -1;
          break;
        }
        if (r === BACKSLASH) i += 2;
        else if (r === SLASH) {
          i++;
          break;
        } else i++;
      }
      take(input, stack, Regex, i);
    }
  },
  { contextual: true },
);
