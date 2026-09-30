// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { firstChangedLine } from "./stale-banner";

describe("stale banner", () => {
  it("names the first changed line", () => {
    expect(firstChangedLine("a\nb\nc", "a\nb\nc")).toBe(0);
    expect(firstChangedLine("a\nb\nc", "a\nB\nc")).toBe(2);
    expect(firstChangedLine("a\nb", "a\nb\nc")).toBe(2);
    expect(firstChangedLine("", "x")).toBe(1);
  });
});
