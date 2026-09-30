// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { displayPath } from "../../components/run/model";
import { useRuns } from "../../state/run";
import { formEdit } from "./edit";
import { useForm, type EntryModel } from "./model";

/** The file's requests, one chip each with its last result; + Request
 * adds one at the end. */
export function RequestStrip({ file, entries, current }: { file: string; entries: EntryModel[]; current: number }) {
  const run = useRuns((s) => s.runs[file]);
  return (
    <div className="request-strip" role="tablist" aria-label="Requests">
      {entries.map((e) => {
        const r = run?.entries[e.Index];
        return (
          <button
            key={e.Index}
            role="tab"
            aria-selected={e.Index === current}
            className="strip-chip"
            onClick={() => useForm.getState().select(file, e.Index)}
          >
            <span className="idx">{e.Index}</span>
            <span className={`mth m-${e.Method}`}>{e.Method}</span>
            <span className="mono path">{displayPath(e.URL)}</span>
            {r && <span className={r.success ? "pass" : "fail"}>{r.success ? "✓" : "✕"}</span>}
          </button>
        );
      })}
      <button
        className="add-request"
        onClick={() =>
          void formEdit(file, { kind: "addEntry", key: "GET", value: "{{base_url}}/" }).then((ok) => ok && useForm.getState().select(file, entries.length + 1))
        }
      >
        + Request
      </button>
    </div>
  );
}
