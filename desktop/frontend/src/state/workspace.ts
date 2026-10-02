// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { create } from "zustand";
import { Git, Workspace, WorkspaceDesktop, type FileStatus, type GitInfo, type Node, type Project, type Recent, type Request } from "../lib/api";
import { on } from "../lib/events";
import { confirm } from "../components/ask";
import { harnessFixture, serverMode } from "../lib/mode";
import { useRuns } from "./run";
import { isDirty, useTabs } from "./tabs";
import { useUI } from "./ui";

interface WorkspaceState {
  project: Project | null;
  loaded: boolean;
  tree: Node | null;
  index: Request[];
  recent: Recent[];
  git: GitInfo | null;
  /** Changed files by project path (A, M, …), trusted folders only. */
  gitStatus: Record<string, string>;
  /** Each changed file's lines added and removed (-1: unknown). */
  gitLines: Record<string, [number, number]>;
  /** The changed files that hold secrets: never committed. */
  gitSecrets: string[];
  load(): Promise<void>;
  refresh(): Promise<void>;
  openFolder(): Promise<void>;
  openRecent(id: string): Promise<void>;
  trust(): Promise<void>;
}

export const useWorkspace = create<WorkspaceState>((set, get) => ({
  project: null,
  loaded: false,
  tree: null,
  index: [],
  recent: [],
  git: null,
  gitStatus: {},
  gitLines: {},
  gitSecrets: [],
  load: async () => {
    // The visual tests show the welcome and the recent folders through
    // the harness, which always has a project and no window.
    const shown = harnessFixture<{ noProject?: boolean; recent?: Recent[] }>("workspace");
    const project = shown?.noProject ? null : await Workspace.Project();
    const recent = shown?.recent?.map((r) => (r.dir === "@project" && project ? { ...r, dir: project.dir } : r)) ?? (serverMode ? [] : ((await WorkspaceDesktop.Recent()) ?? []));
    set({ project, recent, loaded: true });
    if (project) await get().refresh();
  },
  refresh: async () => {
    const [tree, index, git] = await Promise.all([Workspace.Tree(), Workspace.Index(), Git.Info()]);
    let gitStatus: Record<string, string> = {};
    let gitSecrets: string[] = [];
    const gitLines: Record<string, [number, number]> = {};
    if (git?.trusted) {
      try {
        const list = (await Git.Status()) ?? [];
        gitStatus = statusMap(list);
        gitSecrets = list.filter((f) => f.secret).map((f) => f.path);
        for (const f of list) gitLines[f.path] = [f.added, f.removed];
      } catch {
        // git unavailable: no badges
      }
    }
    set({ tree, index: index ?? [], git, gitStatus, gitSecrets, gitLines });
  },
  openFolder: async () => {
    if (!(await discardEdits())) return;
    const project = await WorkspaceDesktop.OpenFolder();
    if (project) await afterOpen(project);
  },
  openRecent: async (id) => {
    if (!(await discardEdits())) return;
    const project = await WorkspaceDesktop.OpenRecent(id);
    if (project) await afterOpen(project);
  },
  trust: async () => {
    await Git.Trust();
    await get().refresh();
  },
}));

/** Whether switching projects may drop the unsaved tabs (asks if any). */
async function discardEdits(): Promise<boolean> {
  const dirty = useTabs.getState().tabs.filter(isDirty);
  if (dirty.length === 0) return true;
  return confirm({
    title: "Unsaved changes",
    message: `${dirty.length === 1 ? dirty[0].path + " has" : dirty.length + " files have"} unsaved changes. Open another folder without saving?`,
    submit: "Open without saving",
  });
}

/** Forgets what belonged to the previous project: its tabs (their paths
 * are relative to it), runs, filter and editor view. */
function forgetProject() {
  useTabs.setState({ tabs: [], active: null });
  useRuns.getState().reset();
  useUI.setState({ treeFilter: "", editorView: null });
}

async function afterOpen(project: Project) {
  forgetProject();
  useWorkspace.setState({ project, recent: (await WorkspaceDesktop.Recent()) ?? [] });
  await useWorkspace.getState().refresh();
}

function statusMap(list: FileStatus[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const s of list) {
    const code = s.index === "?" || s.worktree === "?" || s.index === "A" ? "A" : s.worktree || s.index;
    if (code) out[s.path] = code;
  }
  return out;
}

on("ws:opened", (data) => {
  // Opened here (afterOpen) or elsewhere: tabs of another project go.
  const dir = (data as Project | null)?.dir;
  if (dir !== useWorkspace.getState().project?.dir) forgetProject();
  void useWorkspace.getState().load();
});
let pending: ReturnType<typeof setTimeout> | undefined;
on("ws:changed", () => {
  clearTimeout(pending);
  pending = setTimeout(() => void useWorkspace.getState().refresh(), 150);
});
