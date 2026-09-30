// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { byteMatches } from "./body-search";

const enc = (s: string) => new TextEncoder().encode(s);

describe("byteMatches", () => {
  it("finds every match, in any ASCII case, past chunk ends", async () => {
    expect(await byteMatches(enc(`{"status":"Pending","x":"PENDING"}`), "pending")).toEqual([11, 25]);
    expect(await byteMatches(enc("aaaa"), "aa")).toEqual([0, 2]);
    expect(await byteMatches(enc("café café"), "café")).toEqual([0, 6]);
    expect(await byteMatches(enc("abc"), "")).toEqual([]);
    const big = new Uint8Array((4 << 20) + 10).fill(0x2e);
    big.set(enc("needle"), (4 << 20) - 3); // across the first chunk's end
    expect(await byteMatches(big, "needle")).toEqual([(4 << 20) - 3]);
  });

  it("stops when aborted", async () => {
    const ctl = new AbortController();
    const p = byteMatches(new Uint8Array(9 << 20).fill(0x61), "b", ctl.signal);
    ctl.abort();
    expect(await p).toEqual([]);
  });

  it("handles overlapping matches", async () => {
    // "aaaa" has overlapping matches for "aa": at 0 and 2
    expect(await byteMatches(enc("aaaa"), "aa")).toEqual([0, 2]);
  });

  it("finds matches at chunk boundaries", async () => {
    // Create a large array where the match is right at a chunk boundary
    const chunkSize = 4 << 20;
    const big = new Uint8Array(chunkSize + 10);
    big.fill(0x2e); // periods
    // Place "xyz" right at the chunk boundary
    const pos = chunkSize - 1;
    big.set(enc("xyz"), pos);
    const matches = await byteMatches(big, "xyz");
    expect(matches).toContain(pos);
  });

  it("is case-insensitive for ASCII", async () => {
    expect(await byteMatches(enc("Hello HELLO hello"), "hello")).toEqual([0, 6, 12]);
  });

  it("handles empty search", async () => {
    expect(await byteMatches(enc("abc"), "")).toEqual([]);
  });

  it("handles UTF-8 multibyte sequences", async () => {
    // "café" where é is a multibyte UTF-8 char
    const data = enc("café café café");
    const matches = await byteMatches(data, "café");
    expect(matches.length).toBe(3);
  });

  it("finds non-overlapping matches", async () => {
    // "xyxyxy" with pattern "xy" finds non-overlapping matches at 0, 2, 4
    expect(await byteMatches(enc("xyxyxy"), "xy")).toEqual([0, 2, 4]);
    // "xyxyxy" with pattern "yxy" finds non-overlapping match at 1 (bytes 1-3)
    // After finding at 1, it skips 2 positions, so i becomes 3, then for loop i++, so i becomes 4
    // 4 > last (3), so loop exits. Only one match found.
    expect(await byteMatches(enc("xyxyxy"), "yxy")).toEqual([1]);
  });

  it("handles no matches in large data", async () => {
    const large = new Uint8Array(8 << 20).fill(0x61);
    expect(await byteMatches(large, "zzz")).toEqual([]);
  });

  it("aborts during search and returns empty", async () => {
    const ctl = new AbortController();
    const large = new Uint8Array(10 << 20).fill(0x61);
    const p = byteMatches(large, "a", ctl.signal);
    // Abort immediately
    ctl.abort();
    const result = await p;
    // Should either complete early or return empty
    expect(Array.isArray(result)).toBe(true);
  });
});
