// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ScopeVar } from "../../lib/api";

const api = vi.hoisted(() => ({
  Vars: { For: vi.fn() },
}));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

const tabState = vi.hoisted(() => ({ tabs: [] as { path: string; text: string; version: number }[] }));
vi.mock("../../state/tabs", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useTabs: { getState: () => tabState },
}));

const runState = vi.hoisted(() => ({ runs: {} as Record<string, { entries: Record<number, { captures: { name: string; value: string }[] }> }> }));
vi.mock("../../state/run", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useRuns: { getState: () => runState },
}));

const envState = vi.hoisted(() => ({ current: "dev", project: "test", overrides: {} }));
vi.mock("../../state/env", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useEnv: { getState: () => envState, subscribe: vi.fn(() => vi.fn()) },
}));

vi.mock("../../lib/events", () => ({
  on: vi.fn(),
}));

const entryAtMocks = vi.hoisted(() => ({
  lastParsed: vi.fn(() => null),
  modelOf: vi.fn(() => Promise.resolve(null)),
}));
vi.mock("./entry-at", async (importOriginal) => ({
  ...(await importOriginal<object>()), ...entryAtMocks
}));

const { variablesAt, captureSites, lineOfOffset, clearVarsCache } = await import("./variables");

describe("variables", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.Vars.For.mockResolvedValue([]);
    tabState.tabs = [];
    runState.runs = {};
    entryAtMocks.lastParsed.mockReturnValue(null);
    entryAtMocks.modelOf.mockResolvedValue(null);
    clearVarsCache();
  });

  describe("lineOfOffset", () => {
    it("returns 1 for offset 0", () => {
      expect(lineOfOffset("GET /api", 0)).toBe(1);
    });

    it("counts newlines correctly", () => {
      const text = "GET /api\nPOST /users\nGET /health";
      expect(lineOfOffset(text, 0)).toBe(1);
      expect(lineOfOffset(text, 9)).toBe(2);
      expect(lineOfOffset(text, 23)).toBe(3);
    });

    it("handles offset beyond text length", () => {
      expect(lineOfOffset("one\ntwo", 100)).toBe(2);
    });
  });

  describe("captureSites", () => {
    it("returns empty map when model is empty", () => {
      const sites = captureSites([], "GET /api");
      expect(sites.size).toBe(0);
    });

    it("finds captures from all entries", () => {
      const model = [
        { Index: 1, Rows: { captures: [{ Key: "token", Range: { Start: 5, End: 10 } }] } } as never,
        { Index: 2, Rows: { captures: [{ Key: "user_id", Range: { Start: 15, End: 22 } }] } } as never,
      ];
      const sites = captureSites(model, "GET /api\nPOST /users");

      expect(sites.get("token")).toEqual({ entry: 1, line: 1 });
      expect(sites.get("user_id")).toEqual({ entry: 2, line: 2 });
    });

    it("uses the last capture before offset", () => {
      const model = [
        { Index: 1, Rows: { captures: [{ Key: "x", Range: { Start: 5, End: 6 } }] } } as never,
        { Index: 2, Rows: { captures: [{ Key: "x", Range: { Start: 20, End: 21 } }] } } as never,
      ];
      const sites = captureSites(model, "GET /api\nPOST /users", 15);

      expect(sites.get("x")).toEqual({ entry: 1, line: 1 });
    });
  });

  describe("variablesAt", () => {
    it("orders captures before overrides before project vars", async () => {
      api.Vars.For.mockResolvedValue([
        { name: "base_url", source: "project", display: "http://localhost", secret: false, origin: "sonde.yaml" } as ScopeVar,
        { name: "user", source: "override", display: "alice", secret: false } as ScopeVar,
      ] as ScopeVar[]);
      runState.runs = {
        "test.hurl": {
          entries: {
            1: { captures: [{ name: "token", value: "abc123" }] },
          },
        },
      };
      tabState.tabs = [{ path: "test.hurl", text: "GET /api", version: 1 }];
      entryAtMocks.modelOf.mockResolvedValue([
        { Index: 1, Rows: { captures: [{ Key: "token", Range: { Start: 0, End: 5 } }] } },
      ] as never);

      const vars = await variablesAt("test.hurl");

      const names = vars.map((v) => v.name);
      const captureIdx = names.indexOf("token");
      const overrideIdx = names.indexOf("user");
      const projectIdx = names.indexOf("base_url");

      expect(captureIdx).toBeGreaterThanOrEqual(0);
      expect(captureIdx).toBeLessThan(overrideIdx);
      expect(overrideIdx).toBeLessThan(projectIdx);
    });

    it("marks secrets as ***", async () => {
      api.Vars.For.mockResolvedValue([
        // Go sends no value for a secret: the page shows ***.
        { name: "password", source: "project", display: "", secret: true, origin: "secrets/local.secrets" } as ScopeVar,
      ] as ScopeVar[]);

      const vars = await variablesAt("test.hurl");
      const secret = vars.find((v) => v.name === "password");

      expect(secret?.value).toBe("***");
      expect(secret?.kind).toBe("secret");
      expect(secret?.source).toBe("secrets/local.secrets");
    });

    it("marks unran captures with value empty and captured false", async () => {
      api.Vars.For.mockResolvedValue([]);
      tabState.tabs = [{ path: "test.hurl", text: "GET /api", version: 1 }];
      entryAtMocks.modelOf.mockResolvedValue([
        { Index: 1, Rows: { captures: [{ Key: "token", Range: { Start: 0, End: 5 } }] } },
      ] as never);
      runState.runs = {
        "test.hurl": { entries: { 1: { captures: [] } } },
      };

      const vars = await variablesAt("test.hurl");
      const unran = vars.find((v) => v.name === "token");

      expect(unran?.value).toBe("");
      expect(unran?.captured).toBe(false);
    });

    it("marks run captures with captured value and captured true", async () => {
      api.Vars.For.mockResolvedValue([]);
      tabState.tabs = [{ path: "test.hurl", text: "GET /api", version: 1 }];
      entryAtMocks.modelOf.mockResolvedValue([
        { Index: 1, Rows: { captures: [{ Key: "token", Range: { Start: 0, End: 5 } }] } },
      ] as never);
      runState.runs = {
        "test.hurl": {
          entries: { 1: { captures: [{ name: "token", value: "abc123" }] } },
        },
      };

      const vars = await variablesAt("test.hurl");
      const ran = vars.find((v) => v.name === "token");

      expect(ran?.value).toBe("abc123");
      expect(ran?.captured).toBe(true);
    });
  });
});
