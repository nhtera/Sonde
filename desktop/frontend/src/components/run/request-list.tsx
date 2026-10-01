// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { RequestRow } from "./model";

const mark = { passed: "✓", failed: "✕", running: "…", skipped: "–", pending: "" };

export interface RequestListProps {
  rows: RequestRow[];
  selected?: number;
  onSelect?(entry: number): void;
}

/** The requests of a run: method, path, status, time, captures. */
export function RequestList({ rows, selected, onSelect }: RequestListProps) {
  return (
    <ol className="request-list" aria-label="Requests">
      {rows.map((r) => (
        <li
          key={r.entry}
          className={`req-row state-${r.state}${r.dim ? " dim" : ""}`}
          aria-selected={selected === r.entry}
          aria-label={`${r.method} ${r.path} ${r.state}`}
          onClick={() => onSelect?.(r.entry)}
        >
          <span className="idx mono">{r.entry}</span>
          <span className={`mth m-${r.method}`}>{r.method}</span>
          <span className="path mono">{r.path}</span>
          {r.state === "skipped" ? (
            <span className="skip mono">skip</span>
          ) : (
            <>
              <span className={`code mono ${r.state === "failed" ? "fail" : "pass"}`}>{r.status ?? (r.state === "failed" ? "ERR" : "")}</span>
              <span className="ms mono" data-volatile>{r.ms !== undefined ? `${r.ms} ms` : ""}</span>
            </>
          )}
          <span className="mark">{mark[r.state]}</span>
          {(!!r.tag || (r.used?.length ?? 0) > 0 || r.captured.length > 0) && (
            <span className="vars">
              {r.tag && <span className="var-chip tag">● {r.tag}</span>}
              {r.used?.map((v) => (
                <span key={`u-${v}`} className="var-chip">
                  ← {v}
                </span>
              ))}
              {r.captured.map((v) => (
                <span key={`c-${v}`} className="var-chip out">
                  → {v}
                </span>
              ))}
            </span>
          )}
        </li>
      ))}
    </ol>
  );
}
