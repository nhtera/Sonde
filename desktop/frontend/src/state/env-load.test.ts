// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  Envs: { List: vi.fn(), Overrides: vi.fn(() => Promise.resolve({ count: 0, items: [] })) },
}));
vi.mock("../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

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
