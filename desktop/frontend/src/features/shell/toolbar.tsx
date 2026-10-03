// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The bar over the editor: breadcrumb, syntax chip, then the features'
// toolbar items with the Text | Form switch after the data file.

import { registry, useRegistry } from "../../app/registry";
import { useUI } from "../../state/ui";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { isRequestPath } from "../../lib/files";

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
  // Another file (data, secrets, YAML) is text: no request tools.
  const request = isRequestPath(file);
  const ext = name?.includes(".") ? name.slice(name.lastIndexOf(".")) : "";
  const items = request ? registry.toolbarItems() : [];
  return (
    <div className="toolbar">
      <nav className="crumbs" aria-label="Path">
        {parts.length > 0 && (
          <span className="crumb-dirs">
            {parts.map((p, i) => (
              <span key={i}>
                {p} <span style={{ color: "var(--faint)" }}>/</span>{" "}
              </span>
            ))}
          </span>
        )}
        <b>{name}</b>
      </nav>
      {request ? (
        <span className={`syntax-chip${sonde ? " sonde" : ""}`} title={sonde ? "Sonde extensions allowed" : "Plain Hurl 8 syntax"}>
          {sonde ? ".sonde · extensions" : ".hurl · Hurl 8"}
        </span>
      ) : (
        <span className="syntax-chip" title="Not a request file: plain text">
          {ext ? `${ext} · text` : "text"}
        </span>
      )}
      <span style={{ flex: 1 }} />
      {/* The data file, then the view switch, then the rest. */}
      {items.filter((t) => t.order < 20).map((t) => (
        <t.render key={t.id} file={file} />
      ))}
      {request && editors.length > 1 && (
        <div className="segmented" role="group" aria-label="Editor view" title={toggleKeys && `Switch view (${toggleKeys})`}>
          {editors.map((e) => (
            <button key={e.id} aria-pressed={current === e.id} onClick={() => useUI.getState().setEditorView(e.id)}>
              {e.title}
            </button>
          ))}
        </div>
      )}
      {items.filter((t) => t.order >= 20).map((t) => (
        <t.render key={t.id} file={file} />
      ))}
      {narrow && (
        <button className="btn" aria-pressed={resultsOpen} onClick={() => useUI.getState().setResultsOpen(!resultsOpen)}>
          Results
        </button>
      )}
    </div>
  );
}
