// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { refusedText, unresolvedText } from "./error-cards";

describe("error card lines", () => {
  it("says which address refused", () => {
    expect(refusedText("(7) Failed to connect to 127.0.0.1 port 8080: dial tcp: connection refused")).toBe("127.0.0.1:8080 did not accept the connection.");
    expect(refusedText("something else")).toBeUndefined();
  });
  it("says which host did not resolve", () => {
    expect(unresolvedText("(6) Could not resolve host: stg.shop.dev")).toBe("stg.shop.dev: no such host");
    expect(unresolvedText("timeout")).toBeUndefined();
  });
});
