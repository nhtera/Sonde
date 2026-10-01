// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { shorten } from "./decorations";

describe("shorten", () => {
  it("keeps a short warning whole", () => {
    expect(shorten("Parsing template variable: expecting a variable")).toBe("Parsing template variable: expecting a variable");
  });

  it("keeps a long warning's headline", () => {
    expect(shorten('undefined variable "trace_id": not captured earlier and not defined in environment "local"')).toBe('undefined variable "trace_id"');
  });

  it("cuts a long warning with no headline", () => {
    const s = shorten("x".repeat(80));
    expect(s).toHaveLength(60);
    expect(s.endsWith("…")).toBe(true);
  });
});
