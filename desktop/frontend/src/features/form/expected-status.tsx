// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useRuns } from "../../state/run";
import { formEdit } from "./edit";
import type { EntryModel } from "./model";
import { ListInput } from "./list-input";

/** Statuses offered (any other can be typed; * is any). */
const statuses = ["200", "201", "202", "204", "301", "302", "304", "400", "401", "403", "404", "409", "422", "429", "500", "503", "*"];

/** The expected status: it writes the `HTTP N` line (never an assert
 * row), with the last run's result next to it. */
export function ExpectedStatus({ file, entry }: { file: string; entry: EntryModel }) {
  const got = useRuns((s) => s.runs[file]?.entries[entry.Index]?.calls?.at(-1)?.response.status);
  const status = entry.HasResponse ? entry.Status : "";
  const ok = got !== undefined && status !== "" && (status === "*" || String(got) === status);
  return (
    <div className="expected-status">
      <span className="label">Expected status</span>
      <datalist id="statuses">
        {statuses.map((s) => (
          <option key={s} value={s} />
        ))}
      </datalist>
      <ListInput
        label="Expected status"
        list="statuses"
        className="status-input"
        value={status}
        placeholder="none"
        onCommit={(v) => /^(\*|\d{3})$/.test(v) && void formEdit(file, { kind: "setStatus", entry: entry.Index, value: v })}
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
