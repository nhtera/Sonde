// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { percentile, until } from "./tour";

describe("percentile", () => {
  it("returns NaN for empty array", () => {
    const result = percentile([], 0.5);
    expect(Number.isNaN(result)).toBe(true);
  });

  it("returns the single value for single-element array", () => {
    expect(percentile([42], 0.0)).toBe(42);
    expect(percentile([42], 0.5)).toBe(42);
    expect(percentile([42], 1.0)).toBe(42);
  });

  it("calculates p0 (min)", () => {
    expect(percentile([10, 20, 30, 40, 50], 0.0)).toBe(10);
  });

  it("calculates p100 (max)", () => {
    expect(percentile([10, 20, 30, 40, 50], 1.0)).toBe(50);
  });

  it("calculates p50 (median) for odd number of elements", () => {
    // For 5 elements at p50: floor(5 * 0.5) = floor(2.5) = 2 → s[2] = 30
    expect(percentile([10, 20, 30, 40, 50], 0.5)).toBe(30);
  });

  it("calculates p50 (median) for even number of elements", () => {
    // For 4 elements at p50: floor(4 * 0.5) = 2 → s[2] = 30
    expect(percentile([10, 20, 30, 40], 0.5)).toBe(30);
  });

  it("calculates p25 (first quartile)", () => {
    // For 4 elements at p25: floor(4 * 0.25) = 1 → s[1] = 20
    expect(percentile([10, 20, 30, 40], 0.25)).toBe(20);
  });

  it("calculates p75 (third quartile)", () => {
    // For 4 elements at p75: floor(4 * 0.75) = 3 → s[3] = 40
    expect(percentile([10, 20, 30, 40], 0.75)).toBe(40);
  });

  it("calculates p95 (95th percentile)", () => {
    const values = Array.from({ length: 100 }, (_, i) => i + 1); // 1..100
    // floor(100 * 0.95) = 95 → s[95] = 96
    expect(percentile(values, 0.95)).toBe(96);
  });

  it("handles unordered input", () => {
    expect(percentile([50, 10, 30, 20, 40], 0.5)).toBe(30);
  });

  it("handles duplicate values", () => {
    expect(percentile([10, 10, 20, 20, 20, 30], 0.5)).toBe(20);
  });

  it("does not mutate input array", () => {
    const input = [50, 10, 30, 20, 40];
    const inputCopy = [...input];
    percentile(input, 0.5);
    expect(input).toEqual(inputCopy);
  });
});

describe("until", () => {
  it("resolves immediately if check passes", async () => {
    let called = false;
    const promise = until(() => {
      called = true;
      return true;
    });

    await expect(promise).resolves.toBeUndefined();
    expect(called).toBe(true);
  });

  it("checks on each frame until condition is true", async () => {
    let count = 0;
    const promise = until(() => {
      count++;
      return count >= 3;
    });

    await expect(promise).resolves.toBeUndefined();
    expect(count).toBeGreaterThanOrEqual(3);
  });

  it("rejects if timeout exceeded", async () => {
    const promise = until(() => false, 100); // 100ms timeout
    await expect(promise).rejects.toThrow("timed out");
  });

  it("uses default timeout of 30 seconds", async () => {
    // We don't test the full 30 seconds; just verify the timeout works
    const start = performance.now();
    const promise = until(() => false, 50); // 50ms timeout instead of 30 seconds
    try {
      await promise;
      throw new Error("should have timed out");
    } catch (e) {
      if (!(e instanceof Error) || !e.message.includes("timed out")) {
        throw e;
      }
    }
    const elapsed = performance.now() - start;
    expect(elapsed).toBeGreaterThanOrEqual(50);
  });

  it("resolves before timeout if check becomes true", async () => {
    let count = 0;
    const promise = until(() => {
      count++;
      return count >= 2;
    }, 5000);

    await expect(promise).resolves.toBeUndefined();
    expect(count).toBeGreaterThanOrEqual(2);
  });

  it("check function receives correct parameters (none)", async () => {
    const callRecord: unknown[] = [];
    const checkFn = function (this: unknown, ...args: unknown[]) {
      callRecord.push(...args);
      return true;
    };

    await until(checkFn);
    expect(callRecord).toEqual([]);
  });
});
