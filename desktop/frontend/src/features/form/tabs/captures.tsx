// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Captures: a variable's name, the query it reads and whether it is a
// secret (`redact`), with the value the last run captured.

import { useState } from "react";
import { useRuns } from "../../../state/run";
import { formEdit, setRow } from "../edit";
import { rowsOf, type EntryModel } from "../model";
import { SuggestInput, varSuggester } from "../suggest-input";

const redacted = /\s+redact$/;

export function CapturesTab({ file, entry }: { file: string; entry: EntryModel }) {
  const n = entry.Index;
  const rows = rowsOf(entry, "captures");
  const got = useRuns((s) => s.runs[file]?.entries[n]?.captures);
  const [adding, setAdding] = useState(false);
  return (
    <div className="form-tab">
      <div className="check-grid captures" role="table" aria-label="Captures">
        <div className="check-head" role="row">
          <span />
          <span>Name</span>
          <span>Query</span>
          <span>Secret</span>
          <span />
        </div>
        {rows.map((r, i) => {
          const secret = redacted.test(r.Value);
          const query = r.Value.replace(redacted, "");
          const value = got?.find((c) => c.name === r.Key)?.value;
          return (
            <div key={`${i}:${r.Key}`} role="row" className={`check-row${r.Disabled ? " off" : ""}`}>
              <input type="checkbox" aria-label={`Enable ${r.Key}`} checked={!r.Disabled} onChange={() => void formEdit(file, { kind: "toggleRow", entry: n, section: "captures", index: i })} />
              <SuggestInput label={`Capture name ${i + 1}`} className="mono" value={r.Key} onCommit={(k) => void setRow(file, n, "captures", i, k.trim(), r.Value)} />
              <span className="check-query">
                <SuggestInput
                  label={`Capture query ${i + 1}`}
                  className="mono"
                  value={query}
                  suggest={varSuggester(file)}
                  onCommit={(q) => void setRow(file, n, "captures", i, r.Key, q.trim() + (secret ? " redact" : ""))}
                />
                {value !== undefined && <span className="got mono">= {typeof value === "string" ? `"${value}"` : JSON.stringify(value)}</span>}
              </span>
              <input
                type="checkbox"
                aria-label={`${r.Key} is a secret`}
                checked={secret}
                onChange={() => void setRow(file, n, "captures", i, r.Key, query + (secret ? "" : " redact"))}
              />
              <button className="kv-remove" aria-label={`Remove ${r.Key}`} onClick={() => void formEdit(file, { kind: "removeRow", entry: n, section: "captures", index: i })}>
                ×
              </button>
            </div>
          );
        })}
      </div>
      {adding ? (
        <NewCapture
          onDone={(name, query) => {
            setAdding(false);
            if (name && query) void formEdit(file, { kind: "addCapture", entry: n, key: name, value: query });
          }}
        />
      ) : (
        <button className="add-row" onClick={() => setAdding(true)}>
          + Add capture
        </button>
      )}
    </div>
  );
}

function NewCapture({ onDone }: { onDone(name: string, query: string): void }) {
  const [name, setName] = useState("");
  return (
    <form
      className="new-check"
      onSubmit={(e) => {
        e.preventDefault();
        const q = (e.currentTarget.elements.namedItem("query") as HTMLInputElement).value.trim();
        onDone(name.trim(), q);
      }}
    >
      <input className="mono" aria-label="New capture name" placeholder="name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      <input className="mono" name="query" aria-label="New capture query" placeholder={'jsonpath "$.id"'} />
      <button className="btn" type="submit">
        Add
      </button>
      <button className="btn-ghost" type="button" onClick={() => onDone("", "")}>
        Cancel
      </button>
    </form>
  );
}
