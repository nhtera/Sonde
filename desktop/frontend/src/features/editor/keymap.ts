// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The editor's own keys. The app's shortcuts win over them in the editor
// (ownKeys below), whatever they are rebound to.

import { defaultKeymap, historyKeymap, indentWithTab } from "@codemirror/commands";
import { searchKeymap } from "@codemirror/search";
import { completionKeymap, closeBracketsKeymap } from "@codemirror/autocomplete";
import { foldKeymap } from "@codemirror/language";
import { lintKeymap } from "@codemirror/lint";
import { Prec } from "@codemirror/state";
import { EditorView, type KeyBinding } from "@codemirror/view";
import { appOwnsKey } from "../../app/keymap/keymap-manager";

export const editorKeymap: KeyBinding[] = [
  ...closeBracketsKeymap,
  ...defaultKeymap,
  ...searchKeymap,
  ...historyKeymap,
  ...foldKeymap,
  ...completionKeymap,
  ...lintKeymap,
  indentWithTab,
];

/** An app shortcut pressed in the editor is left to the app: the editor's
 * keymaps do not see it (the app's handler runs as the event bubbles). */
export const ownKeys = Prec.highest(EditorView.domEventHandlers({ keydown: (e) => appOwnsKey(e) }));
