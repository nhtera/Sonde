// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

// An incremental parse after an edit gives the same file tree as parsing
// the edited text afresh (seeded random edits over the test projects).

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { TreeFragment } from "@lezer/common";
import { describe, expect, it } from "vitest";
import { parser } from "./sonde.grammar";

const root = fileURLToPath(new URL("../../../../", import.meta.url));

/** A small seeded PRNG (mulberry32), so failures reproduce. */
function rng(seed: number) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const inserts = ["x", " ", "\n", "{", "}", '"', "{{", "}}", "[", "]", "<", ">", "#", ":", "`", "\\", "GET ", "HTTP 200\n", "[Asserts]\n"];

describe("incremental parsing", () => {
  const dir = join(root, "desktop", "testdata", "shop-api");
  const files = readdirSync(dir)
    .filter((f) => /\.(hurl|sonde)$/.test(f))
    .map((f) => readFileSync(join(dir, f), "utf8"));

  it("matches a fresh parse after random edits", () => {
    const rand = rng(20260930);
    let checked = 0;
    for (const original of files) {
      let doc = original;
      let tree = parser.parse(doc);
      for (let k = 0; k < 80; k++) {
        const from = Math.floor(rand() * (doc.length + 1));
        const del = rand() < 0.3 ? Math.min(doc.length - from, Math.floor(rand() * 4)) : 0;
        const ins = rand() < 0.8 ? inserts[Math.floor(rand() * inserts.length)] : "";
        const next = doc.slice(0, from) + ins + doc.slice(from + del);
        const fragments = TreeFragment.applyChanges(TreeFragment.addTree(tree), [{ fromA: from, toA: from + del, fromB: from, toB: from + ins.length }]);
        tree = parser.parse(next, fragments);
        expect(tree.toString(), `edit ${k} at ${from}`).toBe(parser.parse(next).toString());
        doc = next;
        checked++;
      }
    }
    expect(checked).toBeGreaterThan(300);
  });
});
