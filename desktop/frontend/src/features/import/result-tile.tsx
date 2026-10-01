// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What an import wrote, in honest counts, and what it could not carry over
// (grouped by kind). A Postman import offers its suggestions next.

import * as Dialog from "@radix-ui/react-dialog";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { useImport } from "./state";

const notes: Record<string, string> = {
  script: "Scripts kept as # comments (Sonde does not run scripts)",
  "unsupported-auth": "Auth settings not converted",
  "unsupported-body": "Bodies not converted",
  "unsupported-option": "Options not converted",
  "dynamic-variable": "Dynamic variables to set by hand",
  secret: "Values to move to a secret",
  unsupported: "Other items not converted",
  environments: "Environments not in the app's sonde.yaml",
};

/** The warnings of an import by kind: [kind, count, first message]. */
export function grouped(warnings: { kind: string; message: string }[]): [string, number, string][] {
  const m = new Map<string, [number, string]>();
  for (const w of warnings) {
    const g = m.get(w.kind);
    m.set(w.kind, g ? [g[0] + 1, g[1]] : [1, w.message]);
  }
  return [...m].map(([k, [n, first]]) => [k, n, first]);
}

export function ImportResult() {
  const written = useImport((s) => s.written)!;
  const preview = useImport((s) => s.preview);
  const suggestions = useImport((s) => s.suggestions);
  const c = written.counts;
  const files = written.files ?? [];
  const kept = written.kept ?? [];
  const secrets = written.secrets ?? [];
  const open = () => {
    const first = files.find((f) => /\.(hurl|sonde)$/.test(f));
    if (first) void useTabs.getState().open(first);
    useUI.getState().setPanel("files");
    useImport.getState().close();
  };
  const tiles: [number, string][] = [
    [c.requests, `request${c.requests === 1 ? "" : "s"} → ${c.files} file${c.files === 1 ? "" : "s"}`],
    [c.environments, `environment${c.environments === 1 ? "" : "s"}`],
    [c.statusChecks, `status checks → HTTP lines · ${c.pathVariables} path variable${c.pathVariables === 1 ? "" : "s"} → {{name}}`],
    [c.secretStubs, "secret stubs to fill in"],
  ];
  return (
    <div className="import-result">
      <Dialog.Title className="dialog-title">Imported</Dialog.Title>
      <p className="muted small">
        {files.length} file{files.length === 1 ? "" : "s"} written
        {kept.length > 0 && `, ${kept.length} existing kept`}
        {secrets.length > 0 && ` · ${secrets.join(", ")} in the secrets file`}. Nothing was sent.
      </p>
      <div className="import-tiles" aria-label="Import counts">
        {tiles.map(([n, label]) => (
          <div key={label} className="tile">
            <b>{n}</b>
            <span>{label}</span>
          </div>
        ))}
      </div>
      {c.scripts > 0 && <p className="note-row">{c.scripts} script{c.scripts === 1 ? "" : "s"} kept as # comments above their requests.</p>}
      {grouped(preview?.warnings ?? [])
        .filter(([k]) => k !== "script")
        .map(([k, n, first]) => (
          <p key={k} className="note-row">
            <b>
              {notes[k] ?? k} ({n})
            </b>
            <span className="muted small">{first}</span>
          </p>
        ))}
      <div className="dialog-actions">
        {suggestions.length > 0 && (
          <span className="muted small">Asserts, captures and a login request can be suggested, as diffs.</span>
        )}
        {suggestions.length > 0 && (
          <button className="btn" onClick={() => useImport.getState().review()}>
            Review suggestions ({suggestions.length})
          </button>
        )}
        <button className="btn-primary" onClick={open}>
          Open the files
        </button>
      </div>
    </div>
  );
}
