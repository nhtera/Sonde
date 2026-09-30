// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { registry, useRegistry } from "../../app/registry";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { FilesIcon, ImportIcon, PlayIcon, SondeMark } from "../../components/icons";
import { serverMode } from "../../lib/mode";
import { useRuns } from "../../state/run";
import { useWorkspace } from "../../state/workspace";

/** First launch: no folder open. */
export function Welcome() {
  useRegistry();
  const importCmd = registry.getCommand("import.open");
  const example = registry.getCommand("project.openExample");
  const openKeys = useKeyLabel("folder.open");
  const importKeys = useKeyLabel("import.open");
  return (
    <div className="empty" style={{ gridColumn: "2 / -1" }}>
      <div className="card">
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          <div className="logo">
            <SondeMark size={26} />
          </div>
          <h1>The desktop app for .hurl files.</h1>
          <p>The same file runs in CI and for AI agents.</p>
        </div>
        <div className="actions">
          {!serverMode && (
            <button onClick={() => void useWorkspace.getState().openFolder()}>
              <FilesIcon size={16} />
              <span>Open a folder</span>
              {openKeys && <kbd>{openKeys}</kbd>}
            </button>
          )}
          {importCmd && (
            <button onClick={() => void importCmd.run()}>
              <ImportIcon size={16} />
              <span>Import from Postman, Bruno or curl</span>
              {importKeys && <kbd>{importKeys}</kbd>}
            </button>
          )}
          {example && (
            <button onClick={() => void example.run()}>
              <FilesIcon size={16} />
              <span>Try the example project</span>
              <span className="hint mono" style={{ fontSize: 11 }}>shop-api</span>
            </button>
          )}
        </div>
        <div className="facts">
          <span>
            <i />
            No account. No telemetry.
          </span>
          <span>
            <i />
            Plain files in git
          </span>
        </div>
      </div>
    </div>
  );
}

/** A project with no file open. */
export function NoFile() {
  const paletteKeys = useKeyLabel("palette.open");
  return (
    <div className="empty">
      <div style={{ color: "var(--muted)", textAlign: "center", lineHeight: 1.6 }}>
        Open a file from the tree{paletteKeys && <>, or find one with <kbd>{paletteKeys}</kbd></>}.
      </div>
    </div>
  );
}

/** A file with no requests yet, or never run. */
export function FileEmpty({ file, hasRequests }: { file: string; hasRequests: boolean }) {
  const runKeys = useKeyLabel("file.run");
  return (
    <div className="empty">
      <div style={{ maxWidth: 320, display: "flex", flexDirection: "column", gap: 10 }}>
        <div style={{ font: "600 14px var(--font-ui)" }}>{hasRequests ? "Not run yet" : "No requests yet"}</div>
        <div style={{ color: "var(--muted)", lineHeight: 1.5 }}>
          {hasRequests
            ? "Run the file to see each request, its captures and asserts here. Nothing is sent until you do."
            : "Write a request (GET {{base_url}}/…) or add one from the file's menu."}
        </div>
        {hasRequests && (
          <div style={{ display: "flex", gap: 6, marginTop: 4 }}>
            <button className="btn" style={{ background: "var(--raised)", boxShadow: "none" }} onClick={() => void useRuns.getState().run(file)}>
              <PlayIcon />
              Run file{runKeys && <kbd style={{ background: "none" }}>{runKeys}</kbd>}
            </button>
          </div>
        )}
        <div style={{ marginTop: 14, fontSize: 11.5, color: "var(--faint)" }}>The same file runs in CI and for AI agents.</div>
      </div>
    </div>
  );
}
