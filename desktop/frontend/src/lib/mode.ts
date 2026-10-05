// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { isServerMode } from "../transport/server-auth";

/**
 * Whether the page is served over HTTP (server mode, or the test
 * harness) rather than in the desktop window: the window-only features
 * (folder dialogs, reveal, trash, copy with secret values) are hidden.
 */
export const serverMode = isServerMode(window.location);

// Marks the test harness's bundle: the release check refuses a binary
// that embeds it (scripts/check-no-harness.mjs looks for "e2eharness").
if (import.meta.env.MODE === "harness") document.documentElement.dataset.build = "e2eharness";

/** Whether the visual tests asked the harness to look like the window. */
function harnessWindowLook(): boolean {
  return String(harnessFixture<string | number>("windowLook")) === "1";
}

/**
 * A stand-in the visual tests give the test harness, under "sonde.<name>"
 * in sessionStorage (JSON, or a plain string): a state the harness cannot
 * reach otherwise (no project open, recent folders). Never read outside
 * the harness's build.
 */
export function harnessFixture<T>(name: string): T | undefined {
  if (import.meta.env.MODE !== "harness") return undefined;
  try {
    const raw = sessionStorage.getItem(`sonde.${name}`);
    if (raw === null) return undefined;
    try {
      return JSON.parse(raw) as T;
    } catch {
      return raw as T;
    }
  } catch {
    return undefined;
  }
}

/**
 * Whether the window's own controls show (traffic lights, the project
 * switcher, Reveal and Move to Trash, Select file…). In the window; in
 * the test harness's build only when its visual tests ask, to compare the
 * screens with the design (the controls do nothing there).
 */
export const windowLook = !serverMode || harnessWindowLook();

/**
 * Whether the app updates itself (the window app). The test harness runs
 * the update service on scripted parts, and shows it when its tests ask.
 */
export const updatesOn = !serverMode || String(harnessFixture<string | number>("updates")) === "1";

/** The platform, for key labels and the title bar. */
export const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
