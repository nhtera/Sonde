// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { hunk } from "./hunk";

describe("hunk", () => {
  it("is the lines added, the shared ones left out", () => {
    const before = "GET /a\nHTTP 200\n# pm.expect(x)\n";
    const after = "GET /a\nHTTP 200\n# pm.expect(x)\n[Asserts]\njsonpath \"$.x\" == 1\n";
    expect(hunk(before, after)).toEqual({ line: 4, lines: [{ kind: "add", text: "[Asserts]" }, { kind: "add", text: 'jsonpath "$.x" == 1' }] });
  });

  it("shows a replaced line as removed and added", () => {
    expect(hunk("a\nAuthorization: Bearer x\nb", "a\nAuthorization: Bearer {{t}}\nb").lines).toEqual([
      { kind: "del", text: "Authorization: Bearer x" },
      { kind: "add", text: "Authorization: Bearer {{t}}" },
    ]);
  });

  it("keeps two regions apart: unchanged lines between them are not shown", () => {
    const before = "# list orders\nGET /orders\nHTTP 200";
    const after = "POST /login\nHTTP 200\n\n# list orders\nGET /orders\nAuthorization: Bearer {{t}}\nHTTP 200";
    const h = hunk(before, after);
    expect(h.line).toBe(1);
    expect(h.lines.map((l) => l.kind)).toEqual(["add", "add", "add", "gap", "add"]);
    expect(h.lines.some((l) => l.text === "GET /orders")).toBe(false);
  });

  it("is empty for the same text", () => {
    expect(hunk("a\nb", "a\nb")).toEqual({ line: 3, lines: [] });
  });
});
