// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useRuns } from "../../state/run";
import { formEdit } from "./edit";
import type { EntryModel } from "./model";
import { SuggestInput } from "./suggest-input";

/** The expected status: it writes the `HTTP N` line (never an assert
 * row), with the last run's result next to it. */
export function ExpectedStatus({ file, entry }: { file: string; entry: EntryModel }) {
  const got = useRuns((s) => s.runs[file]?.entries[entry.Index]?.calls?.at(-1)?.response.status);
  const status = entry.HasResponse ? entry.Status : "";
  const ok = got !== undefined && status !== "" && (status === "*" || String(got) === status);
  return (
    <div className="expected-status">
      <span className="label">Expected status</span>
      <SuggestInput
        label="Expected status"
        className="mono status-input"
        value={status}
        placeholder="none"
        onCommit={(v) => /^(\*|\d{3})$/.test(v.trim()) && void formEdit(file, { kind: "setStatus", entry: entry.Index, value: v.trim() })}
      />
      {got !== undefined && status !== "" && (
        <span className={ok ? "pass" : "fail"} title={`The last run got ${got}`}>
          {ok ? "✓" : `✕ got ${got}`}
        </span>
      )}
      <span className="note mono">{status ? `writes HTTP ${status} · not an assert` : "no HTTP line: the status is not checked"}</span>
    </div>
  );
}
