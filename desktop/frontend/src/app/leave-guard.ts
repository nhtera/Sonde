// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Leaving with unsaved edits, in the window app: closing the window or
// quitting waits (Go's close guard) while a tab is unsaved, and asks here
// first. A browser tab asks through beforeunload instead.

import { confirm } from "../components/ask";
import { CloseGuard } from "../lib/api";
import { on } from "../lib/events";
import { isDirty, useTabs } from "../state/tabs";

/** What the confirmation says for the unsaved files. */
export function leaveMessage(paths: string[]): string {
  const files = paths.length === 1 ? `${paths[0]} has` : `${paths.length} files have`;
  return `${files} unsaved changes${paths.length > 1 ? `: ${paths.join(", ")}` : ""}. Quit without saving?`;
}

/** Reports the unsaved tabs to the guard and confirms a leave it holds;
 * returns the cleanup. */
export function startLeaveGuard(): () => void {
  let reported = -1;
  const report = () => {
    const n = useTabs.getState().tabs.filter(isDirty).length;
    if (n !== reported) {
      reported = n;
      void CloseGuard.SetUnsaved(n).catch(() => (reported = -1));
    }
  };
  report();
  const unsubscribe = useTabs.subscribe(report);
  let asking = false;
  const off = on("app:leave", () => {
    if (asking) return;
    const paths = useTabs.getState().tabs.filter(isDirty).map((t) => t.path);
    if (paths.length === 0) return void CloseGuard.Leave();
    asking = true;
    void confirm({ title: "Unsaved changes", message: leaveMessage(paths), submit: "Quit without saving" })
      .then((ok) => {
        if (ok) void CloseGuard.Leave();
      })
      .finally(() => (asking = false));
  });
  return () => {
    unsubscribe();
    off();
  };
}
