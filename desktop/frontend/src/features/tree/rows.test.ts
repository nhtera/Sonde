// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { Node, Request } from "../../lib/api";
import { byFile, rows } from "./rows";

const tree: Node = {
  name: "", path: "", kind: "dir",
  children: [
    { name: "orders", path: "orders", kind: "dir", children: [{ name: "checkout.hurl", path: "orders/checkout.hurl", kind: "request" }] },
    { name: "health.hurl", path: "health.hurl", kind: "request" },
  ],
};
const index: Request[] = [
  { file: "orders/checkout.hurl", entry: 1, method: "POST", url: "{{base_url}}/auth/login", line: 1 },
  { file: "orders/checkout.hurl", entry: 2, method: "GET", url: "{{base_url}}/orders/{{id}}", line: 5 },
  { file: "health.hurl", entry: 1, method: "GET", url: "{{base_url}}/health", line: 1 },
];

describe("tree rows", () => {
  it("shows folders open, files closed, requests of expanded files", () => {
    const r = rows(tree, byFile(index), { closed: new Set(), expanded: new Set(["orders/checkout.hurl"]), filter: "" });
    expect(r.map((x) => x.kind)).toEqual(["dir", "file", "req", "req", "file"]);
    const closed = rows(tree, byFile(index), { closed: new Set(["orders"]), expanded: new Set(), filter: "" });
    expect(closed.map((x) => x.kind)).toEqual(["dir", "file"]);
  });

  it("filters requests across files", () => {
    const r = rows(tree, byFile(index), { closed: new Set(["orders"]), expanded: new Set(), filter: "get /orders" });
    expect(r.map((x) => (x.kind === "req" ? x.req.url : x.path))).toEqual(["orders", "orders/checkout.hurl", "{{base_url}}/orders/{{id}}"]);
    expect(rows(tree, byFile(index), { closed: new Set(), expanded: new Set(), filter: "nothing" })).toEqual([]);
  });

  it("adds the requests the Go filter matched in their headers or body", () => {
    const matches = new Map([["health.hurl", new Set([1])]]);
    const r = rows(tree, byFile(index), { closed: new Set(), expanded: new Set(), filter: "x-probe", matches });
    expect(r.map((x) => (x.kind === "req" ? x.req.url : x.path))).toEqual(["health.hurl", "{{base_url}}/health"]);
    expect(r[1].kind === "req" && r[1].hl).toBeUndefined();
  });

  it("filters 1000 files quickly", () => {
    const big: Node = { name: "", path: "", kind: "dir", children: [] };
    const idx: Request[] = [];
    for (let i = 0; i < 1000; i++) {
      big.children!.push({ name: `f${i}.hurl`, path: `f${i}.hurl`, kind: "request" });
      idx.push({ file: `f${i}.hurl`, entry: 1, method: "GET", url: `{{base_url}}/items/${i}`, line: 1 });
    }
    const m = byFile(idx);
    const t0 = performance.now();
    rows(big, m, { closed: new Set(), expanded: new Set(), filter: "items/99" });
    expect(performance.now() - t0).toBeLessThan(100);
  });
});
