// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Test run side panel: the files to run, the contract check, the
// environment, and the same run as a command for CI.

import { useEffect, useState } from "react";
import { appError, CopyAs, Dialogs } from "../../../lib/api";
import { serverMode } from "../../../lib/mode";
import { useEnv } from "../../../state/env";
import { useSettings } from "../../../state/settings";
import { useUI } from "../../../state/ui";
import { useWorkspace } from "../../../state/workspace";
import { fileSucceeded, requestFiles, useTestRun } from "./state";

/** Picks a data file, then runs the one picked file once per row. */
async function runWithData(files: string[]) {
  try {
    const handle = await Dialogs.OpenFile("Data file", "CSV or JSON", "*.csv;*.json");
    if (handle) await useTestRun.getState().start(files, handle);
  } catch (err) {
    useUI.getState().toast({ kind: "error", text: appError(err).message });
  }
}

export function TestRunSide() {
  const tree = useWorkspace((s) => s.tree);
  const excluded = useTestRun((s) => s.excluded);
  const running = useTestRun((s) => s.running);
  const results = useTestRun((s) => s.files);
  const env = useEnv((s) => s.current);
  const settings = useSettings((s) => s.value);
  const all = requestFiles(tree);
  const picked = all.filter((f) => !excluded.includes(f));
  const summary = useTestRun((s) => s.summary);
  // The overrides reload with every change of env or settings.
  const overrides = useEnv((s) => s.overrides);
  const jobs = useTestRun((s) => s.jobs);
  const continueOnError = useTestRun((s) => s.continueOnError);
  const [command, setCommand] = useState({ text: "", note: "" });
  const key = `${env}|${picked.join(",")}|${JSON.stringify(overrides)}|${jobs}|${continueOnError}`;
  useEffect(() => {
    let live = true;
    CopyAs.Sonde({ file: "", source: "", env, entry: 0, kind: "test", files: picked, shell: "posix", clock: "", jobs, continueOnError }).then(
      (t) => live && setCommand({ text: t?.text ?? "", note: t?.note ?? "" }),
      () => live && setCommand({ text: "", note: "" }),
    );
    return () => {
      live = false;
    };
    // key holds env, the files picked and the overrides.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  const setCheck = (check: boolean) =>
    settings &&
    useSettings
      .getState()
      .save({ ...settings, contract: { check } })
      .catch((err) => useUI.getState().toast({ kind: "error", text: appError(err).message }));
  const dot = (f: string) => {
    const r = results[f];
    if (r?.running) return undefined;
    const ok = fileSucceeded(f, summary, r);
    return ok === undefined ? undefined : ok ? "var(--pass)" : "var(--fail)";
  };
  return (
    <div className="panel-body">
      <div className="panel-head">
        <h2>Test run</h2>
      </div>
      <label className="check-row all">
        <input type="checkbox" checked={picked.length === all.length && all.length > 0} onChange={(e) => useTestRun.getState().setAll(e.target.checked, all)} />
        <span>All .hurl and .sonde files</span>
        <span className="muted">{all.length}</span>
      </label>
      <ul className="file-checks" aria-label="Files to run">
        {all.map((f) => (
          <li key={f}>
            <label className="check-row">
              <input type="checkbox" checked={!excluded.includes(f)} onChange={() => useTestRun.getState().toggle(f)} />
              <span className="mono">{f}</span>
              {dot(f) && <i className="dot" style={{ background: dot(f) }} />}
            </label>
          </li>
        ))}
      </ul>
      <div className="panel-section">
        <h3>Options</h3>
        <label className="opt-line">
          <span>Parallel jobs</span>
          <input
            className="mono jobs"
            type="number"
            min={0}
            max={64}
            aria-label="Parallel jobs"
            placeholder="auto"
            value={jobs || ""}
            onChange={(e) => useTestRun.getState().setOptions({ jobs: Math.max(0, Math.min(64, Number(e.target.value) || 0)) })}
          />
        </label>
        <label className="opt-line">
          <span>Continue after a failed request</span>
          <input
            type="checkbox"
            role="switch"
            className="switch"
            aria-label="Continue after a failed request"
            checked={continueOnError}
            onChange={(e) => useTestRun.getState().setOptions({ continueOnError: e.target.checked })}
          />
        </label>
        <label className="opt-line">
          <span>Check against the OpenAPI spec</span>
          <input type="checkbox" role="switch" className="switch" aria-label="Check responses against the OpenAPI spec" checked={!!settings?.contract.check} onChange={(e) => setCheck(e.target.checked)} />
        </label>
        <div className="opt-line">
          <span>Environment</span>
          <span className="mono">{env || "none"}</span>
        </div>
      </div>
      <button className="btn-primary wide" disabled={running || picked.length === 0} onClick={() => void useTestRun.getState().start(picked)}>
        {running ? "Running…" : `Run ${picked.length} file${picked.length === 1 ? "" : "s"}`}
      </button>
      {!serverMode && (
        <button className="btn wide" disabled={running || picked.length !== 1} title="Runs the one picked file once per CSV or JSON row (--data)" onClick={() => void runWithData(picked)}>
          Run with a data file…
        </button>
      )}
      {command.text && (
        <div className="cli-command">
          <div className="section-note">
            <span>The same run in CI</span>
            <button className="btn-ghost" onClick={() => void navigator.clipboard.writeText(command.text).then(() => useUI.getState().toast({ kind: "success", text: "Command copied" }))}>
              Copy
            </button>
          </div>
          <code className="mono" aria-label="Test run command">
            {command.text}
          </code>
          {command.note && <p className="muted small">{command.note}</p>}
        </div>
      )}
    </div>
  );
}
