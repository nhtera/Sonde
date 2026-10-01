// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Results panel: the run's header and request list (the shared run
// components), then what the picked request sent and got back. The first
// failure is picked when a run ends; a Send dims the requests it reused.

import { useEffect, useMemo, useState, type KeyboardEvent } from "react";
import { counts } from "../../components/run/counts";
import { requestRows } from "../../components/run/model";
import { RequestList } from "../../components/run/request-list";
import { ResultsHeader } from "../../components/run/results-header";
import { outcomeOf } from "../../components/run/run-summary";
import { firstChangedLine, StaleBanner } from "../../components/run/stale-banner";
import { useEnv } from "../../state/env";
import { useHistoryView } from "../../state/history-view";
import { useRuns } from "../../state/run";
import type { FileRun } from "../../state/run-model";
import { useTabs } from "../../state/tabs";
import { useWorkspace } from "../../state/workspace";
import { EntryDetail } from "./entry-detail";
import { ago } from "./model";
import { shownEntry, useResults } from "./state";
import { SessionPanel } from "./ws-session/session-panel";

/** The time now, every few seconds (for "12s ago"). */
function useNow(ms = 5000) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), ms);
    return () => clearInterval(t);
  }, [ms]);
  return now;
}

/** The muted line under the counts: what ran. */
function noteOf(run: FileRun, total: number, now: Date): string {
  if (run.kind === "send") {
    const base = run.summary?.baseRunAt ? ` · reused captures from the run ${ago(new Date(run.summary.baseRunAt), now)}` : "";
    return `Sent request ${run.sent} only${base}`;
  }
  const done = Object.keys(run.entries).map(Number);
  const last = Math.max(0, run.current, ...done);
  if (run.running) return `Running request ${run.current || 1} of ${run.last || total}`;
  const range = last > 1 ? `requests 1–${last}` : `request ${last}`;
  if (last >= total) return `Ran ${range} · full file`;
  const failed = Object.values(run.entries).some((e) => !e.success);
  return `Ran ${range}${failed ? " · stopped at the error" : ""}`;
}

export function ResultsPanel({ file }: { file: string }) {
  const live = useRuns((s) => s.runs[file]);
  // A run opened from the history shows instead, read-only.
  const past = useHistoryView((s) => (s.view?.file === file ? s.view : null));
  const run = past?.run ?? live;
  const session = useResults((s) => s.session);
  const picked = useResults((s) => s.picked[file]);
  const index = useWorkspace((s) => s.index);
  const overrides = useEnv((s) => s.overrides.count);
  const now = useNow();
  const requests = useMemo(() => index.filter((r) => r.file === file), [index, file]);
  // The edited line only (a number): no render per keystroke.
  const changed = useTabs((s) => {
    const text = s.tabs.find((t) => t.path === file)?.text;
    return run && !run.running && text !== undefined ? firstChangedLine(run.source, text) : 0;
  });
  const lines = useMemo(() => run?.source.split("\n") ?? [], [run?.source]);

  if (session?.file === file) return <SessionPanel file={file} entry={session.entry} />;
  if (!run) return null;

  const entry = shownEntry(run, picked);
  const { passed, failed } = counts(Object.values(run.entries));
  const rows = requestRows(requests, run).map((r) =>
    run.kind === "send" ? (r.entry === run.sent ? { ...r, tag: run.running ? "sending" : "just sent" } : { ...r, dim: true }) : r,
  );
  const started = run.summary?.startedAt ? new Date(run.summary.startedAt) : null;
  const onKey = (e: KeyboardEvent) => {
    // ⌘F in the panel searches the body.
    if (e.key === "f" && (e.metaKey || e.ctrlKey)) {
      const input = (e.currentTarget as HTMLElement).querySelector<HTMLInputElement>(".body-search input");
      if (input) {
        e.preventDefault();
        e.stopPropagation();
        input.focus();
        input.select();
      }
    }
  };

  return (
    <div className="results-panel" onKeyDown={onKey}>
      <div className="results-top">
        <ResultsHeader
          title={file.split("/").at(-1)!}
          outcome={outcomeOf(run)}
          lead={run.kind === "send" ? `Sent request ${run.sent}` : undefined}
          passed={passed}
          failed={failed}
          durationMs={run.summary?.durationMs}
          env={run.summary?.env || undefined}
          when={started ? ago(started, now) : undefined}
          note={
            <>
              {noteOf(run, requests.length, now)}
              {overrides > 0 && <span className="overrides-chip small">{overrides} override{overrides === 1 ? "" : "s"}</span>}
            </>
          }
        />
        {past ? (
          <div className="history-banner" role="status">
            <span>From the history · {new Date(past.at).toLocaleString()} · read-only</span>
            <button className="btn-ghost" onClick={() => useHistoryView.getState().show(null)}>
              Back to the last run
            </button>
          </div>
        ) : (
          changed > 0 && <StaleBanner line={changed} onRun={() => void useRuns.getState().run(file)} />
        )}
        {run.error && <p className="run-error">{run.error}</p>}
        <RequestList rows={rows} selected={entry} onSelect={(n) => useResults.getState().pick(file, run.runId, n)} />
      </div>
      {entry > 0 && <EntryDetail file={file} run={run} entry={entry} lines={lines} />}
    </div>
  );
}
