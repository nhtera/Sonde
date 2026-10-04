// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Open files. A tab's text is the truth for runs, saved or not; Save sends
// the hash of the text it replaces and is refused when the file changed on
// disk meanwhile. A tab's line breaks are "\n", as the editor's (offsets
// from Go and the editor then agree); a file with "\r\n" is saved with
// them again.

import { create } from "zustand";
import { Workspace, appError } from "../lib/api";
import { on } from "../lib/events";
import { flushEdits } from "./edits";
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
  /** The file's line break, restored on save. */
  eol?: "\r\n";
  /** Kept at the front; bulk closes leave it open. */
  pinned?: boolean;
}

interface TabsState {
  tabs: Tab[];
  active: string | null;
  /** Paths of the tabs closed, newest last (Reopen closed tab). */
  closed: string[];
  open(path: string): Promise<void>;
  activate(path: string): void;
  setText(path: string, text: string): void;
  save(path: string): Promise<boolean>;
  close(path: string): void;
  /** Closes several tabs at once; the active one stays if it is kept,
   * else the nearest kept tab to its right, else to its left. */
  closeMany(paths: string[]): void;
  /** Opens the newest closed tab that is not open and still exists, read
   * from disk; false when there is none. */
  reopenClosed(): Promise<boolean>;
  /** The tab next to the active one, wrapping (step 1 or -1). */
  cycle(step: 1 | -1): void;
  /** Pins (to the end of the pinned tabs) or unpins (to the start of the
   * others) a tab. */
  setPinned(path: string, pinned: boolean): void;
  reload(path: string): Promise<void>;
}

export const isDirty = (t: Tab) => t.text !== t.savedText;

/** How many closed tabs Reopen closed tab remembers. */
const maxClosed = 20;

/** A file's text with "\n" line breaks, and whether it had "\r\n". */
function fromDisk(text: string): { text: string; eol?: "\r\n" } {
  return text.includes("\r\n") ? { text: text.replace(/\r\n/g, "\n"), eol: "\r\n" } : { text };
}

export const useTabs = create<TabsState>((set, get) => ({
  tabs: [],
  active: null,
  closed: [],
  open: async (path) => {
    if (get().tabs.some((t) => t.path === path)) {
      set({ active: path });
      return;
    }
    const f = await Workspace.Read(path);
    if (!f) return;
    const { text, eol } = fromDisk(f.text);
    const tab: Tab = { path: f.path, text, savedText: text, hash: f.hash, version: 1, conflict: false, eol };
    set({ tabs: [...get().tabs, tab], active: f.path });
  },
  activate: (active) => set({ active }),
  setText: (path, text) =>
    set({ tabs: get().tabs.map((t) => (t.path === path ? { ...t, text, version: t.version + 1 } : t)) }),
  save: async (path) => {
    await flushEdits(path);
    const tab = get().tabs.find((t) => t.path === path);
    if (!tab) return false;
    try {
      const hash = await Workspace.Save(path, tab.eol ? tab.text.replace(/\n/g, tab.eol) : tab.text, tab.hash);
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
  closeMany: (paths) => {
    const gone = new Set(paths);
    const { tabs, active } = get();
    const kept = tabs.filter((t) => !gone.has(t.path));
    let next = active;
    if (active === null || gone.has(active)) {
      const at = Math.max(tabs.findIndex((t) => t.path === active), 0);
      const right = tabs.slice(at + 1).find((t) => !gone.has(t.path));
      const left = tabs.slice(0, at).reverse().find((t) => !gone.has(t.path));
      next = (right ?? left)?.path ?? null;
    }
    const closedNow = tabs.filter((t) => gone.has(t.path)).map((t) => t.path);
    const closed = [...get().closed.filter((p) => !gone.has(p)), ...closedNow].slice(-maxClosed);
    set({ tabs: kept, active: next, closed });
  },
  reopenClosed: async () => {
    for (;;) {
      const path = [...get().closed].reverse().find((p) => !get().tabs.some((t) => t.path === p));
      if (path === undefined) return false;
      set({ closed: get().closed.filter((p) => p !== path) });
      try {
        await get().open(path);
        if (get().tabs.some((t) => t.path === path)) return true;
      } catch {
        // Moved or deleted since: try the one closed before it.
      }
    }
  },
  cycle: (step) => {
    const { tabs, active } = get();
    if (tabs.length < 2) return;
    const at = tabs.findIndex((t) => t.path === active);
    set({ active: tabs[(at + step + tabs.length) % tabs.length].path });
  },
  setPinned: (path, pinned) => {
    const tab = get().tabs.find((t) => t.path === path);
    if (!tab || !!tab.pinned === pinned) return;
    const rest = get().tabs.filter((t) => t.path !== path);
    const firstOther = rest.findIndex((t) => !t.pinned);
    const at = firstOther < 0 ? rest.length : firstOther;
    set({ tabs: [...rest.slice(0, at), { ...tab, pinned }, ...rest.slice(at)] });
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
    const { text, eol } = fromDisk(f.text);
    set({
      tabs: get().tabs.map((t) =>
        t.path === path ? { ...t, text, savedText: text, hash: f.hash, eol, version: t.version + 1, conflict: false } : t,
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
