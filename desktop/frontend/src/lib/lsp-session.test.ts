// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const bus = new Map<string, Set<(d: unknown) => void>>();
vi.mock("./events", () => ({
  on: (topic: string, h: (d: unknown) => void) => {
    if (!bus.has(topic)) bus.set(topic, new Set());
    bus.get(topic)!.add(h);
    return () => bus.get(topic)?.delete(h);
  },
}));

let opened = 0;
const Lsp = {
  Open: vi.fn(async () => `s${++opened}`),
  Ping: vi.fn(async (_id: string) => {}),
  Close: vi.fn(async (_id: string) => {}),
};
vi.mock("./api", () => ({ Lsp }));

const { lspSession, onLspSession, startLspSession } = await import("./lsp-session");

beforeEach(() => {
  vi.useFakeTimers();
  bus.clear();
  opened = 0;
  Object.values(Lsp).forEach((f) => f.mockClear());
});
afterEach(() => vi.useRealTimers());

describe("LSP session", () => {
  it("opens at start and closes on stop", async () => {
    const seen: (string | null)[] = [];
    const off = onLspSession((id) => seen.push(id));
    const stop = startLspSession();
    await vi.advanceTimersByTimeAsync(0);
    expect(lspSession()).toBe("s1");
    stop();
    expect(Lsp.Close).toHaveBeenCalledWith("s1");
    expect(seen).toEqual(["s1", null]);
    off();
  });

  it("reopens when the session ends, backing off", async () => {
    const stop = startLspSession();
    await vi.advanceTimersByTimeAsync(0);
    bus.get("lsp:s1:closed")!.forEach((h) => h("s1"));
    expect(lspSession()).toBeNull();
    await vi.advanceTimersByTimeAsync(1000);
    expect(lspSession()).toBe("s2");

    // A failing open waits twice as long each time.
    Lsp.Open.mockRejectedValueOnce(new Error("no server"));
    bus.get("lsp:s2:closed")!.forEach((h) => h("s2"));
    await vi.advanceTimersByTimeAsync(1000); // the open fails
    expect(lspSession()).toBeNull();
    await vi.advanceTimersByTimeAsync(1999);
    expect(Lsp.Open).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(1);
    expect(lspSession()).toBe("s3");
    stop();
  });

  it("pings every minute and reopens when the ping fails", async () => {
    const stop = startLspSession();
    await vi.advanceTimersByTimeAsync(0);
    Lsp.Ping.mockRejectedValueOnce(new Error("expired"));
    await vi.advanceTimersByTimeAsync(60_000);
    expect(Lsp.Ping).toHaveBeenCalledWith("s1");
    expect(lspSession()).toBeNull();
    await vi.advanceTimersByTimeAsync(1000);
    expect(lspSession()).toBe("s2");
    stop();
  });

  it("closes the session when the page unloads", async () => {
    const stop = startLspSession();
    await vi.advanceTimersByTimeAsync(0);
    window.dispatchEvent(new Event("beforeunload"));
    expect(Lsp.Close).toHaveBeenCalledWith("s1");
    stop();
  });
});
