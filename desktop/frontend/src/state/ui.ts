// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { create } from "zustand";

export interface Toast {
  id: number;
  kind: "info" | "success" | "warn" | "error";
  text: string;
  action?: { label: string; run: () => void };
}

interface UIState {
  /** The side panel shown next to the rail (a registry panel id), or null. */
  panel: string | null;
  /** The panel last shown: the rail marks it while the side is closed. */
  lastPanel: string;
  paletteOpen: boolean;
  /** Text to prefill the palette with (">" for commands only). */
  paletteQuery: string;
  shortcutsOpen: boolean;
  /** Narrow window (1024px and below): the results pane is toggled. */
  narrow: boolean;
  resultsOpen: boolean;
  treeFilter: string;
  /** The editor view of the main area (a registry editor id). */
  editorView: string | null;
  /** The cursor of the active editor (1-based), for the status bar. */
  cursor: { line: number; col: number } | null;
  toasts: Toast[];
  setPanel(id: string | null): void;
  togglePanel(id: string): void;
  openPalette(query?: string): void;
  closePalette(): void;
  setShortcutsOpen(open: boolean): void;
  setNarrow(narrow: boolean): void;
  setResultsOpen(open: boolean): void;
  setTreeFilter(q: string): void;
  setEditorView(id: string | null): void;
  toast(t: Omit<Toast, "id">): void;
  dismiss(id: number): void;
}

let nextToast = 0;

export const useUI = create<UIState>((set, get) => ({
  panel: "files",
  lastPanel: "files",
  paletteOpen: false,
  paletteQuery: "",
  shortcutsOpen: false,
  narrow: false,
  resultsOpen: true,
  treeFilter: "",
  editorView: null,
  cursor: null,
  toasts: [],
  setPanel: (panel) => set(panel ? { panel, lastPanel: panel } : { panel }),
  togglePanel: (id) => set(get().panel === id ? { panel: null } : { panel: id, lastPanel: id }),
  openPalette: (query = "") => set({ paletteOpen: true, paletteQuery: query }),
  closePalette: () => set({ paletteOpen: false }),
  setShortcutsOpen: (shortcutsOpen) => set({ shortcutsOpen }),
  setNarrow: (narrow) => set({ narrow }),
  setResultsOpen: (resultsOpen) => set({ resultsOpen }),
  setTreeFilter: (treeFilter) => set({ treeFilter }),
  setEditorView: (editorView) => set({ editorView }),
  toast: (t) => {
    const id = ++nextToast;
    set({ toasts: [...get().toasts, { ...t, id }] });
    setTimeout(() => get().dismiss(id), t.action ? 8000 : 4000);
  },
  dismiss: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
}));
