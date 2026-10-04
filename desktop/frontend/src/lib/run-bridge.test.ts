// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

// Events are delivered by the test through this fake bus.
const bus = new Map<string, Set<(d: unknown) => void>>();
vi.mock("./events", () => ({
  on: (topic: string, h: (d: unknown) => void) => {
    if (!bus.has(topic)) bus.set(topic, new Set());
    bus.get(topic)!.add(h);
    return () => bus.get(topic)?.delete(h);
  },
}));

const { startRun } = await import("./run-bridge");

function emit(topic: string, data: unknown) {
  bus.get(topic)?.forEach((h) => h(data));
}

beforeEach(() => bus.clear());

describe("run bridge", () => {
  it("subscribes before starting, orders by seq, applies done after lastSeq", async () => {
    const seen: number[] = [];
    const onDone = vi.fn();
    let started = "";
    const h = startRun<string>(
      (runId) => {
        started = runId;
        // Subscribed first: the run's own topic has a listener already.
        expect(bus.get(`run:${runId}`)?.size).toBe(1);
        return new Promise(() => {}) as Promise<string>;
      },
      { onItems: (items) => seen.push(...items.map((i) => i.seq)), onDone },
    );
    expect(started).toBe(h.runId);
    emit(`run:${h.runId}`, { runId: h.runId, items: [{ seq: 2, unit: 0, event: {} }], dropped: 0 });
    expect(seen).toEqual([]); // waits for seq 1
    emit(`run:${h.runId}:done`, { runId: h.runId, lastSeq: 3, summary: "ok" });
    expect(onDone).not.toHaveBeenCalled(); // 1 and 3 are missing
    emit(`run:${h.runId}`, { runId: h.runId, items: [{ seq: 1, unit: 0, event: {} }, { seq: 3, unit: 0, event: {} }], dropped: 0 });
    expect(seen).toEqual([1, 2, 3]);
    expect(onDone).toHaveBeenCalledWith("ok");
    await expect(h.result).resolves.toBe("ok");
    expect(bus.get(`run:${h.runId}`)?.size ?? 0).toBe(0); // unsubscribed
  });

  it("a cancel stops the run through the binding: it ends with its own summary", async () => {
    const callCancel = vi.fn();
    const binding = vi.fn();
    const onDone = vi.fn();
    const h = startRun<string>(() => Object.assign(new Promise<string>(() => {}), { cancel: callCancel }), { onItems: () => {}, onDone }, binding);
    h.cancel();
    expect(binding).toHaveBeenCalledWith(h.runId);
    expect(callCancel).not.toHaveBeenCalled();
    emit(`run:${h.runId}:done`, { runId: h.runId, lastSeq: 0, summary: "canceled" });
    await expect(h.result).resolves.toBe("canceled");
    expect(onDone).toHaveBeenCalledWith("canceled");
  });

  it("a run whose events arrive before the call returns still finishes", async () => {
    const onDone = vi.fn();
    const h = startRun<string>(
      (runId) => {
        emit(`run:${runId}`, { runId, items: [{ seq: 1, unit: 0, event: {} }], dropped: 0 });
        emit(`run:${runId}:done`, { runId, lastSeq: 1, summary: "fast" });
        return Promise.resolve("fast");
      },
      { onItems: () => {}, onDone },
    );
    await expect(h.result).resolves.toBe("fast");
    expect(onDone).toHaveBeenCalledOnce();
  });

  it("a rejected call ends the run", async () => {
    const h = startRun<string>(() => Promise.reject(new Error("busy")), { onItems: () => {}, onDone: () => {} });
    await expect(h.result).rejects.toThrow("busy");
  });

  it("a lost Done: the call's summary ends the run", async () => {
    vi.useFakeTimers();
    try {
      const seen: number[] = [];
      const onDone = vi.fn();
      const h = startRun<string>(
        (runId) => {
          // seq 1 was lost with the event stream; 2 arrived; Done never did.
          emit(`run:${runId}`, { runId, items: [{ seq: 2, unit: 0, event: {} }], dropped: 0 });
          return Promise.resolve("summary");
        },
        { onItems: (items) => seen.push(...items.map((i) => i.seq)), onDone },
      );
      await vi.advanceTimersByTimeAsync(999);
      expect(onDone).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(1);
      expect(seen).toEqual([2]); // what did arrive is applied
      expect(onDone).toHaveBeenCalledWith("summary");
      await expect(h.result).resolves.toBe("summary");
    } finally {
      vi.useRealTimers();
    }
  });

  it("reports dropped stream messages", () => {
    const drops: number[] = [];
    const h = startRun<string>(() => new Promise(() => {}) as Promise<string>, {
      onItems: (_i, d) => drops.push(d),
      onDone: () => {},
    });
    emit(`run:${h.runId}`, { runId: h.runId, items: [], dropped: 4 });
    expect(drops).toEqual([4]);
  });
});
