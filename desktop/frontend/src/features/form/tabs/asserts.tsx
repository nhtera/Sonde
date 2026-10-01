// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The expected status, then the asserts as query, predicate and value
// (Go splits them with the parser), each with the last run's result. An
// assert the grid cannot split stays one text field.

import { useState } from "react";
import { useRuns } from "../../../state/run";
import { useTabs } from "../../../state/tabs";
import { formEdit, setRow } from "../edit";
import { ExpectedStatus } from "../expected-status";
import { RowMenu } from "../row-menu";
import { lineOf, rowsOf, useForm, type Check, type EntryModel } from "../model";
import { SuggestInput, varSuggester } from "../suggest-input";

/** Predicates offered (any other can be typed). */
export const predicates = [
  "==", "!=", ">", ">=", "<", "<=", "startsWith", "endsWith", "contains", "matches", "exists", "isBoolean", "isCollection",
  "isDate", "isEmpty", "isFloat", "isInteger", "isIsoDate", "isNumber", "isString", "isUuid", "not ==", "not contains", "not exists",
];

/** "got "pending"" from a failed assert's message, else its first line. */
export function gotText(message: string): string {
  const m = /actual:\s+(?:(\w+) )?<(.*)>/.exec(message);
  if (!m) return message.split("\n")[0];
  return `got ${m[1] === "string" ? JSON.stringify(m[2]) : m[2]}`;
}

/** Whether a predicate takes no value (exists, isString…). */
export const valueless = (predicate: string) => /^(not\s+)?(exists|is[A-Z]\w*)$/.test(predicate.trim());

/** The assert text of a check's parts: a predicate without a value drops
 * it, one that needs a value and has none gets "". */
export function joinCheck(c: Pick<Check, "query" | "predicate" | "value">): string {
  const p = c.predicate.trim();
  const v = valueless(p) ? "" : c.value.trim() || '""';
  return [c.query.trim(), p, v].filter(Boolean).join(" ");
}

export function AssertsTab({ file, entry }: { file: string; entry: EntryModel }) {
  const n = entry.Index;
  const rows = rowsOf(entry, "asserts");
  const checks = useForm((s) => s.models[file]?.checks[n]) ?? [];
  const text = useTabs((s) => s.tabs.find((t) => t.path === file)?.text ?? "");
  const results = useRuns((s) => s.runs[file]?.entries[n]?.asserts);
  const [adding, setAdding] = useState(false);
  const vars = varSuggester(file);
  return (
    <div className="form-tab">
      <ExpectedStatus file={file} entry={entry} />
      <datalist id="predicates">
        {predicates.map((p) => (
          <option key={p} value={p} />
        ))}
      </datalist>
      <div className="check-grid asserts" role="table" aria-label="Asserts">
        <div className="check-head" role="row">
          <span />
          <span>Query</span>
          <span>Predicate</span>
          <span>Value</span>
          <span />
        </div>
        {rows.map((r, i) => {
          const c = checks[i];
          const line = lineOf(text, r.Range.Start);
          const res = results?.find((a) => a.line === line);
          const write = (next: Pick<Check, "query" | "predicate" | "value">) => void setRow(file, n, "asserts", i, "", joinCheck(next));
          return (
            <div key={`${i}:${r.Value}`} role="row" className={`check-row${r.Disabled ? " off" : ""}${res && !res.success ? " failed" : ""}`}>
              <input type="checkbox" aria-label={`Enable assert ${i + 1}`} checked={!r.Disabled} onChange={() => void formEdit(file, { kind: "toggleRow", entry: n, section: "asserts", index: i })} />
              {c?.parsed ? (
                <>
                  <SuggestInput label={`Assert query ${i + 1}`} className="mono" value={c.query} onCommit={(q) => write({ ...c, query: q })} />
                  <PredicateInput label={`Assert predicate ${i + 1}`} value={c.predicate} onCommit={(p) => write({ ...c, predicate: p })} />
                  <SuggestInput label={`Assert value ${i + 1}`} className="mono" value={c.value} suggest={vars} onCommit={(v) => write({ ...c, value: v })} />
                </>
              ) : (
                <span className="check-whole">
                  <SuggestInput label={`Assert ${i + 1}`} className="mono" value={r.Value} onCommit={(v) => void setRow(file, n, "asserts", i, "", v.trim())} />
                </span>
              )}
              <span className="check-state">
                {res && (res.success ? <span className="pass">✓</span> : <span className="fail">✕</span>)}
                <RowMenu file={file} entry={n} section="asserts" index={i} count={rows.length} label={`Assert ${i + 1}`} disabled={!!r.Disabled} copy={{ kind: "addAssert", entry: n, value: r.Value }} />
              </span>
              {res && !res.success && res.message && <p className="check-fail mono">{gotText(res.message)}</p>}
            </div>
          );
        })}
      </div>
      {adding ? (
        <NewAssert
          onDone={(c) => {
            setAdding(false);
            if (c.query.trim()) void formEdit(file, { kind: "addAssert", entry: n, value: joinCheck(c) });
          }}
        />
      ) : (
        <button className="add-row" onClick={() => setAdding(true)}>
          + Add assert
        </button>
      )}
    </div>
  );
}

function PredicateInput({ label, value, onCommit }: { label: string; value: string; onCommit(v: string): void }) {
  return (
    <input
      className="mono predicate"
      aria-label={label}
      list="predicates"
      defaultValue={value}
      key={value}
      onBlur={(e) => e.target.value.trim() && e.target.value.trim() !== value && onCommit(e.target.value.trim())}
      onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
    />
  );
}

function NewAssert({ onDone }: { onDone(c: Pick<Check, "query" | "predicate" | "value">): void }) {
  return (
    <form
      className="new-check"
      onSubmit={(e) => {
        e.preventDefault();
        const f = e.currentTarget.elements;
        const get = (name: string) => (f.namedItem(name) as HTMLInputElement).value;
        onDone({ query: get("query"), predicate: get("predicate") || "exists", value: get("value") });
      }}
    >
      <input className="mono" name="query" aria-label="New assert query" placeholder={'jsonpath "$.id"'} autoFocus />
      <input className="mono" name="predicate" aria-label="New assert predicate" list="predicates" placeholder="==" />
      <input className="mono" name="value" aria-label="New assert value" placeholder="1" />
      <button className="btn" type="submit">
        Add
      </button>
      <button className="btn-ghost" type="button" onClick={() => onDone({ query: "", predicate: "", value: "" })}>
        Cancel
      </button>
    </form>
  );
}
