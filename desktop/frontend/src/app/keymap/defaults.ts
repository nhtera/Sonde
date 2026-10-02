// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The approved shortcuts (tinykeys syntax; $mod is ⌘ on macOS, Ctrl
// elsewhere). Users rebind them in the shortcuts sheet (saved in settings).
export const defaultKeys: Record<string, string> = {
  "request.send": "$mod+Enter",
  "request.runTo": "$mod+Shift+Enter",
  "file.run": "$mod+KeyR",
  "response.save": "$mod+Alt+KeyS",
  "palette.open": "$mod+KeyK",
  "editor.toggleView": "$mod+KeyE",
  "tree.filter": "$mod+Shift+KeyF",
  "file.save": "$mod+KeyS",
  "editor.toggleComment": "$mod+Slash",
  "shortcuts.open": "Shift+Slash",
  // Owned by later features; bound once they register the command.
  "folder.open": "$mod+KeyO",
  "import.open": "$mod+KeyI",
  "editor.gotoLine": "$mod+KeyG",
  "copyas.curl.request": "$mod+Shift+KeyK",
  "copyas.sonde.file": "$mod+Alt+KeyK",
  "testrun.again": "$mod+Alt+KeyR",
};
