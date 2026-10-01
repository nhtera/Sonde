// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Features plug into the shell here, from their own index.ts: commands
// (palette and keymap), rail panels, toolbar items, and the editor and
// results hosts. The shell never imports a feature, so features built in
// parallel never edit the shell.

import type { FileRun } from "../state/run-model";
import type { ComponentType } from "react";
import { useSyncExternalStore } from "react";

export interface Command {
  id: string;
  title: string;
  /** Muted words after the title in the palette (e.g. "sonde check"). */
  hint?: string;
  group?: string;
  /** Default keys (tinykeys syntax, e.g. "$mod+KeyR"). */
  keys?: string;
  /** Hidden in the palette (still bound to keys). */
  hidden?: boolean;
  when?(): boolean;
  /** arg is passed by callers that have one (a folder to run…). */
  run(arg?: unknown): void | Promise<void>;
}

export interface Panel {
  id: string;
  title: string;
  icon: ComponentType<{ size?: number }>;
  order: number;
  render: ComponentType;
  /** Shown in the main area, in place of the tabs and editor, while the
   * panel is open (the Test run report, the environment editor). */
  main?: ComponentType;
  /** The main view takes the results' column too (Settings). */
  wide?: boolean;
  /** Hidden in server mode. */
  windowOnly?: boolean;
}

/** A piece a feature adds to another's view (the git Changes card under
 * the file tree). */
export interface Slot {
  id: string;
  order: number;
  render: ComponentType;
}

export interface ToolbarItem {
  id: string;
  order: number;
  render: ComponentType<{ file: string }>;
}

/** What the main area shows for a file: an editor view (Text, Form). */
export interface EditorView {
  id: string;
  title: string;
  order: number;
  render: ComponentType<{ file: string }>;
}

export interface ResultsView {
  /** run, when given, is shown read-only instead of the file's last run
   * (a file of a test run). */
  render: ComponentType<{ file: string; run?: FileRun }>;
}

const commands = new Map<string, Command>();
const panels = new Map<string, Panel>();
const toolbar = new Map<string, ToolbarItem>();
const editors = new Map<string, EditorView>();
let results: ResultsView | null = null;
const slots = new Map<string, Map<string, Slot>>();
let version = 0;
const listeners = new Set<() => void>();

function changed() {
  version++;
  listeners.forEach((l) => l());
}

export const registry = {
  command(c: Command) {
    commands.set(c.id, c);
    changed();
  },
  panel(p: Panel) {
    panels.set(p.id, p);
    changed();
  },
  toolbarItem(t: ToolbarItem) {
    toolbar.set(t.id, t);
    changed();
  },
  editor(e: EditorView) {
    editors.set(e.id, e);
    changed();
  },
  results(r: ResultsView) {
    results = r;
    changed();
  },
  /** Adds s to the slot named name. */
  slot(name: string, s: Slot) {
    const m = slots.get(name) ?? new Map<string, Slot>();
    m.set(s.id, s);
    slots.set(name, m);
    changed();
  },
  getSlots: (name: string) => [...(slots.get(name)?.values() ?? [])].sort((a, b) => a.order - b.order),
  commands: () => [...commands.values()],
  getCommand: (id: string) => commands.get(id),
  panels: () => [...panels.values()].sort((a, b) => a.order - b.order),
  getPanel: (id: string) => panels.get(id),
  toolbarItems: () => [...toolbar.values()].sort((a, b) => a.order - b.order),
  editors: () => [...editors.values()].sort((a, b) => a.order - b.order),
  getResults: () => results,
};

/** Re-renders when features register. */
export function useRegistry(): number {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => version,
  );
}
