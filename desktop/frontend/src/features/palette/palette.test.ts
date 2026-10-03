// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { paletteFilter } from "./palette";

describe("paletteFilter", () => {
  it("matches a request by method and path, not by its file", () => {
    const value = "POST /carts/{{cart_id}}/items \tcheckout.hurl#4";
    expect(paletteFilter(value, "cart")).toBeGreaterThan(0);
    expect(paletteFilter(value, "post /carts")).toBeGreaterThan(0);
    expect(paletteFilter(value, "checkout.hurl")).toBe(0);
  });

  it("ranks an earlier match higher and ignores the command prefix", () => {
    expect(paletteFilter(">Run file", ">run")).toBeGreaterThan(paletteFilter(">Toggle run", ">run"));
    expect(paletteFilter("users.hurl", "")).toBe(1);
  });
});
