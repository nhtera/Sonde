// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Edits computed by Go (editsvc) for a tab's text at a version. An open
// editor takes the new text as one change of its own (so ⌘Z undoes it); a
// tab without one takes it as is. An edit for an older version is not
// applied: the caller asks again. The editor diffs the text rather than
// replaying Go's ranges: its line breaks are "\n" while the tab may still
// hold the file's "\r\n", so the offsets would not match.

import { useTabs } from "./tabs";

export interface TextEdit {
  Range: { Start: number; End: number };
  NewText: string;
}

export interface EditResult {
  version: number;
  edits: TextEdit[];
  text: string;
}

/** An editor showing a tab. */
export interface EditTarget {
  /** Replaces the text with text, as one change. */
  apply(text: string): void;
  /** Undoes the last change. */
  undo(): void;
}

const targets = new Map<string, EditTarget>();

/** Registers the editor of path; returns the unregister function. */
export function registerEditTarget(path: string, t: EditTarget): () => void {
  targets.set(path, t);
  return () => {
    if (targets.get(path) === t) targets.delete(path);
  };
}

/** Applies a result to path's tab; false when the tab changed since the
 * result was computed (or is closed). */
export function applyEdit(path: string, res: EditResult): boolean {
  const tab = useTabs.getState().tabs.find((t) => t.path === path);
  if (!tab || tab.version !== res.version) return false;
  const target = targets.get(path);
  if (target) target.apply(res.text);
  else useTabs.getState().setText(path, res.text);
  return true;
}

/** Whether path has an editor (Undo is offered only then). */
export const hasEditTarget = (path: string) => targets.has(path);

/** Undoes the last change of path's editor. */
export function undoEdit(path: string) {
  targets.get(path)?.undo();
}

/** The 1-based line of a UTF-16 offset of text. */
export function lineAt(text: string, offset: number): number {
  let line = 1;
  for (let i = 0; i < offset && i < text.length; i++) if (text.charCodeAt(i) === 10) line++;
  return line;
}
