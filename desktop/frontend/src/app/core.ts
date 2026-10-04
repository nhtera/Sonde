// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The shell's own commands and the Files panel. Features register theirs
// from their index.ts the same way.

import { FilesIcon } from "../components/icons";
import { closeTabs } from "../features/shell/tabs-bar";
import { FileTree } from "../features/tree/file-tree";
import { appError } from "../lib/api";
import { windowLook } from "../lib/mode";
import { useRuns } from "../state/run";
import { useSettings } from "../state/settings";
import { isDirty, useTabs } from "../state/tabs";
import { useUI } from "../state/ui";
import { useWorkspace } from "../state/workspace";
import { registry } from "./registry";
import { isRequestPath } from "../lib/files";

const active = () => useTabs.getState().active;

registry.panel({ id: "files", title: "Files", icon: FilesIcon, order: 0, render: FileTree });

registry.command({
  id: "file.run",
  title: "Run file",
  hint: "every request, in order",
  when: () => {
    const f = active();
    return isRequestPath(f) && !useRuns.getState().runs[f!]?.running;
  },
  run: () => {
    const f = active();
    if (f) return useRuns.getState().run(f);
  },
});

// A run stops between requests, or mid-body: what came stays, as Canceled.
registry.command({
  id: "file.stop",
  title: "Stop the run",
  when: () => !!useRuns.getState().runs[active() ?? ""]?.running,
  run: () => {
    const f = active();
    if (f) useRuns.getState().cancel(f);
  },
});

registry.command({
  id: "file.save",
  title: "Save file",
  when: () => {
    const t = useTabs.getState().tabs.find((x) => x.path === active());
    return !!t && isDirty(t);
  },
  run: async () => {
    const f = active();
    if (f) await useTabs.getState().save(f);
  },
});

// Tabs: the tab menu passes its tab, the palette and the keys act on the
// active one. Their keys are the window's (a browser keeps ⌘W, ⇧⌘T and
// ⌃Tab). Closing several tabs leaves the pinned ones open.
const tabPaths = () => useTabs.getState().tabs.map((t) => t.path);
const unpinned = (paths: string[]) => paths.filter((p) => !useTabs.getState().tabs.find((t) => t.path === p)?.pinned);
const target = (arg?: unknown) => (typeof arg === "string" ? arg : active());
const closing = (id: string, title: string, pick: (paths: string[], at: number) => string[], opts: { force?: boolean; keys?: string; when?: () => boolean } = {}) =>
  registry.command({
    id,
    title,
    group: "Tabs",
    keys: windowLook ? opts.keys : undefined,
    when: opts.when ?? (() => tabPaths().length > 0),
    run: (arg) => {
      const paths = tabPaths();
      const at = paths.indexOf(target(arg) ?? "");
      return closeTabs(pick(paths, at), opts.force);
    },
  });
const dirtyTab = () => useTabs.getState().tabs.some((t) => t.path === active() && isDirty(t));
closing("tab.close", "Close tab", (p, at) => (at < 0 ? [] : [p[at]]), { keys: "$mod+KeyW", when: () => !!active() });
closing("tab.closeWithoutSaving", "Close tab without saving", (p, at) => (at < 0 ? [] : [p[at]]), { force: true, keys: "$mod+Alt+KeyW", when: dirtyTab });
closing("tab.closeOthers", "Close other tabs", (p, at) => unpinned(p.filter((_, i) => i !== at)), { when: () => tabPaths().length > 1 });
closing("tab.closeRight", "Close tabs to the right", (p, at) => (at < 0 ? [] : unpinned(p.slice(at + 1))));
closing("tab.closeSaved", "Close saved tabs", () => unpinned(useTabs.getState().tabs.filter((t) => !isDirty(t)).map((t) => t.path)));
closing("tab.closeAll", "Close all tabs", unpinned);
closing("tab.closeAllWithoutSaving", "Close all tabs without saving", unpinned, { force: true });

registry.command({
  id: "tab.reopenClosed",
  title: "Reopen closed tab",
  group: "Tabs",
  keys: windowLook ? "$mod+Shift+KeyT" : undefined,
  when: () => useTabs.getState().closed.some((p) => !tabPaths().includes(p)),
  run: async () => {
    if (!(await useTabs.getState().reopenClosed())) useUI.getState().toast({ kind: "info", text: "No closed tab to reopen" });
  },
});
registry.command({ id: "tab.next", title: "Next tab", group: "Tabs", keys: windowLook ? "Control+Tab" : undefined, when: () => tabPaths().length > 1, run: () => useTabs.getState().cycle(1) });
registry.command({ id: "tab.previous", title: "Previous tab", group: "Tabs", keys: windowLook ? "Control+Shift+Tab" : undefined, when: () => tabPaths().length > 1, run: () => useTabs.getState().cycle(-1) });
const pinnedTab = (arg?: unknown) => useTabs.getState().tabs.find((t) => t.path === target(arg))?.pinned;
registry.command({ id: "tab.pin", title: "Pin tab", group: "Tabs", when: () => !!active() && !pinnedTab(), run: (arg) => useTabs.getState().setPinned(target(arg) ?? "", true) });
registry.command({ id: "tab.unpin", title: "Unpin tab", group: "Tabs", when: () => !!pinnedTab(), run: (arg) => useTabs.getState().setPinned(target(arg) ?? "", false) });

registry.command({
  id: "palette.open",
  title: "Search files and commands",
  hidden: true,
  run: () => {
    const ui = useUI.getState();
    if (ui.paletteOpen) ui.closePalette();
    else ui.openPalette();
  },
});

registry.command({
  id: "tree.filter",
  title: "Filter requests in all files",
  hint: "method, path, headers, body",
  when: () => !!useWorkspace.getState().project,
  run: () => {
    useUI.getState().setPanel("files");
    requestAnimationFrame(() => document.getElementById("tree-filter")?.focus());
  },
});

registry.command({ id: "shortcuts.open", title: "Keyboard shortcuts", run: () => useUI.getState().setShortcutsOpen(true) });

registry.command({
  id: "editor.toggleView",
  title: "Switch Text / Form",
  when: () => !!active() && registry.editors().length > 1,
  run: () => {
    const editors = registry.editors();
    const ui = useUI.getState();
    const i = editors.findIndex((e) => e.id === (ui.editorView ?? editors[0]?.id));
    ui.setEditorView(editors[(i + 1) % editors.length].id);
  },
});

// The window's own (the harness shows it for the design screens).
if (windowLook) {
  registry.command({ id: "folder.open", title: "Open a folder…", run: () => useWorkspace.getState().openFolder() });
  registry.command({ id: "project.openExample", title: "Try the example project", run: () => useWorkspace.getState().openExample() });
}

registry.command({ id: "theme.select", title: "Select theme…", run: () => useUI.getState().setThemePickerOpen(true) });

registry.command({
  id: "theme.toggle",
  title: "Toggle day / night theme",
  run: () =>
    useSettings
      .getState()
      .toggleTheme()
      .catch((err) => useUI.getState().toast({ kind: "error", text: appError(err).message })),
});
