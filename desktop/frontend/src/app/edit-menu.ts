// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Edit › Undo and Redo in the window app's menu (app:edit): the focused
// code editor undoes in its own history; a form field, in the webview's.

import { redo, undo } from "@codemirror/commands";
import { EditorView } from "@codemirror/view";
import { on } from "../lib/events";

/** Undoes or redoes in the focused field. */
export function editCommand(kind: unknown) {
  if (kind !== "undo" && kind !== "redo") return;
  const el = document.activeElement;
  const view = el instanceof HTMLElement ? EditorView.findFromDOM(el) : null;
  if (view) (kind === "undo" ? undo : redo)(view);
  else document.execCommand(kind);
}

/** Listens for the menu's Undo and Redo; returns the cleanup. */
export function startEditMenu(): () => void {
  return on("app:edit", editCommand);
}
