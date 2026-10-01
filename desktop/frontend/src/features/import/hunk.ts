// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// One suggestion's change, as the lines it removes and adds: what differs
// between the file before and after, past the lines both share at their
// start and end.

export interface Hunk {
  /** The first changed line (1-based), in after. */
  line: number;
  removed: string[];
  added: string[];
}

export function hunk(before: string, after: string): Hunk {
  const b = before.split("\n");
  const a = after.split("\n");
  let start = 0;
  while (start < b.length && start < a.length && b[start] === a[start]) start++;
  let end = 0;
  while (end < b.length - start && end < a.length - start && b[b.length - 1 - end] === a[a.length - 1 - end]) end++;
  return { line: start + 1, removed: b.slice(start, b.length - end), added: a.slice(start, a.length - end) };
}
