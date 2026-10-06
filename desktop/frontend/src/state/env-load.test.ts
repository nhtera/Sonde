// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  Envs: { List: vi.fn(), Overrides: vi.fn(() => Promise.resolve({ count: 0, items: [] })) },
}));
vi.mock("../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));
const handlers = vi.hoisted(() => new Map<string, (data: unknown) => void>());
vi.mock("../lib/events", () => ({ on: (topic: string, h: (data: unknown) => void) => (handlers.set(topic, h), () => handlers.delete(topic)) }));

const { useEnv } = await import("./env");

beforeEach(() => useEnv.setState({ project: null, current: "" }));

describe("useEnv.load", () => {
  it("has no environments, and no error, with no folder open", async () => {
    api.Envs.List.mockRejectedValueOnce(new Error(JSON.stringify({ code: "not-found", message: "no project is open" })));
    await expect(useEnv.getState().load()).resolves.toBeUndefined();
    expect(useEnv.getState().project).toBeNull();
  });

  it("still fails on any other error", async () => {
    api.Envs.List.mockRejectedValueOnce(new Error(JSON.stringify({ code: "invalid", message: "sonde.yaml: bad" })));
    await expect(useEnv.getState().load()).rejects.toThrow("sonde.yaml: bad");
  });
});

describe("switching projects", () => {
  it("keeps the new project's environments when the old one's load ends late", async () => {
    let finishOld!: (p: unknown) => void;
    api.Envs.List.mockReturnValueOnce(new Promise((r) => (finishOld = r)));
    const late = useEnv.getState().load();

    api.Envs.List.mockResolvedValueOnce({ envs: [{ name: "new-env" }] });
    handlers.get("ws:opened")?.({ name: "new", dir: "/p/new" });
    expect(useEnv.getState().project).toBeNull();
    await vi.waitFor(() => expect(useEnv.getState().current).toBe("new-env"));

    finishOld({ envs: [{ name: "old-env" }] });
    await late;
    expect(useEnv.getState().project?.envs?.[0].name).toBe("new-env");
    expect(useEnv.getState().current).toBe("new-env");
  });
});
