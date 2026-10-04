// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const m = vi.hoisted(() => ({
  CloseGuard: { SetUnsaved: vi.fn(() => Promise.resolve()), Leave: vi.fn(() => Promise.resolve()) },
  handlers: new Map<string, (data: unknown) => void>(),
  answer: true,
  confirm: vi.fn(() => Promise.resolve(m.answer)),
}));
vi.mock("../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), CloseGuard: m.CloseGuard }));
vi.mock("../lib/events", () => ({ on: (topic: string, h: (data: unknown) => void) => (m.handlers.set(topic, h), () => m.handlers.delete(topic)) }));
vi.mock("../components/ask", () => ({ confirm: m.confirm }));

const { leaveMessage, startLeaveGuard } = await import("./leave-guard");
const { useTabs } = await import("../state/tabs");

const tab = (path: string, text = "") => ({ path, text, savedText: "", hash: "h", version: 1, conflict: false });
const flush = () => new Promise((r) => setTimeout(r));

beforeEach(() => {
  vi.clearAllMocks();
  m.answer = true;
  useTabs.setState({ tabs: [], active: null, closed: [] });
});

describe("leave guard", () => {
  it("reports the unsaved tabs as they change", () => {
    const stop = startLeaveGuard();
    expect(m.CloseGuard.SetUnsaved).toHaveBeenLastCalledWith(0);
    useTabs.setState({ tabs: [tab("a.hurl", "x"), tab("b.hurl")] });
    expect(m.CloseGuard.SetUnsaved).toHaveBeenLastCalledWith(1);
    useTabs.setState({ active: "b.hurl" }); // no change in count: no call
    expect(m.CloseGuard.SetUnsaved).toHaveBeenCalledTimes(2);
    stop();
  });

  it("asks before leaving, and leaves only when the user agrees", async () => {
    const stop = startLeaveGuard();
    useTabs.setState({ tabs: [tab("a.hurl", "x")] });
    m.answer = false;
    m.handlers.get("app:leave")?.(null);
    await flush();
    expect(m.confirm).toHaveBeenCalledWith(expect.objectContaining({ message: "a.hurl has unsaved changes. Quit without saving?" }));
    expect(m.CloseGuard.Leave).not.toHaveBeenCalled();
    m.answer = true;
    m.handlers.get("app:leave")?.(null);
    await flush();
    expect(m.CloseGuard.Leave).toHaveBeenCalledOnce();
    stop();
  });

  it("leaves at once when nothing is unsaved any more", () => {
    const stop = startLeaveGuard();
    m.handlers.get("app:leave")?.(null);
    expect(m.confirm).not.toHaveBeenCalled();
    expect(m.CloseGuard.Leave).toHaveBeenCalledOnce();
    stop();
  });

  it("names several files", () => {
    expect(leaveMessage(["a.hurl", "b.hurl"])).toBe("2 files have unsaved changes: a.hurl, b.hurl. Quit without saving?");
  });
});
