// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Open files. A tab's text is the truth for runs, saved or not; Save sends
// the hash of the text it replaces and is refused when the file changed on
// disk meanwhile.

import { create } from "zustand";
import { Workspace, appError } from "../lib/api";
import { on } from "../lib/events";
import { useUI } from "./ui";

export interface Tab {
  path: string;
  text: string;
  /** The text on disk when last read or saved, and its hash. */
  savedText: string;
  hash: string;
  /** Bumped on every edit, for results computed against an older text. */
  version: number;
  /** The file changed on disk while the tab had unsaved edits. */
  conflict: boolean;
}

interface TabsState {
  tabs: Tab[];
  active: string | null;
  open(path: string): Promise<void>;
  activate(path: string): void;
  setText(path: string, text: string): void;
  save(path: string): Promise<boolean>;
  close(path: string): void;
  reload(path: string): Promise<void>;
}

export const isDirty = (t: Tab) => t.text !== t.savedText;

export const useTabs = create<TabsState>((set, get) => ({
  tabs: [],
  active: null,
  open: async (path) => {
    if (get().tabs.some((t) => t.path === path)) {
      set({ active: path });
      return;
    }
    const f = await Workspace.Read(path);
    if (!f) return;
    const tab: Tab = { path: f.path, text: f.text, savedText: f.text, hash: f.hash, version: 1, conflict: false };
    set({ tabs: [...get().tabs, tab], active: f.path });
  },
  activate: (active) => set({ active }),
  setText: (path, text) =>
    set({ tabs: get().tabs.map((t) => (t.path === path ? { ...t, text, version: t.version + 1 } : t)) }),
  save: async (path) => {
    const tab = get().tabs.find((t) => t.path === path);
    if (!tab) return false;
    try {
      const hash = await Workspace.Save(path, tab.text, tab.hash);
      set({ tabs: get().tabs.map((t) => (t.path === path ? { ...t, savedText: tab.text, hash, conflict: false } : t)) });
      return true;
    } catch (err) {
      const e = appError(err);
      useUI.getState().toast({
        kind: e.code === "conflict" ? "warn" : "error",
        text: e.code === "conflict" ? `${path} changed on disk` : e.message,
        action: e.code === "conflict" ? { label: "Reload", run: () => void get().reload(path) } : undefined,
      });
      return false;
    }
  },
  close: (path) => {
    const tabs = get().tabs.filter((t) => t.path !== path);
    const active = get().active === path ? (tabs.at(-1)?.path ?? null) : get().active;
    set({ tabs, active });
  },
  reload: async (path) => {
    const before = get().tabs.find((t) => t.path === path)?.version;
    const f = await Workspace.Read(path);
    if (!f) return;
    // An edit made while reading wins: the tab is then marked instead.
    const now = get().tabs.find((t) => t.path === path);
    if (now && now.version !== before) {
      set({ tabs: get().tabs.map((t) => (t.path === path ? { ...t, conflict: true } : t)) });
      return;
    }
    set({
      tabs: get().tabs.map((t) =>
        t.path === path ? { ...t, text: f.text, savedText: f.text, hash: f.hash, version: t.version + 1, conflict: false } : t,
      ),
    });
  },
}));

// A file changed outside the app: a clean tab reloads, a dirty one is
// marked (its next save is refused until reloaded).
on("ws:changed", (data) => {
  const paths = new Set((data as { paths: string[] }).paths);
  for (const t of useTabs.getState().tabs) {
    if (!paths.has(t.path)) continue;
    if (isDirty(t)) {
      useTabs.setState({ tabs: useTabs.getState().tabs.map((x) => (x.path === t.path ? { ...x, conflict: true } : x)) });
    } else {
      void useTabs.getState().reload(t.path);
    }
  }
});
