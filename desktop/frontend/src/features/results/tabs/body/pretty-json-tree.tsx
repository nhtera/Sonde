// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A JSON body as a foldable tree. The worker builds it from the body's
// bytes (every number as written); the page draws the visible rows only,
// decoding each from the bytes. Hovering a value offers + Assert and
// + Capture, which Go writes into the file.

import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, useState } from "react";
import { isClose, isOpen, keyText, Kind, nodeAt, pathOf, valueText, visibleRows, type Folds, type JsonTree } from "../../../../workers/json-tree";
import type { TreeReply } from "../../../../workers/json-tree.worker";
import { bodyURL } from "./body-fetch";
import { byteMatches } from "./body-search";

const ROW = 22;

let worker: Worker | null = null;
let nextId = 0;
const waiting = new Map<number, (r: TreeReply) => void>();
/** The last trees built, by body id (they hold the body's bytes). */
const trees = new Map<string, Promise<TreeReply>>();

/** Builds the tree of body id in the worker. */
export function loadTree(id: string): Promise<TreeReply> {
  const had = trees.get(id);
  if (had) return had;
  if (!worker) {
    worker = new Worker(new URL("../../../../workers/json-tree.worker.ts", import.meta.url), { type: "module" });
    worker.onmessage = (e: MessageEvent<TreeReply>) => {
      waiting.get(e.data.id)?.(e.data);
      waiting.delete(e.data.id);
    };
    // A worker that fails to load answers every waiting body with the
    // error (the body then shows as text), and none of them stays cached.
    worker.onerror = (e) => {
      e.preventDefault();
      for (const [reqId, resolve] of waiting) resolve({ id: reqId, ok: false, error: "the formatter could not start" });
      waiting.clear();
      trees.clear();
      worker?.terminate();
      worker = null;
    };
  }
  const reqId = ++nextId;
  const p = new Promise<TreeReply>((resolve) => waiting.set(reqId, resolve));
  worker.postMessage({ id: reqId, url: bodyURL(id) });
  trees.set(id, p);
  while (trees.size > 2) trees.delete(trees.keys().next().value!);
  return p;
}

export interface TreeProps {
  bodyId: string;
  query: string;
  /** Bumped to move to the next (step > 0) or previous match. */
  nav: { n: number; step: number };
  onMatches(current: number, total: number): void;
  /** Not JSON after all: the caller shows the raw text. */
  onInvalid(error: string): void;
  onAssert(path: string): void;
  onCapture(path: string): void;
}

export function PrettyJsonTree(p: TreeProps) {
  const [loaded, setLoaded] = useState<{ id: string; reply: TreeReply } | null>(null);
  useEffect(() => {
    let live = true;
    void loadTree(p.bodyId).then((reply) => live && setLoaded({ id: p.bodyId, reply }));
    return () => {
      live = false;
    };
  }, [p.bodyId]);
  const reply = loaded?.id === p.bodyId ? loaded.reply : null;
  const { onInvalid } = p;
  useEffect(() => {
    if (reply && !reply.ok) onInvalid(reply.error);
  }, [reply, onInvalid]);
  if (!reply) return <div className="body-note">Formatting…</div>;
  if (!reply.ok) return null;
  return <TreeRows {...p} model={reply} />;
}

interface TreeRowsProps extends TreeProps {
  /** The cached tree: its folds and rows change in place, so a tree shown
   * again keeps them together. */
  model: { tree: JsonTree; folds: Folds; rows: Int32Array };
}

function TreeRows({ model, query, nav, onMatches, onAssert, onCapture }: TreeRowsProps) {
  const { tree, folds } = model;
  // Folds change in place; the version redraws.
  const [version, setVersion] = useState(0);
  const refold = () => {
    model.rows = visibleRows(tree, folds);
    setVersion((v) => v + 1);
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const rows = useMemo(() => model.rows, [model, version]);
  const scroller = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtual = useVirtualizer({ count: rows.length, getScrollElement: () => scroller.current, estimateSize: () => ROW, overscan: 30 });

  // Search: the nodes whose row holds a match, then the current one.
  const [found, setFound] = useState<{ query: string; nodes: number[] }>({ query: "", nodes: [] });
  const [current, setCurrent] = useState(-1);
  const handled = useRef(nav.n);
  useEffect(() => {
    const ctl = new AbortController();
    void byteMatches(tree.bytes, query, ctl.signal).then((offsets) => {
      if (ctl.signal.aborted) return;
      const nodes: number[] = [];
      for (const at of offsets) {
        const n = nodeAt(tree, at);
        if (nodes.at(-1) !== n) nodes.push(n);
      }
      setFound({ query, nodes });
      setCurrent(nodes.length ? 0 : -1);
    });
    return () => ctl.abort();
  }, [tree, query]);
  const matches = found.query === query ? found.nodes : [];
  useEffect(() => {
    // Each Enter moves once: not again when new matches arrive.
    if (nav.n === handled.current || matches.length === 0) return;
    handled.current = nav.n;
    setCurrent((c) => (c + nav.step + matches.length) % matches.length);
  }, [nav, matches.length]);
  const shown = current < matches.length ? current : -1;
  useEffect(() => onMatches(shown, matches.length), [shown, matches.length, onMatches]);
  // Show the current match: unfold its containers, then scroll to it.
  const target = shown >= 0 ? matches[shown] : -1;
  useEffect(() => {
    if (target < 0) return;
    let hidden = false;
    for (let n = tree.parent[target]; n >= 0; n = tree.parent[n]) {
      if (folds[n]) {
        folds[n] = 0;
        hidden = true;
      }
    }
    if (hidden) {
      model.rows = visibleRows(tree, folds);
      setVersion((v) => v + 1);
      return;
    }
    const row = rows.indexOf(target);
    if (row >= 0) virtual.scrollToIndex(row, { align: "center" });
  }, [target, rows, folds, tree, virtual, model]);

  const toggle = (node: number) => {
    folds[node] ^= 1;
    refold();
  };

  return (
    <div className="json-tree mono" ref={scroller} role="tree" aria-label="Response body">
      <div style={{ height: virtual.getTotalSize(), position: "relative" }}>
        {virtual.getVirtualItems().map((v) => {
          const node = rows[v.index];
          const kind = tree.kind[node];
          const open = isOpen(kind);
          const folded = open && folds[node] === 1;
          const key = keyText(tree, node);
          const value = valueText(tree, node);
          const count = open ? tree.aux[tree.aux[node]] : 0;
          const unit = kind === Kind.Array ? (count === 1 ? "item" : "items") : count === 1 ? "key" : "keys";
          return (
            <div
              key={v.key}
              className="jt-row"
              role="treeitem"
              aria-level={tree.depth[node] + 1}
              aria-expanded={open ? !folded : undefined}
              data-match={node === target || undefined}
              tabIndex={0}
              style={{ transform: `translateY(${v.start}px)`, paddingLeft: 8 + tree.depth[node] * 14 }}
            >
              {open ? (
                <button className="jt-caret" aria-label={folded ? "Unfold" : "Fold"} onClick={() => toggle(node)}>
                  {folded ? "▸" : "▾"}
                </button>
              ) : (
                <span className="jt-caret" />
              )}
              {key && (
                <>
                  <span className="jt-key">{key}</span>
                  <span className="jt-colon">: </span>
                </>
              )}
              <span className={`jt-v jt-k${kind}`}>
                {value.text}
                {value.cut && "…"}
              </span>
              {folded && <span className="jt-fold"> … {kind === Kind.Array ? "]" : "}"}</span>}
              {open && <span className="jt-count">{`${count} ${unit}`}</span>}
              {!isClose(kind) && (
                <span className="jt-actions">
                  <button className="jt-assert" onClick={() => onAssert(pathOf(tree, node))}>
                    + Assert
                  </button>
                  <button className="jt-capture" onClick={() => onCapture(pathOf(tree, node))}>
                    + Capture
                  </button>
                </span>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
