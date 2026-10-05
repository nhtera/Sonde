// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { leaveMessage } from "../../app/leave-guard";
import { confirm } from "../../components/ask";
import { Update, appError } from "../../lib/api";
import { isDirty, useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";

/** Restarts into the downloaded update, after asking about unsaved tabs:
 * the restart quits without saving them. */
export async function restartToUpdate(): Promise<void> {
  const paths = useTabs.getState().tabs.filter(isDirty).map((t) => t.path);
  if (paths.length > 0) {
    const ok = await confirm({ title: "Unsaved changes", message: leaveMessage(paths, "Restart"), submit: "Restart without saving" });
    if (!ok) return;
  }
  try {
    await Update.Restart();
  } catch (err) {
    useUI.getState().toast({ kind: "error", text: appError(err).message });
  }
}

/** Opens the release page in the browser. */
export function openReleasePage(): void {
  Update.OpenReleasePage().catch((err) => useUI.getState().toast({ kind: "error", text: appError(err).message }));
}
