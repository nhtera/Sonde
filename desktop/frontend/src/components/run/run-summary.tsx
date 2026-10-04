// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A file's run as a header and its request list: what the Results panel
// and each file of the Test run panel show.

import type { ReactNode } from "react";
import type { Request } from "../../lib/api";
import type { FileRun } from "../../state/run-model";
import { counts } from "./counts";
import { requestRows } from "./model";
import { RequestList } from "./request-list";
import { ResultsHeader, type Outcome } from "./results-header";

export interface RunSummaryProps {
  file: string;
  run: FileRun;
  /** The file's requests (Workspace.Index). */
  requests: Request[];
  selected?: number;
  onSelect?(entry: number): void;
  /** Shown between the header and the requests (the stale banner). */
  notice?: ReactNode;
}

/** The outcome a run shows: running, its summary's, or an error. */
export function outcomeOf(run: FileRun): Outcome {
  if (run.running) return "running";
  if (run.summary) return run.summary.outcome as Outcome;
  if (run.outcome) return run.outcome;
  return run.error ? "error" : "failed";
}

export function RunSummary({ file, run, requests, selected, onSelect, notice }: RunSummaryProps) {
  const { passed, failed } = counts(Object.values(run.entries));
  return (
    <>
      <ResultsHeader
        title={file.split("/").at(-1)!}
        outcome={outcomeOf(run)}
        passed={passed}
        failed={failed}
        durationMs={run.summary?.durationMs}
        env={run.summary?.env || undefined}
      />
      {notice}
      {run.error && <p className="run-error">{run.error}</p>}
      <RequestList rows={requestRows(requests, run)} selected={selected} onSelect={onSelect} />
    </>
  );
}
