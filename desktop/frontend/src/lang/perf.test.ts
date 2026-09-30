// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

// A keystroke in a 5,000-line file re-highlights in under 16 ms (p95): an
// incremental parse from the previous tree, then the visible lines'
// highlighting.

import { TreeFragment, type Tree } from "@lezer/common";
import { classHighlighter, highlightTree } from "@lezer/highlight";
import { describe, expect, it } from "vitest";
import { sondeFileLanguage } from "./language";

/** A file of entries (request line, headers, JSON body, captures,
 * asserts) repeated to about lines lines. */
function bigFile(lines: number): string {
  const entry = [
    "# Create order {{n}}",
    "POST {{base_url}}/orders?page={{n}}",
    "Authorization: Bearer {{token}}",
    "Content-Type: application/json",
    "{",
    '  "sku": "TEA-{{n}}",',
    '  "quantity": {{quantity}},',
    '  "tags": ["a", "b", {{tag}}]',
    "}",
    "HTTP 201",
    "[Captures]",
    'order_id: jsonpath "$.id"',
    "[Asserts]",
    'jsonpath "$.status" == "created"',
    'header "Location" matches /orders\\/\\d+/',
    "",
  ];
  const out: string[] = [];
  for (let i = 0; out.length < lines; i++) out.push(...entry.map((l) => l.replaceAll("{{n}}", String(i))));
  return out.join("\n");
}

describe("highlighting performance", () => {
  it("re-highlights a keystroke in a 5,000-line file under 16 ms (p95)", () => {
    const parser = sondeFileLanguage.parser;
    let doc = bigFile(5000);
    let tree: Tree = parser.parse(doc);
    let fragments = TreeFragment.addTree(tree);
    const lineStarts = [0];
    for (let i = 0; i < doc.length; i++) if (doc[i] === "\n") lineStarts.push(i + 1);

    const times: number[] = [];
    for (let k = 0; k < 120; k++) {
      // Keystrokes spread over the file: in URLs, headers and JSON bodies.
      const line = 7 + ((k * 397) % (lineStarts.length - 20));
      const at = lineStarts[line] + 2;
      const start = performance.now();
      doc = doc.slice(0, at) + "x" + doc.slice(at);
      fragments = TreeFragment.applyChanges(fragments, [{ fromA: at, toA: at, fromB: at, toB: at + 1 }]);
      tree = parser.parse(doc, fragments);
      fragments = TreeFragment.addTree(tree, fragments);
      // The visible lines: about 60 around the edit.
      let spans = 0;
      highlightTree(tree, classHighlighter, () => spans++, Math.max(0, at - 1500), Math.min(doc.length, at + 1500));
      times.push(performance.now() - start);
      expect(spans).toBeGreaterThan(0);
      for (let i = line + 1; i < lineStarts.length; i++) lineStarts[i]++;
    }
    times.sort((a, b) => a - b);
    const p95 = times[Math.floor(times.length * 0.95)];
    console.log(`keystroke to highlight: p50 ${times[times.length >> 1].toFixed(2)} ms, p95 ${p95.toFixed(2)} ms`);
    expect(p95).toBeLessThan(16);
  });

  it("re-highlights a keystroke that breaks a JSON body under 16 ms (p95)", () => {
    const parser = sondeFileLanguage.parser;
    const base = bigFile(5000);
    let tree: Tree = parser.parse(base);
    const times: number[] = [];
    // An unclosed string in a body, at 40 places over the file: the body
    // becomes plain lines up to the next response, not to the file's end.
    const bodies = [...base.matchAll(/"sku": "/g)].map((m) => m.index! + 8);
    for (let k = 0; k < 40; k++) {
      const at = bodies[(k * 37) % bodies.length];
      const doc = base.slice(0, at) + '"' + base.slice(at);
      const fragments = TreeFragment.applyChanges(TreeFragment.addTree(tree), [{ fromA: at, toA: at, fromB: at, toB: at + 1 }]);
      const start = performance.now();
      const next = parser.parse(doc, fragments);
      highlightTree(next, classHighlighter, () => {}, Math.max(0, at - 1500), Math.min(doc.length, at + 1500));
      times.push(performance.now() - start);
      tree = parser.parse(base);
    }
    times.sort((a, b) => a - b);
    const p95 = times[Math.floor(times.length * 0.95)];
    console.log(`broken body keystroke: p95 ${p95.toFixed(2)} ms`);
    expect(p95).toBeLessThan(16);
  });
});
