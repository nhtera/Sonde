// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// New project: a folder of its name in Documents/Sonde (or a location
// picked), with a starter sonde.yaml, opened at once: no folder to make
// first in the system's dialog. Opened from the project switcher, the
// welcome screen, the palette, and an import with no project open.

import * as Dialog from "@radix-ui/react-dialog";
import { useEffect, useState } from "react";
import { create } from "zustand";
import { appError, WorkspaceDesktop } from "../../lib/api";
import { useWorkspace } from "../../state/workspace";

interface Ask {
  /** For an import: the dialog says so, and offers an existing folder. */
  forImport: boolean;
  resolve(created: boolean): void;
}

const useNewProject = create<{ ask: Ask | null }>(() => ({ ask: null }));

/** Opens the New project dialog; true once the project is made and open
 * (for an import, an existing folder picked instead counts too). */
export function newProject(forImport = false): Promise<boolean> {
  useNewProject.getState().ask?.resolve(false);
  return new Promise((resolve) => useNewProject.setState({ ask: { forImport, resolve } }));
}

export function NewProjectDialog() {
  const ask = useNewProject((s) => s.ask);
  if (!ask) return null;
  return <NewProjectForm ask={ask} />;
}

function NewProjectForm({ ask }: { ask: Ask }) {
  const [name, setName] = useState(ask.forImport ? "Imported" : "Untitled project");
  const [parent, setParent] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    void WorkspaceDesktop.ProjectsDir().then((dir) => setParent((p) => p || dir));
  }, []);
  const close = (created: boolean) => {
    useNewProject.setState({ ask: null });
    ask.resolve(created);
  };
  const create = async () => {
    setBusy(true);
    setError("");
    try {
      if (await useWorkspace.getState().createProject(parent, name)) close(true);
    } catch (err) {
      setError(appError(err).message);
    } finally {
      setBusy(false);
    }
  };
  const change = async () => {
    const dir = await WorkspaceDesktop.PickProjectsDir().catch(() => "");
    if (dir) setParent(dir);
  };
  const existing = async () => {
    await useWorkspace.getState().openFolder(true);
    close(!!useWorkspace.getState().project);
  };
  const sep = parent.includes("\\") && !parent.includes("/") ? "\\" : "/";
  return (
    <Dialog.Root open onOpenChange={(open) => !open && close(false)}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog dialog-pad new-project" aria-describedby="new-project-note">
          <Dialog.Title className="dialog-title">{ask.forImport ? "Make a project to import into" : "New project"}</Dialog.Title>
          <p id="new-project-note" className="dialog-note">
            A folder with its own sonde.yaml (environment local), history and cookie jar.
            {ask.forImport && " You pick the collection to import next."}
          </p>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (!busy && name.trim() && parent) void create();
            }}
          >
            <label className="define-field">
              <span>Name</span>
              <input aria-label="Project name" autoFocus spellCheck={false} autoComplete="off" value={name} onFocus={(e) => e.target.select()} onChange={(e) => setName(e.target.value)} />
            </label>
            <div className="define-field">
              <span>Location</span>
              <div className="new-project-where">
                <code className="mono" title={parent ? `${parent}${sep}${name.trim()}` : undefined}>
                  {parent ? `${parent}${sep}${name.trim()}` : "…"}
                </code>
                <button type="button" className="btn" onClick={() => void change()}>
                  Change…
                </button>
              </div>
            </div>
            {error && (
              <span className="define-error" role="alert">
                {error}
              </span>
            )}
            <div className="dialog-actions">
              {ask.forImport && (
                <button type="button" className="btn-ghost new-project-existing" onClick={() => void existing()}>
                  Use an existing folder…
                </button>
              )}
              <button type="button" className="btn-ghost" onClick={() => close(false)}>
                Cancel
              </button>
              <button type="submit" className="btn-primary" disabled={busy || !name.trim() || !parent}>
                Create
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
