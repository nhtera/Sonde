// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { isServerMode } from "../transport/server-auth";

/**
 * Whether the page is served over HTTP (server mode, or the test
 * harness) rather than in the desktop window: the window-only features
 * (folder dialogs, reveal, trash, copy with secret values) are hidden.
 */
export const serverMode = isServerMode(window.location);

/** Whether the visual tests asked the harness to look like the window. */
function harnessWindowLook(): boolean {
  if (import.meta.env.MODE !== "harness") return false;
  try {
    return sessionStorage.getItem("sonde.windowLook") === "1";
  } catch {
    return false;
  }
}

/**
 * Whether the window's own controls show (traffic lights, the project
 * switcher, Reveal and Move to Trash, Select file…). In the window; in
 * the test harness's build only when its visual tests ask, to compare the
 * screens with the design (the controls do nothing there).
 */
export const windowLook = !serverMode || harnessWindowLook();

/** The platform, for key labels and the title bar. */
export const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
