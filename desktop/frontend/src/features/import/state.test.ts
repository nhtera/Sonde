// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { ImportSuggestion } from "../../lib/api";
import { base64 } from "./pick";
import { grouped } from "./result-tile";
import { liftOf, ready, requestFile, tally } from "./state";

const cands = [
  { id: 0, name: "token", where: "Authorization header" },
  { id: 1, name: "api_key", where: "query api_key" },
];

describe("import helpers", () => {
  it("lifts every candidate until the user picks (with an env), then only those still offered", () => {
    expect(liftOf(cands, null, "local")).toEqual([0, 1]);
    expect(liftOf(cands, null, "")).toEqual([]);
    expect(liftOf(cands, [1, 7], "local")).toEqual([1]);
  });

  it("tallies the decisions", () => {
    const sugg = ["a", "b", "c"].map((path) => ({ path }) as ImportSuggestion);
    expect(tally(sugg, { a: "accepted", c: "rejected", gone: "accepted" })).toEqual({ accepted: 1, rejected: 1, pending: 1 });
  });

  it("needs an input to preview", () => {
    const req = { kind: "curl", input: "", text: "  ", environments: [], group: "", baseUrlVar: "", ext: "", folder: "", env: "", lift: null, target: "", targetText: "" };
    expect(ready(req)).toBe(false);
    expect(ready({ ...req, text: "curl x" })).toBe(true);
    expect(ready({ ...req, input: "id" })).toBe(true);
  });

  it("inserts into request files only", () => {
    expect(requestFile("a/b.hurl")).toBe(true);
    expect(requestFile("b.sonde")).toBe(true);
    expect(requestFile("sonde.yaml")).toBe(false);
    expect(requestFile(null)).toBe(false);
  });

  it("groups warnings by kind, counting each", () => {
    expect(grouped([
      { kind: "script", message: "a" },
      { kind: "unsupported", message: "b" },
      { kind: "script", message: "c" },
    ])).toEqual([["script", 2, "a"], ["unsupported", 1, "b"]]);
  });

  it("encodes a file's bytes in base64, large ones included", () => {
    expect(base64(new TextEncoder().encode("curl x"))).toBe(btoa("curl x"));
    const big = new Uint8Array(70_000).fill(65);
    expect(atob(base64(big)).length).toBe(70_000);
  });
});
