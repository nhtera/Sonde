// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { hunk } from "./hunk";

describe("hunk", () => {
  it("is the lines added between the shared start and end", () => {
    const before = "GET /a\nHTTP 200\n# pm.expect(x)\n";
    const after = "GET /a\nHTTP 200\n# pm.expect(x)\n[Asserts]\njsonpath \"$.x\" == 1\n";
    expect(hunk(before, after)).toEqual({ line: 4, removed: [], added: ["[Asserts]", 'jsonpath "$.x" == 1'] });
  });

  it("shows a replaced line as removed and added", () => {
    expect(hunk("a\nAuthorization: Bearer x\nb", "a\nAuthorization: Bearer {{t}}\nb")).toEqual({
      line: 2,
      removed: ["Authorization: Bearer x"],
      added: ["Authorization: Bearer {{t}}"],
    });
  });

  it("is empty for the same text", () => {
    expect(hunk("a\nb", "a\nb")).toEqual({ line: 3, removed: [], added: [] });
  });
});
