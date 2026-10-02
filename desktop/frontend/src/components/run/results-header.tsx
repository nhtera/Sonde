// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from "react";
import { envColor } from "../../lib/env-color";

export type Outcome = "running" | "passed" | "failed" | "canceled" | "error" | "interrupted";

const outcomeLabel: Record<Outcome, string> = {
  running: "Running",
  passed: "Passed",
  failed: "Failed",
  canceled: "Canceled",
  error: "Error",
  interrupted: "Interrupted",
};

export interface ResultsHeaderProps {
  title: string;
  outcome: Outcome;
  passed: number;
  failed: number;
  durationMs?: number;
  /** Extra counts before passed (e.g. "3 rows"). */
  lead?: string;
  env?: string;
  /** The muted line under the counts ("Ran requests 1–5 · full file"). */
  note?: ReactNode;
  /** When it ran ("12s ago"). */
  when?: string;
  /** The counts in place of passed and failed (a stream's events). */
  counts?: ReactNode;
}

/** The title, outcome and counts above a run's results. */
export function ResultsHeader(p: ResultsHeaderProps) {
  return (
    <header className="run-header">
      <div className="run-title">
        <h2>{p.title}</h2>
        <span className={`outcome outcome-${p.outcome}`}>{outcomeLabel[p.outcome]}</span>
        {p.env && (
          <span className="env-chip">
            <i className="dot" style={{ background: envColor(p.env) }} />
            {p.env}
          </span>
        )}
      </div>
      <div className="run-counts">
        {p.lead && <span>{p.lead}</span>}
        {p.counts ?? (
          <>
            <span>
              <b className="pass">{p.passed}</b> passed
            </span>
            <span>
              <b className={p.failed ? "fail" : ""}>{p.failed}</b> failed
            </span>
          </>
        )}
        {p.durationMs !== undefined && !p.counts && (
          <span className="mono" data-volatile>
            {p.durationMs} ms
          </span>
        )}
        {p.when && <span data-volatile>{p.when}</span>}
      </div>
      {p.note && <div className="run-note">▸ {p.note}</div>}
    </header>
  );
}
