// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { dayOf } from "./history-panel";

// Local times: a day is the user's calendar day, whatever the time zone.
const at = (y: number, m: number, d: number, h: number) => new Date(y, m - 1, d, h).toISOString();

describe("dayOf", () => {
  const now = new Date(2026, 9, 15, 0, 30);

  it("says Today for any time of the same calendar day", () => {
    expect(dayOf(at(2026, 10, 15, 0), now)).toBe("Today");
    expect(dayOf(at(2026, 10, 15, 23), now)).toBe("Today");
  });

  it("says Yesterday for the day before, even an hour ago", () => {
    expect(dayOf(at(2026, 10, 14, 23), now)).toBe("Yesterday");
    expect(dayOf(at(2026, 10, 14, 0), now)).toBe("Yesterday");
  });

  it("gives the date of an older day", () => {
    const iso = at(2026, 10, 13, 12);
    expect(dayOf(iso, now)).toBe(new Date(iso).toLocaleDateString());
  });
});
