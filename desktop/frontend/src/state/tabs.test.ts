// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  Workspace: { Read: vi.fn(), Save: vi.fn() },
}));
vi.mock("../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

const { useTabs, isDirty } = await import("./tabs");

beforeEach(() => {
  vi.clearAllMocks();
  useTabs.setState({ tabs: [], active: null, closed: [] });
});

describe("tabs and line breaks", () => {
  it("holds a CRLF file with \\n and saves it with \\r\\n again", async () => {
    api.Workspace.Read.mockResolvedValue({ path: "a.hurl", text: "GET x\r\nHTTP 200\r\n", hash: "h1" });
    api.Workspace.Save.mockResolvedValue("h2");
    await useTabs.getState().open("a.hurl");
    const tab = useTabs.getState().tabs[0];
    expect(tab.text).toBe("GET x\nHTTP 200\n");
    expect(isDirty(tab)).toBe(false);
    useTabs.getState().setText("a.hurl", "GET y\nHTTP 200\n");
    expect(await useTabs.getState().save("a.hurl")).toBe(true);
    expect(api.Workspace.Save).toHaveBeenCalledWith("a.hurl", "GET y\r\nHTTP 200\r\n", "h1");
    expect(isDirty(useTabs.getState().tabs[0])).toBe(false);
  });

  it("leaves an LF file as it is", async () => {
    api.Workspace.Read.mockResolvedValue({ path: "b.hurl", text: "GET x\nHTTP 200\n", hash: "h1" });
    api.Workspace.Save.mockResolvedValue("h2");
    await useTabs.getState().open("b.hurl");
    await useTabs.getState().save("b.hurl");
    expect(api.Workspace.Save).toHaveBeenCalledWith("b.hurl", "GET x\nHTTP 200\n", "h1");
  });
});

describe("closing several tabs", () => {
  const tab = (path: string) => ({ path, text: "", savedText: "", hash: "h", version: 1, conflict: false });
  const paths = () => useTabs.getState().tabs.map((t) => t.path);

  it("activates the nearest kept tab to the right, else to the left", () => {
    useTabs.setState({ tabs: ["a", "b", "c", "d"].map(tab), active: "b" });
    useTabs.getState().closeMany(["b", "c"]);
    expect(paths()).toEqual(["a", "d"]);
    expect(useTabs.getState().active).toBe("d");
    useTabs.getState().closeMany(["d"]);
    expect(useTabs.getState().active).toBe("a");
    useTabs.getState().closeMany(["a"]);
    expect(useTabs.getState().active).toBeNull();
  });

  it("keeps the active tab when it stays open", () => {
    useTabs.setState({ tabs: ["a", "b", "c"].map(tab), active: "b" });
    useTabs.getState().closeMany(["a", "c"]);
    expect(paths()).toEqual(["b"]);
    expect(useTabs.getState().active).toBe("b");
  });
});

describe("reopening, switching and pinning tabs", () => {
  const tab = (path: string, pinned = false) => ({ path, text: "", savedText: "", hash: "h", version: 1, conflict: false, pinned });
  const paths = () => useTabs.getState().tabs.map((t) => t.path);

  it("reopens the newest closed tab, from disk, skipping one deleted since", async () => {
    useTabs.setState({ tabs: ["a", "b", "c"].map((p) => tab(p)), active: "a" });
    useTabs.getState().closeMany(["b"]);
    useTabs.getState().closeMany(["c"]);
    expect(useTabs.getState().closed).toEqual(["b", "c"]);
    api.Workspace.Read.mockImplementation(async (p: string) => {
      if (p === "c") throw new Error("not found");
      return { path: p, text: "GET x\n", hash: "h" };
    });
    expect(await useTabs.getState().reopenClosed()).toBe(true);
    expect(paths()).toEqual(["a", "b"]);
    expect(useTabs.getState().active).toBe("b");
    expect(useTabs.getState().closed).toEqual([]);
    expect(await useTabs.getState().reopenClosed()).toBe(false);
  });

  it("switches to the next and previous tab, wrapping", () => {
    useTabs.setState({ tabs: ["a", "b", "c"].map((p) => tab(p)), active: "c" });
    useTabs.getState().cycle(1);
    expect(useTabs.getState().active).toBe("a");
    useTabs.getState().cycle(-1);
    expect(useTabs.getState().active).toBe("c");
  });

  it("keeps pinned tabs at the front", () => {
    useTabs.setState({ tabs: ["a", "b", "c"].map((p) => tab(p)), active: "a" });
    useTabs.getState().setPinned("c", true);
    useTabs.getState().setPinned("b", true);
    expect(paths()).toEqual(["c", "b", "a"]);
    useTabs.getState().setPinned("c", false);
    expect(paths()).toEqual(["b", "c", "a"]);
  });
});
