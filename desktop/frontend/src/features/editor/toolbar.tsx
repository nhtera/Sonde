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
    <button className="btn" disabled={running} onClick={() => void registry.getCommand("request.runTo")?.run()} title="Run requests 1 to the one at the cursor">
      ⇥ Run to cursor{keys && <kbd style={{ background: "none" }}>{keys}</kbd>}
    </button>
  );
}
