// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// One suggestion's change, as the lines it removes and adds: a line diff
// of the file before and after it (each changed region, a gap between
// two regions), unchanged lines left out.

export interface HunkLine {
  kind: "add" | "del" | "gap";
  text: string;
}

export interface Hunk {
  /** The first changed line (1-based), in after. */
  line: number;
  lines: HunkLine[];
}

export function hunk(before: string, after: string): Hunk {
  const a = before.split("\n");
  const b = after.split("\n");
  // The longest common subsequence of lines (files are small).
  const n = a.length;
  const m = b.length;
  const lcs: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
  }
  const lines: HunkLine[] = [];
  let first = 0;
  let same = false;
  let i = 0;
  let j = 0;
  const push = (l: HunkLine) => {
    if (same && lines.length > 0) lines.push({ kind: "gap", text: "…" });
    if (!first) first = j + 1;
    same = false;
    lines.push(l);
  };
  while (i < n || j < m) {
    if (i < n && j < m && a[i] === b[j]) {
      same = true;
      i++;
      j++;
    } else if (j < m && (i >= n || lcs[i][j + 1] > lcs[i + 1][j])) {
      push({ kind: "add", text: b[j] });
      j++;
    } else {
      push({ kind: "del", text: a[i] });
      i++;
    }
  }
  return { line: first || m + 1, lines };
}
