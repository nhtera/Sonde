// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { autoCollapse, buildTree, indexOf, isClose, isOpen, JsonError, keyText, Kind, nodeAt, pathOf, valueText, visibleRows } from "./json-tree";

const enc = new TextEncoder();
const tree = (s: string) => buildTree(enc.encode(s));

/** Each row as `depth key value`. */
function rows(s: string, folded: number[] = []) {
  const t = tree(s);
  const folds = new Uint8Array(t.count);
  folded.forEach((n) => (folds[n] = 1));
  return [...visibleRows(t, folds)].map((i) =>
    [t.depth[i], keyText(t, i) || (indexOf(t, i) >= 0 ? `[${indexOf(t, i)}]` : ""), valueText(t, i).text].filter((p) => p !== "").join(" "),
  );
}

describe("buildTree", () => {
  it("keeps every number as written", () => {
    const t = tree(`{"id": 9007199254740993, "big": 12345678901234567890, "f": -1.50e+3, "z": 0}`);
    const values = [...visibleRows(t, null)].filter((i) => t.kind[i] === Kind.Number).map((i) => valueText(t, i).text);
    expect(values).toEqual(["9007199254740993", "12345678901234567890", "-1.50e+3", "0"]);
  });

  it("flattens objects and arrays with closing rows and counts", () => {
    expect(rows(`{"a": [1, {"b": null}], "c": "x\\"y", "e": {}, "t": true}`)).toEqual([
      "0 {",
      `1 "a" [`,
      "2 [0] 1",
      "2 [1] {",
      `3 "b" null`,
      "2 }",
      "1 ]",
      `1 "c" "x\\"y"`,
      `1 "e" {`,
      "1 }",
      `1 "t" true`,
      "0 }",
    ]);
    const t = tree(`[1, [2, 3], []]`);
    expect(t.aux[t.aux[0]]).toBe(3); // the root's children
    expect(t.aux[t.aux[2]]).toBe(2);
    expect(t.aux[t.aux[6]]).toBe(0);
  });

  it("builds paths for +Assert", () => {
    const t = tree(`{"items": [{"id": 1, "a b": {"it's": 2}}]}`);
    const find = (text: string) => [...visibleRows(t, null)].find((i) => keyText(t, i) === text)!;
    expect(pathOf(t, find(`"id"`))).toBe("$.items[0].id");
    expect(pathOf(t, find(`"it's"`))).toBe("$.items[0]['a b']['it\\'s']");
    expect(pathOf(t, 0)).toBe("$");
  });

  it("folds containers", () => {
    const s = `{"a": [1, 2], "b": 3}`;
    expect(rows(s, [1])).toEqual(["0 {", `1 "a" [`, `1 "b" 3`, "0 }"]);
  });

  it("folds the deepest levels first for a large body", () => {
    const t = tree(`[${Array.from({ length: 50 }, (_, i) => `{"id": ${i}, "tags": ["x"]}`).join(",")}]`);
    const folded = autoCollapse(t, 120);
    expect(visibleRows(t, folded).length).toBeLessThanOrEqual(120);
    expect(folded[0]).toBe(0); // never the root
    expect(autoCollapse(t, 100_000).every((f) => f === 0)).toBe(true);
  });

  it("finds the row of a byte offset", () => {
    const s = `{"a": 1, "bb": "pending"}`;
    const t = tree(s);
    expect(keyText(t, nodeAt(t, s.indexOf("pending")))).toBe(`"bb"`);
    expect(keyText(t, nodeAt(t, s.indexOf("bb")))).toBe(`"bb"`);
  });

  it("cuts long values", () => {
    const t = tree(`["${"x".repeat(1000)}"]`);
    const v = valueText(t, 1, 100);
    expect(v.cut).toBe(true);
    expect(v.text.length).toBe(100);
  });

  it("refuses what is not JSON", () => {
    for (const bad of [`{"a" 1}`, `[1,]`, `{"a": 1`, `nul`, `"x`, `[1] 2`, `{,}`, ``, `{"a":01x}`]) {
      expect(() => tree(bad), bad).toThrow(JsonError);
    }
    expect(() => tree(" \n[true, false, null, -0.5e-2] \n")).not.toThrow();
  });

  it("parses 50 MB in well under the budget", () => {
    const item = `{"id":12345678901234567890,"sku":"TEA-00001","status":"paid","total":12.5,"currency":"EUR"}`;
    const count = Math.ceil((50 << 20) / (item.length + 1));
    const bytes = enc.encode(`[${Array(count).fill(item).join(",")}]`);
    const t0 = performance.now();
    const t = buildTree(bytes);
    const ms = performance.now() - t0;
    expect(t.count).toBe(count * 7 + 2);
    // The worker's budget is 3 s end to end; the parse alone is far below.
    expect(ms).toBeLessThan(2500);
  });

  describe("edge cases: escapes, numbers, and formats", () => {
    it("preserves unicode escapes exactly", () => {
      const t = tree(`{"emoji":"\\u0041","path":"\\u002Fhome","null":"\\u0000"}`);
      const emojis = [...visibleRows(t, null)].filter((i) => keyText(t, i) === `"emoji"`);
      expect(valueText(t, emojis[0]).text).toBe(`"\\u0041"`);
    });

    it("handles surrogate pairs in strings", () => {
      // 😀 is U+1F600, encoded as \\uD83D\\uDE00 in JSON
      const t = tree(`{"smile":"\\uD83D\\uDE00"}`);
      const nodes = [...visibleRows(t, null)].filter((i) => keyText(t, i) === `"smile"`);
      const text = valueText(t, nodes[0]).text;
      expect(text).toContain("\\uD83D\\uDE00");
    });

    it("preserves negative zero", () => {
      const t = tree(`{"z":-0}`);
      const nodes = [...visibleRows(t, null)].filter((i) => t.kind[i] === Kind.Number);
      expect(valueText(t, nodes[0]).text).toBe("-0");
    });

    it("preserves scientific notation", () => {
      const t = tree(`{"big":1.5E+10,"small":2.5e-3,"neg":-3E+2}`);
      const values = [...visibleRows(t, null)].filter((i) => t.kind[i] === Kind.Number).map((i) => valueText(t, i).text);
      expect(values).toEqual(["1.5E+10", "2.5e-3", "-3E+2"]);
    });

    it("handles various whitespace", () => {
      // Tabs, spaces, newlines, carriage returns around values and separators
      const t = tree(`{  "a" : \n  1  , \r\n "b" : [ \t 2 \t ] \n }`);
      // Counts: object, a:1, b:array, 2 (in array), close array, close object = 6
      expect([...visibleRows(t, null)].length).toBe(6);
    });

    it("parses BOM at the start", () => {
      const withBom = new Uint8Array([0xef, 0xbb, 0xbf, ...enc.encode(`{"x":1}`)]);
      const t = buildTree(withBom);
      const nodes = [...visibleRows(t, null)].filter((i) => t.kind[i] === Kind.Number);
      expect(valueText(t, nodes[0]).text).toBe("1");
    });

    it("rejects bad numbers and incomplete values", () => {
      for (const bad of [
        `[1e]`, // e without exponent
        `[+5]`, // leading + is not valid JSON
        `[.5]`, // leading . is not valid JSON
        `["unterminated`, // unterminated string
      ]) {
        expect(() => tree(bad), bad).toThrow(JsonError);
      }
    });

    it("rejects control characters in strings", () => {
      const tab = new Uint8Array(enc.encode(`{"a":"b\t"}`));
      tab[8] = 0x09; // replace " with a tab
      expect(() => buildTree(tab)).toThrow(JsonError);
    });
  });

  describe("path evaluation (round-trip)", () => {
    /** Evaluates the paths pathOf writes ($, .name, ['name'], [i]). */
    function evaluate(root: unknown, path: string): unknown {
      let v = root as Record<string, unknown>;
      const re = /\.([A-Za-z_][A-Za-z0-9_]*)|\['((?:[^'\\]|\\.)*)'\]|\[(\d+)\]/y;
      re.lastIndex = 1;
      while (re.lastIndex < path.length) {
        const m = re.exec(path);
        if (!m) throw new Error(`bad path ${path}`);
        const key = m[1] ?? (m[2] !== undefined ? m[2].replace(/\\(.)/g, "$1") : Number(m[3]));
        v = v[key as string] as Record<string, unknown>;
      }
      return v;
    }

    it("each value's path finds that value", () => {
      const doc = `{"items":[{"id":42,"name":"x"},[true,null,{"deep":[1.5,"y"]}]],"a b":{"it's":"q","q\\"t":2},"":0,"é":"ü"}`;
      const t = tree(doc);
      const root = JSON.parse(doc);
      let checked = 0;
      for (let i = 0; i < t.count; i++) {
        const k = t.kind[i];
        if (k === Kind.CloseObject || k === Kind.CloseArray) continue;
        const got = evaluate(root, pathOf(t, i));
        if (k === Kind.Object || k === Kind.Array) {
          expect(typeof got, pathOf(t, i)).toBe("object");
          continue;
        }
        expect(got, pathOf(t, i)).toEqual(JSON.parse(valueText(t, i).text));
        checked++;
      }
      expect(checked).toBe(10);
    });
  });

  describe("visibleRows and autoCollapse invariants", () => {
    it("visible rows never exceed total rows or skip middle rows", () => {
      const t = tree(`{"a":[1,2,{"b":3}],"c":4}`);
      const visible = visibleRows(t, null);
      expect(visible.length).toBeLessThanOrEqual(t.count);
      // Rows must be in order.
      for (let i = 1; i < visible.length; i++) {
        expect(visible[i] > visible[i - 1]).toBe(true);
      }
    });

    it("folding a node skips it and its children", () => {
      const t = tree(`{"a":[1,[2,3]],"b":4}`);
      const folds = new Uint8Array(t.count);
      folds[1] = 1; // fold the "a" array
      const visible = visibleRows(t, folds);
      for (const idx of visible) {
        // The folded array is at index 1; its children should not appear.
        if (idx > 1 && t.parent[idx] === 1) {
          throw new Error(`child of folded node at ${idx}`);
        }
      }
    });

    it("autoCollapse reduces rows but never folds the root", () => {
      const t = tree(`[${Array.from({ length: 20 }, (_, i) => `{"id":${i}}`).join(",")}]`);
      const folds = autoCollapse(t, 50);
      let visibleCount = 0;
      for (let i = 0; i < t.count; i++) {
        if (!folds[i]) visibleCount++;
        if (folds[i] && isOpen(t.kind[i])) {
          i = t.aux[i]; // skip to closing bracket
        }
      }
      expect(visibleCount).toBeLessThanOrEqual(50);
      expect(folds[0]).toBe(0); // never fold the root
    });

  });

  describe("nodeAt with various boundaries", () => {
    it("finds nodes by key boundaries", () => {
      const s = `{"key1":1,"key2":2}`;
      const t = tree(s);
      const keyStart1 = s.indexOf(`"key1"`);
      const keyStart2 = s.indexOf(`"key2"`);
      expect(keyText(t, nodeAt(t, keyStart1))).toBe(`"key1"`);
      expect(keyText(t, nodeAt(t, keyStart2))).toBe(`"key2"`);
    });

    it("finds nodes by value boundaries", () => {
      const s = `{"a":100,"b":"long string"}`;
      const t = tree(s);
      expect(valueText(t, nodeAt(t, s.indexOf("100"))).text).toBe("100");
      expect(valueText(t, nodeAt(t, s.indexOf("long"))).text).toContain("long");
    });

    it("finds closing bracket nodes", () => {
      const s = `{"x":[1,2]}`;
      const t = tree(s);
      const closeIdx = s.lastIndexOf("]");
      const node = nodeAt(t, closeIdx);
      expect(isClose(t.kind[node])).toBe(true);
    });

    it("handles edge offsets", () => {
      const s = `[0,1,2]`;
      const t = tree(s);
      expect(t.kind[nodeAt(t, 0)]).toBe(Kind.Array);
      expect(t.kind[nodeAt(t, s.length - 1)]).toBe(Kind.CloseArray);
    });
  });
});

describe("review fixes", () => {
  it("refuses leading zeros, as Go's decoder does", () => {
    for (const bad of [`[01]`, `[-012]`, `{"a":00}`]) expect(() => tree(bad), bad).toThrow(JsonError);
    expect(() => tree(`[0, -0, 0.5, 10, -0e1]`)).not.toThrow();
  });

  it("escapes control characters and quotes in a path's names", () => {
    const t = tree(`{"a\\nb":{"it's\\t\\u0001":1}}`);
    const leaf = [...visibleRows(t, null)].find((i) => t.kind[i] === Kind.Number)!;
    expect(pathOf(t, leaf)).toBe(`$['a\\nb']['it\\'s\\t\\u0001']`);
  });
});
