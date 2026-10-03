// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { isRequestPath } from "./files";

describe("isRequestPath", () => {
  it("is true for .hurl and .sonde files only", () => {
    expect(isRequestPath("orders/checkout.hurl")).toBe(true);
    expect(isRequestPath("ws.sonde")).toBe(true);
    for (const p of ["data/logins.csv", "sonde.yaml", "secrets/local.secrets", "notes.hurl.bak", "", null, undefined]) {
      expect(isRequestPath(p)).toBe(false);
    }
  });
});
