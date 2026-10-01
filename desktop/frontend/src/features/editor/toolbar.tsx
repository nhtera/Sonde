// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { registry } from "../../app/registry";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { useRuns } from "../../state/run";

export function FormatButton() {
  return (
    <button className="btn-ghost" onClick={() => void registry.getCommand("editor.format")?.run()} title="Format the file">
      Format
    </button>
  );
}

export function RunToCursorButton({ file }: { file: string }) {
  const running = useRuns((s) => s.runs[file]?.running);
  const keys = useKeyLabel("request.runTo");
  return (
    <button
      className="btn run-to"
      disabled={running}
      onClick={() => void registry.getCommand("request.runTo")?.run()}
      aria-label="Run to cursor"
      title={`Run requests 1 to the one at the cursor${keys ? ` (${keys})` : ""}`}
    >
      ⇥<span className="label"> Run to cursor</span>
      {keys && <kbd style={{ background: "none" }}>{keys}</kbd>}
    </button>
  );
}
