// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  Workspace: { Tree: vi.fn(), Index: vi.fn(), Project: vi.fn() },
  WorkspaceDesktop: { OpenRecent: vi.fn(), Recent: vi.fn() },
  Git: { Info: vi.fn(), Status: vi.fn() },
}));
vi.mock("../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

const { useWorkspace } = await import("./workspace");

const oldProject = { name: "old", dir: "/p/old" };
const newProject = { name: "new", dir: "/p/new" };
const tree = (name: string) => ({ name: "", path: "", kind: "dir", children: [{ name, path: name, kind: "file", children: [] }] });

beforeEach(() => {
  vi.resetAllMocks();
  api.Workspace.Index.mockResolvedValue([]);
  api.WorkspaceDesktop.Recent.mockResolvedValue([]);
  api.Git.Info.mockResolvedValue({ trusted: false });
  useWorkspace.setState({ project: oldProject, tree: tree("old.http"), index: [], git: null });
});

describe("switching projects", () => {
  it("keeps the new project's tree when a refresh of the old one ends late", async () => {
    // A refresh of the old project is in flight (a change the watcher
    // saw, a save…) when another folder is opened.
    // The new project's refresh does not wait for it.
    const old = deferred();
    api.Workspace.Tree.mockReturnValueOnce(old.promise);
    const late = useWorkspace.getState().refresh();

    api.WorkspaceDesktop.OpenRecent.mockResolvedValueOnce(newProject);
    api.Workspace.Tree.mockResolvedValueOnce(tree("new.http"));
    await useWorkspace.getState().openRecent("new");
    expect(useWorkspace.getState().tree?.children?.[0]?.name).toBe("new.http");

    old.resolve(tree("old.http"));
    await late;
    expect(useWorkspace.getState().project).toEqual(newProject);
    expect(useWorkspace.getState().tree?.children?.[0]?.name).toBe("new.http");
  });

  it("shows no tree of the old project while the new one's is read", async () => {
    const next = deferred();
    api.WorkspaceDesktop.OpenRecent.mockResolvedValueOnce(newProject);
    api.Workspace.Tree.mockReturnValueOnce(next.promise);
    const opening = useWorkspace.getState().openRecent("new");
    await vi.waitFor(() => expect(useWorkspace.getState().project).toEqual(newProject));
    expect(useWorkspace.getState().tree).toBeNull();

    next.resolve(tree("new.http"));
    await opening;
    expect(useWorkspace.getState().tree?.children?.[0]?.name).toBe("new.http");
  });
});

describe("refresh", () => {
  it("runs one at a time, and the calls made meanwhile share the next", async () => {
    // Files changing faster than a refresh takes: each refresh is shown
    // (none is dropped for a later one), and they do not pile up.
    const first = deferred();
    const second = deferred();
    api.Workspace.Tree.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const a = useWorkspace.getState().refresh();
    const b = useWorkspace.getState().refresh();
    const c = useWorkspace.getState().refresh();
    expect(b).toBe(c);
    expect(api.Workspace.Tree).toHaveBeenCalledTimes(1);

    first.resolve(tree("first.http"));
    await a;
    expect(useWorkspace.getState().tree?.children?.[0]?.name).toBe("first.http");

    await vi.waitFor(() => expect(api.Workspace.Tree).toHaveBeenCalledTimes(2));
    second.resolve(tree("second.http"));
    await b;
    expect(useWorkspace.getState().tree?.children?.[0]?.name).toBe("second.http");
  });

  it("runs the next one when the running one fails", async () => {
    api.Workspace.Tree.mockRejectedValueOnce(new Error("denied")).mockResolvedValueOnce(tree("after.http"));
    const a = useWorkspace.getState().refresh();
    const b = useWorkspace.getState().refresh();
    await expect(a).rejects.toThrow("denied");
    await b;
    expect(useWorkspace.getState().tree?.children?.[0]?.name).toBe("after.http");
  });
});

function deferred() {
  let resolve!: (v: unknown) => void;
  const promise = new Promise((r) => (resolve = r));
  return { promise, resolve };
}
