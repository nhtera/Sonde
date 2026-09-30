// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Picks a file for a body (form-data, binary). Runs read files inside the
// project only: a file outside is copied in first (never written with
// ..), or not written at all. The window app only: server mode types the
// path.

import { useState } from "react";
import { appError, Dialogs, WorkspaceDesktop } from "../../../../lib/api";
import { serverMode } from "../../../../lib/mode";
import { useUI } from "../../../../state/ui";
import { useWorkspace } from "../../../../state/workspace";

export interface Outside {
  name: string;
  handle: string;
}

/** A "Select file…" button: onPath gets a project path; a file outside
 * the project is reported with onOutside. */
export function SelectFile({ onPath, onOutside }: { onPath(path: string): void; onOutside(o: Outside): void }) {
  if (serverMode) return null;
  const pick = async () => {
    try {
      const handle = await Dialogs.OpenFile("Select a file", "", "");
      if (!handle) return;
      const p = await WorkspaceDesktop.PickedFile(handle);
      if (!p) return;
      if (p.path) onPath(p.path);
      else if (p.handle) onOutside({ name: p.name, handle: p.handle });
    } catch (err) {
      useUI.getState().toast({ kind: "error", text: appError(err).message });
    }
  };
  return (
    <button className="btn-ghost accent" onClick={() => void pick()}>
      Select file…
    </button>
  );
}

/** The notice for a file outside the project, with Copy into project. */
export function OutsideNotice({ outside, onCopied, onDismiss }: { outside: Outside; onCopied(path: string): void; onDismiss(): void }) {
  const project = useWorkspace((s) => s.project?.name ?? "the project");
  const [busy, setBusy] = useState(false);
  const target = `assets/${outside.name}`;
  const copy = async () => {
    setBusy(true);
    try {
      onCopied(await WorkspaceDesktop.CopyIntoProject(outside.handle, "assets"));
    } catch (err) {
      useUI.getState().toast({ kind: "error", text: appError(err).message });
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="outside-notice" role="alert">
      <span>
        {outside.name} is outside {project}. Sonde only reads files inside the project, so this row is not written yet.
      </span>
      <button className="btn" disabled={busy} onClick={() => void copy()}>
        Copy into project → {target}
      </button>
      <button className="btn-ghost" onClick={onDismiss}>
        Cancel
      </button>
    </div>
  );
}
