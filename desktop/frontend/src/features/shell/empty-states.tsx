// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { registry, useRegistry } from "../../app/registry";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { DocIcon, FilesIcon, ImportIcon, PlayIcon, SondeMark } from "../../components/icons";
import { windowLook } from "../../lib/mode";
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
          {windowLook && (
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
              <DocIcon size={16} />
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

/** A file never run: run it, or check it without sending. */
export function FileEmpty({ file, hasRequests }: { file: string; hasRequests: boolean }) {
  const runKeys = useKeyLabel("file.run");
  return (
    <div className="empty">
      <div className="file-empty">
        <span className="file-empty-icon" aria-hidden>
          <PlayIcon size={14} />
        </span>
        <b>Not run yet</b>
        <p>Run the file to see each request, its captures and asserts here. Nothing is sent until you do.</p>
        <div className="file-empty-actions">
          <button className="run" disabled={!hasRequests} onClick={() => void useRuns.getState().run(file)}>
            Run file{runKeys && <kbd>{runKeys}</kbd>}
          </button>
          <button className="check" onClick={() => void registry.getCommand("file.check")?.run()}>
            Check only
          </button>
        </div>
        <small>The same file runs in CI and for AI agents.</small>
      </div>
    </div>
  );
}
