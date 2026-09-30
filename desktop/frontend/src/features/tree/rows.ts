// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The tree as a flat list of visible rows: folders, files and each open
// file's requests; with a filter, only the matching requests (by method,
// path or title here, by any line through Workspace.Filter) and their
// files and folders.

import type { Node, Request } from "../../lib/api";

export type Row =
  | { kind: "dir"; path: string; name: string; depth: number; open: boolean }
  | { kind: "file"; path: string; name: string; depth: number; fileKind: string; open: boolean; hl?: [number, number] }
  | { kind: "req"; path: string; req: Request; depth: number; hl?: [number, number] };

export interface RowOptions {
  /** Folders the user closed (all are open by default). */
  closed: ReadonlySet<string>;
  /** Files whose requests are shown. */
  expanded: ReadonlySet<string>;
  filter: string;
  /** Requests whose text (headers, body…) holds the filter, by file:
   * Workspace.Filter's answer for it. */
  matches?: ReadonlyMap<string, ReadonlySet<number>>;
}

/** Requests grouped by file. */
export function byFile(index: Request[]): Map<string, Request[]> {
  const m = new Map<string, Request[]>();
  for (const r of index) {
    const list = m.get(r.file);
    if (list) list.push(r);
    else m.set(r.file, [r]);
  }
  return m;
}

/** Where q matches s, case-insensitively. */
function match(s: string, q: string): [number, number] | undefined {
  const i = s.toLowerCase().indexOf(q);
  return i < 0 ? undefined : [i, i + q.length];
}

export function reqLabel(r: Request): string {
  return r.url.replace(/^\{\{[^}]+\}\}/, "").replace(/^https?:\/\/[^/]+/, "") || r.url;
}

/** Flattens tree into visible rows. */
export function rows(tree: Node | null, requests: Map<string, Request[]>, o: RowOptions): Row[] {
  const out: Row[] = [];
  if (!tree) return out;
  const q = o.filter.trim().toLowerCase();
  const walk = (n: Node, depth: number): boolean => {
    let any = false;
    for (const c of n.children ?? []) {
      if (!c) continue;
      if (c.kind === "dir") {
        const at = out.length;
        const open = q !== "" || !o.closed.has(c.path);
        out.push({ kind: "dir", path: c.path, name: c.name, depth, open });
        const inner = open ? walk(c, depth + 1) : false;
        if (q && !inner) out.length = at; // no match inside
        else any = true;
        continue;
      }
      const reqs = requests.get(c.path) ?? [];
      if (!q) {
        const open = o.expanded.has(c.path);
        out.push({ kind: "file", path: c.path, name: c.name, depth, fileKind: c.kind, open });
        if (open) for (const r of reqs) out.push({ kind: "req", path: c.path, req: r, depth: depth + 1 });
        any = true;
        continue;
      }
      const nameHit = match(c.name, q);
      const hits = reqs
        .map((r) => ({ r, hl: match(`${r.method} ${reqLabel(r)}`, q) ?? match(r.title ?? "", q) }))
        .filter((h) => h.hl || o.matches?.get(c.path)?.has(h.r.entry));
      if (!nameHit && hits.length === 0) continue;
      out.push({ kind: "file", path: c.path, name: c.name, depth, fileKind: c.kind, open: true, hl: nameHit });
      for (const h of hits) out.push({ kind: "req", path: c.path, req: h.r, depth: depth + 1, hl: h.hl });
      any = true;
    }
    return any;
  };
  walk(tree, 0);
  return out;
}
