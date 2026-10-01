// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Test run report: the summary `sonde --test` prints, the checks, a
// row per file with its first failure, Re-run failed, and the reports.

import { counts } from "../../../components/run/counts";
import { DataRowPills } from "../../../components/run/data-row-pills";
import { failureOf } from "../../../components/run/model";
import { appError, Dialogs, Reports } from "../../../lib/api";
import { serverMode } from "../../../lib/mode";
import { useTabs } from "../../../state/tabs";
import { useUI } from "../../../state/ui";
import type { FileRun } from "../../../state/run-model";
import { fileSucceeded, requestFiles, useTestRun } from "./state";
import { useWorkspace } from "../../../state/workspace";

/** The totals block of the summary text (after its rule). */
export function totals(text: string): string {
  const at = text.indexOf("-----");
  return at < 0 ? text : text.slice(text.indexOf("\n", at) + 1).trimEnd();
}

const formats = [
  { id: "html", label: "HTML" },
  { id: "json", label: "JSON" },
  { id: "junit", label: "JUnit" },
  { id: "tap", label: "TAP" },
];

async function exportReport(runId: string, format: string) {
  try {
    const dir = await Dialogs.OpenFolder("Save the report in");
    if (!dir) return;
    const path = await Reports.Export(runId, format, dir);
    useUI.getState().toast({ kind: "success", text: `Report written: ${path}` });
  } catch (err) {
    useUI.getState().toast({ kind: "error", text: appError(err).message });
  }
}

function outcome(r: FileRun | undefined, ok: boolean | undefined, errored: boolean): "Passed" | "Failed" | "Error" | "Running" | "" {
  if (r?.running) return "Running";
  if (errored) return "Error";
  if (ok === undefined) return "";
  return ok ? "Passed" : "Failed";
}

export function TestRunMain() {
  const { summary, files, running, runId } = useTestRun();
  const tree = useWorkspace((s) => s.tree);
  const all = Object.values(files).flatMap((r) => Object.values(r.entries));
  const { passed, failed } = counts(all);
  const open = (file: string) => {
    void useTabs.getState().open(file);
    useUI.getState().setPanel("files");
  };
  const failedFiles = (summary?.units ?? []).filter((u) => !u.success).map((u) => u.file);
  const order = requestFiles(tree);
  const rows = [...new Set([...(summary?.units ?? []).map((u) => u.file), ...Object.keys(files)])].sort((a, b) => order.indexOf(a) - order.indexOf(b));
  if (!summary && !running) {
    return (
      <div className="testrun-main empty">
        <h1>Test run</h1>
        <p className="muted">Pick files on the left and run them as `sonde --test` does: every file, a line each, then the totals.</p>
      </div>
    );
  }
  return (
    <div className="testrun-main">
      <header className="testrun-head">
        <div>
          <h1>Test run</h1>
          <p className="muted">
            {summary ? `${summary.files} files · ${summary.env || "no env"} · ${summary.outcome}` : "Running…"}
          </p>
        </div>
        <button className="btn" disabled={running || failedFiles.length === 0} onClick={() => void useTestRun.getState().start(failedFiles)}>
          Re-run failed
        </button>
        <button className="btn" disabled={running || rows.length === 0} onClick={() => void useTestRun.getState().start(rows)}>
          Run again
        </button>
      </header>
      {summary?.text && (
        <section className="summary-card">
          <pre className="mono" aria-label="Test summary">
            {totals(summary.text)}
          </pre>
          <div className="checks">
            <div className="bar" aria-hidden>
              <i className="ok" style={{ flex: passed || 0.0001 }} />
              <i className="ko" style={{ flex: failed }} />
            </div>
            <div className="checks-line">
              <span>
                <b className="pass">{passed}</b> checks passed
              </span>
              {failed > 0 && <b className="fail">{failed} failed</b>}
            </div>
            <p className="muted">Same summary as sonde --test in CI.</p>
            <button className="btn-ghost" onClick={() => void navigator.clipboard.writeText(summary.text ?? "")}>
              Copy the output
            </button>
          </div>
        </section>
      )}
      <table className="file-table" aria-label="Files">
        <thead>
          <tr>
            <th>File</th>
            <th>Result</th>
            <th>Requests</th>
            <th>Checks</th>
            <th>Time</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((f) => {
            const r = files[f];
            const units = (summary?.units ?? []).filter((u) => u.file === f);
            const unit = units[0];
            // A data-driven file runs once per row: a pill each.
            const dataRows = units.filter((u) => (u.row ?? 0) > 0);
            const entries = Object.values(r?.entries ?? {});
            const c = counts(entries);
            const first = entries.map((e) => failureOf(e)).find(Boolean);
            const res = outcome(r, fileSucceeded(f, summary, r), units.some((u) => !!u.parseError || !!u.error));
            return (
              <tr key={f} onClick={() => open(f)}>
                <td>
                  <span className="mono">{f}</span>
                  {first && (
                    <div className="first-fail mono">
                      line {first.line} · {first.code ?? first.title}
                      {first.actual !== undefined ? ` · got ${first.actual}` : ""}
                    </div>
                  )}
                  {unit?.parseError && <div className="first-fail mono">line {unit.parseError.line} · {unit.parseError.description}</div>}
                  {unit?.error && <div className="first-fail mono">{unit.error}</div>}
                  {dataRows.length > 0 && <DataRowPills rows={dataRows.map((u) => ({ row: u.row ?? 0, state: u.success ? "passed" : "failed" }))} />}
                </td>
                <td>{res && <span className={`outcome outcome-${res.toLowerCase()}`}>{res}</span>}</td>
                <td className="num">{unit ? units.reduce((n, u) => n + u.requests, 0) : entries.length}</td>
                <td className="num">
                  <span className="pass">✓ {c.passed}</span>
                  {c.failed > 0 && <span className="fail"> ✕ {c.failed}</span>}
                </td>
                <td className="num mono" data-volatile>{unit ? `${units.reduce((n, u) => n + u.durationMs, 0)} ms` : ""}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {summary && runId && !serverMode && (
        <div className="export-row">
          <span className="muted">Export report</span>
          {formats.map((f) => (
            <button key={f.id} className="btn" onClick={() => void exportReport(runId, f.id)}>
              {f.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
