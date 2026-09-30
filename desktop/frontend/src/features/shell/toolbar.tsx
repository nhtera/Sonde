// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The bar over the editor: breadcrumb, syntax chip, the features' toolbar
// items, and the Text | Form switch.

import { registry, useRegistry } from "../../app/registry";
import { useUI } from "../../state/ui";
import { useKeyLabel } from "../../app/keymap/use-keys";

export function Toolbar({ file }: { file: string }) {
  useRegistry();
  const view = useUI((s) => s.editorView);
  const narrow = useUI((s) => s.narrow);
  const resultsOpen = useUI((s) => s.resultsOpen);
  const editors = registry.editors();
  const toggleKeys = useKeyLabel("editor.toggleView");
  const current = view ?? editors[0]?.id;
  const parts = file.split("/");
  const name = parts.pop();
  const sonde = file.endsWith(".sonde");
  return (
    <div className="toolbar">
      <nav className="crumbs" aria-label="Path">
        {parts.map((p, i) => (
          <span key={i}>
            {p} <span style={{ color: "var(--faint)" }}>/</span>
          </span>
        ))}
        <b>{name}</b>
      </nav>
      <span className="syntax-chip" title={sonde ? "Sonde extensions allowed" : "Plain Hurl 8 syntax"}>
        {sonde ? ".sonde · extensions" : ".hurl · Hurl 8"}
      </span>
      {registry.toolbarItems().map((t) => (
        <t.render key={t.id} file={file} />
      ))}
      <span style={{ flex: 1 }} />
      {editors.length > 1 && (
        <div className="segmented" role="group" aria-label="Editor view" title={toggleKeys && `Switch view (${toggleKeys})`}>
          {editors.map((e) => (
            <button key={e.id} aria-pressed={current === e.id} onClick={() => useUI.getState().setEditorView(e.id)}>
              {e.title}
            </button>
          ))}
        </div>
      )}
      {narrow && (
        <button className="btn" aria-pressed={resultsOpen} onClick={() => useUI.getState().setResultsOpen(!resultsOpen)}>
          Results
        </button>
      )}
    </div>
  );
}
