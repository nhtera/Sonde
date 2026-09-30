// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";
import { appError } from "../../lib/api";
import { useUI } from "../../state/ui";
import { useWorkspace } from "../../state/workspace";

// "Not now" is remembered per folder in this browser only: a convenience,
// the trust itself is kept by the app.
const NOT_NOW = "sonde.trust.notNow";

function notNow(dir: string): boolean {
  try {
    return (JSON.parse(localStorage.getItem(NOT_NOW) ?? "[]") as string[]).includes(dir);
  } catch {
    return false;
  }
}

function rememberNotNow(dir: string) {
  try {
    const list = JSON.parse(localStorage.getItem(NOT_NOW) ?? "[]") as string[];
    localStorage.setItem(NOT_NOW, JSON.stringify([...list.filter((d) => d !== dir), dir].slice(-50)));
  } catch {
    // storage unavailable: dismissed for this session only
  }
}

/**
 * Asks, once per folder, to trust its git settings: until then the app
 * runs no git command in it (a repository's config can run programs), and
 * only the branch shows.
 */
export function TrustBar() {
  const git = useWorkspace((s) => s.git);
  const dir = useWorkspace((s) => s.project?.dir);
  const [dismissed, setDismissed] = useState<string | null>(null);
  if (!git?.repo || !git.git || git.trusted || !dir || dismissed === dir || notNow(dir)) return null;
  return (
    <div className="trust-bar" role="note">
      <span>Trust this folder's git settings to show changes and commit.</span>
      <button
        className="btn"
        onClick={() => useWorkspace.getState().trust().catch((err) => useUI.getState().toast({ kind: "error", text: appError(err).message }))}
      >
        Trust
      </button>
      <button
        className="btn-ghost"
        onClick={() => {
          rememberNotNow(dir);
          setDismissed(dir);
        }}
      >
        Not now
      </button>
    </div>
  );
}
