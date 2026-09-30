// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Fetches a response body and builds its JSON tree off the page's thread;
// the tree's arrays and the body's bytes move to the page without a copy.

import { autoCollapse, buildTree, JsonError, visibleRows, type Folds } from "./json-tree";

/** Rows past this are folded, deepest levels first (a browser cannot lay
 * out millions of rows). */
export const MAX_ROWS = 200_000;

export interface TreeRequest {
  id: number;
  /** The body's URL (/_sonde/body/<id>). */
  url: string;
}

export type TreeReply =
  | { id: number; ok: true; tree: ReturnType<typeof buildTree>; folds: Folds; rows: Int32Array; ms: number }
  | { id: number; ok: false; error: string; offset?: number };

self.onmessage = async (e: MessageEvent<TreeRequest>) => {
  const { id, url } = e.data;
  try {
    const res = await fetch(url, { cache: "no-store", credentials: "same-origin" });
    if (!res.ok) throw new Error(`the body is no longer available (${res.status})`);
    const bytes = new Uint8Array(await res.arrayBuffer());
    const t0 = performance.now();
    const tree = buildTree(bytes);
    // The first rows too: the page only draws.
    const folds = autoCollapse(tree, MAX_ROWS);
    const rows = visibleRows(tree, folds);
    const ms = Math.round(performance.now() - t0);
    const reply: TreeReply = { id, ok: true, tree, folds, rows, ms };
    const buffers = [tree.bytes, tree.kind, tree.depth, tree.parent, tree.key, tree.val, tree.aux, folds, rows].map((a) => a.buffer as ArrayBuffer);
    (self as unknown as Worker).postMessage(reply, [...new Set(buffers)]);
  } catch (err) {
    const reply: TreeReply = { id, ok: false, error: err instanceof Error ? err.message : String(err), offset: err instanceof JsonError ? err.offset : undefined };
    (self as unknown as Worker).postMessage(reply);
  }
};
