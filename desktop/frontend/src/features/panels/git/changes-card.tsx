// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Changes card under the file tree: the files git sees changed, a
// message, and Commit to the branch (your git, your hooks, your key). A
// folder not trusted yet shows no changes: git's config there could run
// commands.

import { useState, type KeyboardEvent } from "react";
import { appError, Git } from "../../../lib/api";
import { useUI } from "../../../state/ui";
import { label } from "../../../app/keymap/keymap-manager";
import { useWorkspace } from "../../../state/workspace";

export function ChangesCard() {
  const git = useWorkspace((s) => s.git);
  const status = useWorkspace((s) => s.gitStatus);
  const lines = useWorkspace((s) => s.gitLines);
  const commitKeys = label("$mod+Enter");
  const secrets = useWorkspace((s) => s.gitSecrets);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  if (!git?.git || !git.repo) return null;
  if (!git.trusted) {
    return (
      <section className="changes-card" aria-label="Changes">
        <b>Changes</b>
        <p className="muted small">Git settings in a folder can run commands: trust it to see its changes and commit.</p>
        <button className="btn" onClick={() => void useWorkspace.getState().trust()}>
          Trust this folder to see changes
        </button>
      </section>
    );
  }
  const changed = Object.keys(status);
  if (changed.length === 0) return null;
  // Secrets files are listed, never committed.
  const files = changed.filter((f) => !secrets.includes(f));
  const commit = async () => {
    if (!message.trim() || busy || files.length === 0) return;
    setBusy(true);
    try {
      const hash = await Git.Commit(message.trim(), files);
      setMessage("");
      useUI.getState().toast({ kind: "success", text: `Committed ${hash} to ${git.branch}` });
      await useWorkspace.getState().refresh();
    } catch (err) {
      useUI.getState().toast({ kind: "error", text: appError(err).message });
    } finally {
      setBusy(false);
    }
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      e.stopPropagation();
      void commit();
    }
  };
  return (
    <section className="changes-card" aria-label="Changes">
      <div className="section-note">
        <b>Changes</b>
        <span>
          {changed.length} file{changed.length === 1 ? "" : "s"}
        </span>
      </div>
      <ul className="changes" aria-label="Changed files">
        {changed.map((f) => (
          <li key={f}>
            <span className={`git-badge g-${status[f]}`}>{status[f]}</span>
            <span className="mono">{f}</span>
            {lines[f] && lines[f][0] >= 0 && (
              <span className="lines mono" aria-label={`${lines[f][0]} lines added, ${lines[f][1]} removed`}>
                {lines[f][0] > 0 && <span className="pass">+{lines[f][0]}</span>}
                {lines[f][1] > 0 && <span className="fail"> −{lines[f][1]}</span>}
              </span>
            )}
            {secrets.includes(f) && (
              <span className="fail small" title="This file holds secrets: keep it in .gitignore">
                secrets · never committed
              </span>
            )}
          </li>
        ))}
      </ul>
      <textarea aria-label="Commit message" placeholder="Commit message" value={message} onChange={(e) => setMessage(e.target.value)} onKeyDown={onKey} />
      <button className="btn" disabled={!message.trim() || busy || files.length === 0} onClick={() => void commit()}>
        Commit to {git.branch || "HEAD"}
        {commitKeys && <kbd aria-hidden>{commitKeys}</kbd>}
      </button>
      <p className="muted small">Uses your git, your hooks and your signing key.</p>
    </section>
  );
}
