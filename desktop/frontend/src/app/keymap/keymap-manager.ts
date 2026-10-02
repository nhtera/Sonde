// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { matchKeybindingPress, parseKeybinding, tinykeys } from "tinykeys";
import { isMac } from "../../lib/mode";
import { registry } from "../registry";
import { defaultKeys } from "./defaults";

/** The keys of every command: the user's, else the defaults ("" unbinds). */
export function effectiveKeys(user: Record<string, string | undefined> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const c of registry.commands()) {
    const k = user?.[c.id] ?? c.keys ?? defaultKeys[c.id];
    if (k) out[c.id] = k;
  }
  return out;
}

/** Commands sharing the same keys, by keys. */
export function conflicts(keys: Record<string, string>): Record<string, string[]> {
  const byKeys: Record<string, string[]> = {};
  for (const [id, k] of Object.entries(keys)) (byKeys[normalize(k)] ??= []).push(id);
  return Object.fromEntries(Object.entries(byKeys).filter(([, ids]) => ids.length > 1));
}

/** Canonical form of a key combination (modifier order, $mod resolved). */
export function normalize(keys: string): string {
  const parts = keys.split("+");
  const key = parts.pop() ?? "";
  const mods = parts.map((m) => (m === "$mod" ? (isMac ? "Meta" : "Control") : m)).sort();
  return [...mods, key].join("+");
}

/** The keys bound now (the app's own, with a modifier), parsed. */
let bound: ReturnType<typeof parseKeybinding>[number][] = [];

/** Whether a keydown is one of the app's shortcuts: an editor inside the
 * page leaves it to the app (⌘↵ sends, ⌘G goes to a line…). */
export function appOwnsKey(e: KeyboardEvent): boolean {
  return bound.some((b) => matchKeybindingPress(e, b));
}

/** Binds every command's keys on target; returns the unbind function. */
export function bindKeys(target: Window | HTMLElement, keys: Record<string, string>): () => void {
  bound = Object.values(keys)
    .filter(hasModifier)
    .map((k) => parseKeybinding(k)[0]);
  const bindings: Record<string, (e: KeyboardEvent) => void> = {};
  for (const [id, k] of Object.entries(keys)) {
    bindings[k] = (e) => {
      // Text fields keep their own keys, except shortcuts with a modifier.
      const t = e.target as HTMLElement | null;
      if (!hasModifier(k) && t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
      // A bound key never reaches the browser (⌘R reload, ⌘S save page),
      // whether or not its command can run now.
      e.preventDefault();
      // Behind a dialog only the palette key works (it toggles the palette).
      if (id !== "palette.open" && document.querySelector('[role="dialog"][data-state="open"]')) return;
      const c = registry.getCommand(id);
      if (!c || (c.when && !c.when())) return;
      void c.run();
    };
  }
  // tinykeys skips keys typed in text fields by default; the rule above
  // decides instead, so ⌘↵ works in the editor. Held-down keys and IME
  // composition stay ignored.
  return tinykeys(target, bindings, { ignore: (e) => e.repeat || e.isComposing });
}

function hasModifier(keys: string): boolean {
  return /(^|\+)(\$mod|Alt|Control|Meta)\+/.test(keys);
}

/** Whether keys may be bound: with ⌘/Ctrl or ⌥, a function key, or ?.
 * A bare key (Tab, Enter, a letter) would be taken from the whole app. */
export function isBindable(keys: string): boolean {
  return hasModifier(keys) || /(^|\+)F([1-9]|1[0-2])$/.test(keys) || keys === "Shift+Slash";
}

/** A key combination as the user sees it: ⇧⌘F, Ctrl+Shift+F. */
export function label(keys: string): string {
  const parts = keys.split("+");
  const key = (parts.pop() ?? "").replace(/^Key/, "").replace(/^Digit/, "");
  const names: Record<string, string> = isMac
    ? { $mod: "⌘", Meta: "⌘", Control: "⌃", Alt: "⌥", Shift: "⇧" }
    : { $mod: "Ctrl", Meta: "Win", Control: "Ctrl", Alt: "Alt", Shift: "Shift" };
  const keyNames: Record<string, string> = { Enter: "↵", Slash: "/", Escape: "Esc", ArrowUp: "↑", ArrowDown: "↓", ...(isMac ? { Backspace: "⌫" } : {}) };
  const k = keyNames[key] ?? key;
  // macOS lists modifiers as ⌃⌥⇧⌘.
  const order = ["Control", "Alt", "Shift", "$mod", "Meta"];
  const mods = (isMac ? [...parts].sort((a, b) => order.indexOf(a) - order.indexOf(b)) : parts).map((p) => names[p] ?? p);
  return isMac ? [...mods, k].join("") : [...mods, k].join("+");
}

/** The tinykeys form of a keydown event, for rebinding. */
export function fromEvent(e: KeyboardEvent): string | null {
  if (["Meta", "Control", "Alt", "Shift"].includes(e.key)) return null;
  const mods: string[] = [];
  if (isMac ? e.metaKey : e.ctrlKey) mods.push("$mod");
  if (isMac && e.ctrlKey) mods.push("Control");
  if (e.altKey) mods.push("Alt");
  if (e.shiftKey) mods.push("Shift");
  return [...mods, e.code].join("+");
}
