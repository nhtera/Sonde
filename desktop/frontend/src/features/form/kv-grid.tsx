// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Rows of a section (Query, Headers, Form, Multipart…): a checkbox per row
// (an unchecked row is written as a `# key: value` comment), its key and
// value, a remove button, and a new row at the end. A value that spans
// lines is shown read-only: it is edited in Text.

import { useRef, useState, type ReactNode } from "react";
import { RowMenu } from "./row-menu";
import { formEdit, setRow } from "./edit";
import { rowsOf, type EntryModel, type ModelRow, type Sec } from "./model";
import { SuggestInput, varSuggester, type Suggester } from "./suggest-input";

export interface KvGridProps {
  file: string;
  entry: EntryModel;
  sec: Sec;
  keyLabel?: string;
  valueLabel?: string;
  /** The value column's header, when its cells hold more than a value. */
  valueHead?: ReactNode;
  keySuggest?: Suggester;
  /** Rows shown (their indices in the section), all by default. */
  only?(row: ModelRow, index: number): boolean;
  /** A value cell of its own (form-data's Text/File). */
  renderValue?(row: ModelRow, index: number): ReactNode;
  addLabel?: string;
}

export function KvGrid({ file, entry, sec, keyLabel = "Key", valueLabel = "Value", valueHead, keySuggest, only, renderValue, addLabel = "+ Add row" }: KvGridProps) {
  const rows = rowsOf(entry, sec)
    .map((r, i) => ({ r, i }))
    .filter(({ r, i }) => !only || only(r, i));
  const [adding, setAdding] = useState(false);
  const n = entry.Index;
  const vars = varSuggester(file);
  return (
    <div className="kv-grid" role="table" aria-label={`${sec} rows`}>
      {(rows.length > 0 || adding) && (
        <div className="kv-head" role="row">
          <span />
          <span role="columnheader">{keyLabel}</span>
          <span role="columnheader">{valueHead ?? valueLabel}</span>
          <span />
        </div>
      )}
      {rows.map(({ r, i }) => {
        const multiline = r.Value.includes("\n");
        return (
          <div key={`${i}:${r.Key}`} className={`kv-row${r.Disabled ? " off" : ""}`} role="row">
            <input
              type="checkbox"
              aria-label={`${r.Disabled ? "Enable" : "Disable"} ${r.Key}`}
              checked={!r.Disabled}
              onChange={() => void formEdit(file, { kind: "toggleRow", entry: n, section: sec, index: i })}
            />
            <SuggestInput
              label={`${keyLabel} ${i + 1}`}
              className="mono"
              value={r.Key}
              suggest={keySuggest}
              readOnly={multiline}
              onCommit={(k) => void setRow(file, n, sec, i, k.trim(), r.Value)}
            />
            {renderValue ? (
              renderValue(r, i)
            ) : multiline ? (
              <span className="kv-multiline mono" title="Spans lines: edit it in Text (⌘E)">
                {r.Value.split("\n")[0]} …
              </span>
            ) : (
              <SuggestInput label={`${valueLabel} ${i + 1}`} className="mono" value={r.Value} suggest={vars} onCommit={(v) => void setRow(file, n, sec, i, r.Key, v)} />
            )}
            <RowMenu file={file} entry={n} section={sec} index={i} label={r.Key} disabled={!!r.Disabled} copy={{ kind: "addRow", entry: n, section: sec, key: r.Key, value: r.Value }} />
          </div>
        );
      })}
      {adding ? (
        <NewRow
          keyLabel={keyLabel}
          valueLabel={valueLabel}
          keySuggest={keySuggest}
          vars={vars}
          onDone={(k, v) => {
            setAdding(false);
            if (k.trim()) void setRow(file, n, sec, -1, k.trim(), v);
          }}
        />
      ) : (
        <button className="add-row" onClick={() => setAdding(true)}>
          {addLabel}
        </button>
      )}
    </div>
  );
}

/** A row being added: written once it has a key and loses the focus. */
function NewRow({ keyLabel, valueLabel, keySuggest, vars, onDone }: { keyLabel: string; valueLabel: string; keySuggest?: Suggester; vars: Suggester; onDone(key: string, value: string): void }) {
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  // The fields commit on blur, just before the row sees the focus leave:
  // refs hold what they committed.
  const latest = useRef({ key: "", value: "" });
  return (
    <div
      className="kv-row new"
      role="row"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node)) onDone(latest.current.key, latest.current.value);
      }}
    >
      <input type="checkbox" checked disabled aria-label="New row" />
      <SuggestInput label={`New ${keyLabel.toLowerCase()}`} className="mono" value={key} suggest={keySuggest} onCommit={(k) => { latest.current.key = k; setKey(k); }} placeholder={keyLabel} />
      <SuggestInput label={`New ${valueLabel.toLowerCase()}`} className="mono" value={value} suggest={vars} onCommit={(v) => { latest.current.value = v; setValue(v); }} placeholder={valueLabel} />
      <span />
    </div>
  );
}
