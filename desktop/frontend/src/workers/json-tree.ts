// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A JSON body as a flat list of nodes, one per row of the tree view, built
// from the body's bytes without JSON.parse: every value keeps its text, so
// a 64-bit id shows (and is asserted) with every digit. Nodes live in typed
// arrays (about 19 bytes each) that the worker hands to the page without a
// copy; the page decodes the few rows it shows.

export const Kind = {
  Object: 1,
  Array: 2,
  /** The `}` of an object (parent: its opening node). */
  CloseObject: 3,
  /** The `]` of an array (parent: its opening node). */
  CloseArray: 4,
  String: 5,
  Number: 6,
  True: 7,
  False: 8,
  Null: 9,
} as const;
export type Kind = (typeof Kind)[keyof typeof Kind];

/** No key: the root, or a closing bracket. */
export const NO_KEY = -0x80000000;

export interface JsonTree {
  bytes: Uint8Array;
  count: number;
  kind: Uint8Array;
  depth: Uint16Array;
  /** The containing node's index (-1 for the root). */
  parent: Int32Array;
  /** The byte offset of the key's opening quote; for an array element
   * -(index + 1); NO_KEY for none. */
  key: Int32Array;
  /** The byte offset of the value's first byte (bracket, quote, digit…). */
  val: Int32Array;
  /** An opening node: the index of its closing node; a closing node: its
   * number of children. */
  aux: Int32Array;
}

export class JsonError extends Error {
  constructor(
    message: string,
    readonly offset: number,
  ) {
    super(`${message} at byte ${offset}`);
  }
}

/** Nesting deeper than this is refused (the depth array is 16-bit). */
const MAX_DEPTH = 10_000;

const QUOTE = 0x22;
const BACKSLASH = 0x5c;

const isSpace = (c: number) => c === 0x20 || c === 0x0a || c === 0x0d || c === 0x09;

/** Builds the tree of a JSON document; throws JsonError when it is not
 * JSON. */
export function buildTree(bytes: Uint8Array): JsonTree {
  let cap = Math.max(64, Math.ceil(bytes.length / 8));
  let kind = new Uint8Array(cap);
  let depth = new Uint16Array(cap);
  let parent = new Int32Array(cap);
  let key = new Int32Array(cap);
  let val = new Int32Array(cap);
  let aux = new Int32Array(cap);
  let n = 0;
  const grow = () => {
    cap *= 2;
    const k = new Uint8Array(cap);
    k.set(kind);
    kind = k;
    const d = new Uint16Array(cap);
    d.set(depth);
    depth = d;
    const p = new Int32Array(cap);
    p.set(parent);
    parent = p;
    const ky = new Int32Array(cap);
    ky.set(key);
    key = ky;
    const v = new Int32Array(cap);
    v.set(val);
    val = v;
    const a = new Int32Array(cap);
    a.set(aux);
    aux = a;
  };
  const push = (k: Kind, keyAt: number, valAt: number, par: number, dep: number) => {
    if (n === cap) grow();
    kind[n] = k;
    depth[n] = dep;
    parent[n] = par;
    key[n] = keyAt;
    val[n] = valAt;
    aux[n] = 0;
    return n++;
  };

  const len = bytes.length;
  let i = 0;
  const skip = () => {
    while (i < len && isSpace(bytes[i])) i++;
  };
  /** Skips a string whose opening quote is at i. */
  const skipString = () => {
    const start = i;
    i++;
    for (;;) {
      if (i >= len) throw new JsonError("unterminated string", start);
      const c = bytes[i];
      if (c === QUOTE) break;
      if (c === BACKSLASH) i++;
      else if (c < 0x20) throw new JsonError("control character in string", i);
      i++;
    }
    i++;
  };
  const literal = (text: string) => {
    for (let j = 0; j < text.length; j++) {
      if (bytes[i + j] !== text.charCodeAt(j)) throw new JsonError("unexpected value", i);
    }
    i += text.length;
  };
  const skipNumber = () => {
    const start = i;
    if (bytes[i] === 0x2d) i++;
    const digits = () => {
      const from = i;
      while (i < len && bytes[i] >= 0x30 && bytes[i] <= 0x39) i++;
      if (i === from) throw new JsonError("bad number", start);
    };
    // No leading zero: 0, 0.5, but not 01.
    if (bytes[i] === 0x30 && bytes[i + 1] >= 0x30 && bytes[i + 1] <= 0x39) throw new JsonError("leading zero", start);
    digits();
    if (bytes[i] === 0x2e) {
      i++;
      digits();
    }
    if (bytes[i] === 0x65 || bytes[i] === 0x45) {
      i++;
      if (bytes[i] === 0x2b || bytes[i] === 0x2d) i++;
      digits();
    }
  };

  // The open containers: their node index and number of children so far.
  const stack: number[] = [];
  const children: number[] = [];
  skip();
  if (bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf) {
    i = 3; // a byte order mark
    skip();
  }
  let keyAt = NO_KEY;
  for (;;) {
    // A value, at i.
    if (i >= len) throw new JsonError("unexpected end", i);
    const top = stack.length - 1;
    const par = top >= 0 ? stack[top] : -1;
    const dep = stack.length;
    if (top >= 0 && kind[par] === Kind.Array) keyAt = -(children[top] + 1);
    const c = bytes[i];
    let opened = false;
    if (c === 0x7b || c === 0x5b) {
      if (stack.length >= MAX_DEPTH) throw new JsonError("nesting too deep", i);
      const node = push(c === 0x7b ? Kind.Object : Kind.Array, keyAt, i, par, dep);
      if (top >= 0) children[top]++;
      stack.push(node);
      children.push(0);
      i++;
      opened = true;
    } else {
      const at = i;
      let k: Kind;
      if (c === QUOTE) {
        skipString();
        k = Kind.String;
      } else if (c === 0x2d || (c >= 0x30 && c <= 0x39)) {
        skipNumber();
        k = Kind.Number;
      } else if (c === 0x74) {
        literal("true");
        k = Kind.True;
      } else if (c === 0x66) {
        literal("false");
        k = Kind.False;
      } else if (c === 0x6e) {
        literal("null");
        k = Kind.Null;
      } else {
        throw new JsonError("unexpected character", i);
      }
      push(k, keyAt, at, par, dep);
      if (top >= 0) children[top]++;
    }
    keyAt = NO_KEY;
    // After a value (or an opening bracket): close containers, find the
    // next key or value.
    for (;;) {
      skip();
      if (stack.length === 0) {
        if (i < len) throw new JsonError("text after the value", i);
        return { bytes, count: n, kind, depth, parent, key, val, aux };
      }
      const t = stack.length - 1;
      const open = stack[t];
      const isObject = kind[open] === Kind.Object;
      const close = isObject ? 0x7d : 0x5d;
      if (bytes[i] === close) {
        // After the last value, or right after the opening bracket.
        const node = push(isObject ? Kind.CloseObject : Kind.CloseArray, NO_KEY, i, open, t);
        aux[open] = node;
        aux[node] = children[t];
        stack.pop();
        children.pop();
        i++;
        opened = false;
        continue;
      }
      // A comma before every value but a container's first.
      if (!opened) {
        if (bytes[i] !== 0x2c) throw new JsonError(`expected , or ${String.fromCharCode(close)}`, i);
        i++;
        skip();
      }
      if (isObject) {
        if (bytes[i] !== QUOTE) throw new JsonError("expected a key", i);
        keyAt = i;
        skipString();
        skip();
        if (bytes[i] !== 0x3a) throw new JsonError("expected :", i);
        i++;
        skip();
      }
      break;
    }
  }
}

/** The byte offset just past the value of node i (a primitive or a
 * closing bracket). */
export function valueEnd(t: JsonTree, i: number): number {
  const b = t.bytes;
  let j = t.val[i];
  switch (t.kind[i]) {
    case Kind.String:
      j++;
      while (b[j] !== QUOTE) j += b[j] === BACKSLASH ? 2 : 1;
      return j + 1;
    case Kind.Number:
      while (j < b.length && !isSpace(b[j]) && b[j] !== 0x2c && b[j] !== 0x7d && b[j] !== 0x5d) j++;
      return j;
    case Kind.True:
    case Kind.Null:
      return j + 4;
    case Kind.False:
      return j + 5;
    default:
      return j + 1;
  }
}

/** The byte offset just past a key whose quote is at `at`. */
function keyEnd(b: Uint8Array, at: number): number {
  let j = at + 1;
  while (b[j] !== QUOTE) j += b[j] === BACKSLASH ? 2 : 1;
  return j + 1;
}

const decoder = new TextDecoder();

/** Node i's value as written (for a container, its bracket), cut after
 * max characters. */
export function valueText(t: JsonTree, i: number, max = 400): { text: string; cut: boolean } {
  const start = t.val[i];
  const end = valueEnd(t, i);
  // max characters are at most 4·max bytes.
  const cut = end - start > max * 4;
  let text = decoder.decode(t.bytes.subarray(start, cut ? start + max * 4 : end));
  if (text.length > max) {
    text = text.slice(0, max);
    return { text, cut: true };
  }
  return { text, cut };
}

/** Node i's key as written (with quotes), "" for none. */
export function keyText(t: JsonTree, i: number): string {
  const k = t.key[i];
  if (k < 0) return "";
  return decoder.decode(t.bytes.subarray(k, keyEnd(t.bytes, k)));
}

/** Node i's array index, or -1 when it is not an array element. */
export function indexOf(t: JsonTree, i: number): number {
  const k = t.key[i];
  return k < 0 && k !== NO_KEY ? -k - 1 : -1;
}

const shorthand = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** A member name quoted for a JSONPath ['…'] selector (RFC 9535). */
function quoteName(name: string): string {
  const named: Record<string, string> = { "\b": "b", "\f": "f", "\n": "n", "\r": "r", "\t": "t", "\\": "\\", "'": "'" };
  let out = "";
  for (const c of name) {
    const code = c.charCodeAt(0);
    if (named[c]) out += `\\${named[c]}`;
    else if (code < 0x20) out += `\\u${code.toString(16).padStart(4, "0")}`;
    else out += c;
  }
  return out;
}

/** The JSONPath of node i ("$.items[0].id"). */
export function pathOf(t: JsonTree, i: number): string {
  const parts: string[] = [];
  for (let n = i; n >= 0 && t.parent[n] >= 0; n = t.parent[n]) {
    const idx = indexOf(t, n);
    if (idx >= 0) {
      parts.push(`[${idx}]`);
      continue;
    }
    const name = JSON.parse(keyText(t, n)) as string;
    parts.push(shorthand.test(name) ? `.${name}` : `['${quoteName(name)}']`);
  }
  return "$" + parts.reverse().join("");
}

export const isOpen = (k: number) => k === Kind.Object || k === Kind.Array;
export const isClose = (k: number) => k === Kind.CloseObject || k === Kind.CloseArray;

/** Folded nodes: one byte per node, 1 when folded. */
export type Folds = Uint8Array;

/** The rows shown with folds: node indices in order (a folded node's
 * children and closing bracket are skipped). */
export function visibleRows(t: JsonTree, folds: Folds | null): Int32Array {
  const out = new Int32Array(t.count);
  let n = 0;
  for (let i = 0; i < t.count; i++) {
    out[n++] = i;
    if (folds && folds[i] && isOpen(t.kind[i])) i = t.aux[i];
  }
  return n === t.count ? out : out.slice(0, n);
}

/** The folds that leave at most max rows: the deepest levels first, never
 * the root. */
export function autoCollapse(t: JsonTree, max: number): Folds {
  const folds = new Uint8Array(t.count);
  if (t.count <= max) return folds;
  let maxDepth = 0;
  for (let i = 0; i < t.count; i++) if (t.depth[i] > maxDepth) maxDepth = t.depth[i];
  for (let d = maxDepth; d >= 1; d--) {
    // Rows left when every container at depth >= d is folded.
    let rows = 0;
    for (let i = 0; i < t.count; i++) {
      rows++;
      if (t.depth[i] >= d && isOpen(t.kind[i])) i = t.aux[i];
    }
    if (rows <= max || d === 1) {
      // Folding at depth d hides the deeper nodes: only depth d is marked.
      for (let i = 0; i < t.count; i++) if (t.depth[i] === d && isOpen(t.kind[i])) folds[i] = 1;
      return folds;
    }
  }
  return folds;
}

/** The node whose row holds byte offset `at` (the last row starting at or
 * before it). */
export function nodeAt(t: JsonTree, at: number): number {
  const start = (i: number) => (t.key[i] >= 0 ? t.key[i] : t.val[i]);
  let lo = 0;
  let hi = t.count - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (start(mid) <= at) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}
