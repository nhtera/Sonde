// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The shell's own commands and the Files panel. Features register theirs
// from their index.ts the same way.

import { FilesIcon } from "../components/icons";
import { FileTree } from "../features/tree/file-tree";
import { windowLook } from "../lib/mode";
import { useRuns } from "../state/run";
import { resolvedTheme, useSettings, type Theme } from "../state/settings";
import { isDirty, useTabs } from "../state/tabs";
import { useUI } from "../state/ui";
import { useWorkspace } from "../state/workspace";
import { registry } from "./registry";

const active = () => useTabs.getState().active;

registry.panel({ id: "files", title: "Files", icon: FilesIcon, order: 0, render: FileTree });

registry.command({
  id: "file.run",
  title: "Run file",
  hint: "every request, in order",
  when: () => {
    const f = active();
    return !!f && !useRuns.getState().runs[f]?.running;
  },
  run: () => {
    const f = active();
    if (f) return useRuns.getState().run(f);
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
}

registry.command({
  id: "theme.toggle",
  title: "Toggle light / dark theme",
  run: () => {
    const s = useSettings.getState();
    const theme = (s.value?.appearance.theme ?? "system") as Theme;
    return s.setTheme(resolvedTheme(theme) === "dark" ? "light" : "dark");
  },
});
