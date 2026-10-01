// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { expiresText } from "./cookie-jar";

describe("expiresText", () => {
  const now = 1_700_000_000_000; // ms; expiries are in seconds
  const inDays = (d: number) => now / 1000 + d * 86_400;

  it("says session for a cookie without an expiry", () => {
    expect(expiresText(0, now)).toBe("session");
  });

  it("says expired once the time has passed", () => {
    expect(expiresText(inDays(-1), now)).toBe("expired");
  });

  it("says today within half a day", () => {
    expect(expiresText(inDays(0.4), now)).toBe("today");
  });

  it("counts days to the nearest one", () => {
    expect(expiresText(inDays(1), now)).toBe("in 1 day");
    expect(expiresText(inDays(2.4), now)).toBe("in 2 days");
    expect(expiresText(inDays(2.6), now)).toBe("in 3 days");
  });
});
