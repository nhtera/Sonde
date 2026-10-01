// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Contract & mock: the check of every response against the project's
// OpenAPI spec, the operations the session's runs called, and the mock
// server (base_url points to it while it runs: an override).

import { useEffect, useState } from "react";
import { appError, Mocks } from "../../../lib/api";
import { on } from "../../../lib/events";
import { useRuns } from "../../../state/run";
import { useSettings } from "../../../state/settings";
import { useUI } from "../../../state/ui";

interface Status {
  running: boolean;
  url: string;
}

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

export function ContractPanel() {
  const settings = useSettings((s) => s.value);
  const [spec, setSpec] = useState<{ file: string; operations: string[] } | null>(null);
  const [covered, setCovered] = useState<string[]>([]);
  const [status, setStatus] = useState<Status>({ running: false, url: "" });
  const [log, setLog] = useState<string[]>([]);
  const [port, setPort] = useState("4010");
  // A run that ends may cover more operations.
  const runsDone = useRuns((s) => Object.values(s.runs).filter((r) => !r.running).length);

  useEffect(() => {
    Mocks.Spec().then((s) => setSpec({ file: s?.file ?? "", operations: s?.operations ?? [] }), fail);
    Mocks.Status().then((s) => s && setStatus(s), fail);
    const offStatus = on("mock:status", (d) => setStatus(d as Status));
    const offLog = on("mock:log", (d) => setLog((l) => [...l.slice(-199), String(d)]));
    return () => {
      offStatus();
      offLog();
    };
  }, []);
  useEffect(() => {
    Mocks.Coverage().then((c) => setCovered(c?.covered ?? []), () => setCovered([]));
  }, [runsDone]);

  if (spec && !spec.file) {
    return (
      <div className="panel-body">
        <div className="panel-head">
          <h2>Contract &amp; mock</h2>
        </div>
        <p className="form-note">sonde.yaml names no OpenAPI spec. Add an openapi: spec: entry to check responses against it and serve it as a mock.</p>
      </div>
    );
  }
  const ops = spec?.operations ?? [];
  const n = ops.filter((o) => covered.includes(o)).length;
  const start = async (p: number) => {
    try {
      const st = await Mocks.Start(p);
      if (st) setStatus(st);
    } catch (err) {
      const e = appError(err);
      const next = (e.data as { next?: number } | undefined)?.next;
      if (e.code === "busy" && next) {
        setPort(String(next));
        useUI.getState().toast({ kind: "warn", text: `${e.message}: port ${next} is free` });
      } else fail(err);
    }
  };
  return (
    <div className="panel-body">
      <div className="panel-head">
        <h2>Contract &amp; mock</h2>
      </div>
      <label className="card toggle-card">
        <span>
          <b>Check responses against {spec?.file}</b>
          <span className="muted small">Every run validates status, headers and body schema (--openapi).</span>
        </span>
        <input
          type="checkbox"
          role="switch"
          className="switch"
          aria-label="Check responses against the OpenAPI spec"
          checked={!!settings?.contract.check}
          onChange={(e) => settings && void useSettings.getState().save({ ...settings, contract: { check: e.target.checked } }).catch(fail)}
        />
      </label>
      <div className="panel-section">
        <div className="section-note">
          <span>Coverage</span>
          <span className="mono">
            {n} of {ops.length} operations
          </span>
        </div>
        <div className="coverage-bar" aria-label={`${n} of ${ops.length} operations covered`}>
          <i style={{ width: `${ops.length ? (100 * n) / ops.length : 0}%` }} />
        </div>
        <ul className="op-list" aria-label="Operations">
          {ops.map((o) => {
            const [method, ...path] = o.split(" ");
            return (
              <li key={o} className={covered.includes(o) ? "covered" : ""}>
                <span className={`mth m-${method}`}>{method}</span>
                <span className="mono path">{path.join(" ")}</span>
                <i className="dot" aria-label={covered.includes(o) ? "called" : "not called"} />
              </li>
            );
          })}
        </ul>
      </div>
      <section className="card mock-card" aria-label="Mock server">
        <div className="mock-head">
          <i className="dot" style={{ background: status.running ? "var(--pass)" : "var(--faint)" }} />
          <b>Mock server</b>
          {status.running ? (
            <button className="btn danger" onClick={() => void Mocks.Stop().then(() => setStatus({ running: false, url: "" }), fail)}>
              Stop
            </button>
          ) : (
            <span className="mock-start">
              <input className="mono" aria-label="Mock port" value={port} onChange={(e) => setPort(e.target.value.replace(/\D/g, ""))} />
              <button className="btn" onClick={() => void start(Number(port))}>
                Start
              </button>
            </span>
          )}
        </div>
        <p className="muted small mono">
          {status.running ? `${status.url.replace(/^https?:\/\//, "")} · from ${spec?.file} · base_url points here` : "Serves the spec's examples on 127.0.0.1 only."}
        </p>
        {log.length > 0 && (
          <pre className="mock-log mono" aria-label="Mock requests">
            {log.join("\n")}
          </pre>
        )}
      </section>
    </div>
  );
}
