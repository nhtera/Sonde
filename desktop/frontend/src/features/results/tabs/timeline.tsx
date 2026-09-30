// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What happened on the wire: the timing waterfall, the certificate, and
// the engine's own verbose log of the request (curl -v style, secrets
// already ***). While the request is in flight, the request as sent.

import { useState } from "react";
import type { LogLine } from "../../../state/run-model";
import type { Entry, ReportRequest } from "../../../lib/view";
import { logPrefix, timingsOf } from "../model";
import { Waterfall } from "./waterfall";

export function LogLines({ logs }: { logs: LogLine[] }) {
  if (logs.length === 0) return null;
  return (
    <pre className="log-lines mono" aria-label="Log">
      {logs.map((l, i) => (
        <div key={i} className={`log-${logPrefix(l.level) === ">" ? "out" : logPrefix(l.level) === "<" ? "in" : "info"}`}>
          {logPrefix(l.level)} {l.text}
        </div>
      ))}
    </pre>
  );
}

/** The request being sent, as the log would show it. */
function Sending({ req }: { req: ReportRequest }) {
  return (
    <pre className="log-lines mono live" aria-label="Sending">
      <div className="log-out">
        {">"} {req.method} {req.url}
      </div>
      {(req.headers ?? []).map((h, i) => (
        <div key={i} className="log-out">
          {">"} {h.name}: {h.value}
        </div>
      ))}
      <div className="log-info">* Waiting for the response…</div>
    </pre>
  );
}

/** The lines of the exchange itself: what was sent and received, the
 * response's size, errors. The rest (options, the cookie store, the curl
 * command) shows on demand. */
const wire = (l: LogLine) => l.level !== "debug";

export function TimelineTab({ entry, logs, sending }: { entry?: Entry; logs: LogLine[]; sending?: ReportRequest }) {
  const [all, setAll] = useState(false);
  const call = entry?.calls?.at(-1);
  const timings = entry && timingsOf(entry);
  const cert = call?.response.certificate;
  return (
    <div className="tab-pad">
      {call && timings && <Waterfall timings={timings} secure={call.request.url.startsWith("https:") || call.request.url.startsWith("wss:")} />}
      {cert && (
        <dl className="kv cert">
          <div><dt>Subject</dt><dd className="mono">{cert.subject}</dd></div>
          <div><dt>Issuer</dt><dd className="mono">{cert.issuer}</dd></div>
          <div><dt>Expires</dt><dd className="mono">{cert.expire_date}</dd></div>
        </dl>
      )}
      {sending && !entry && <Sending req={sending} />}
      <LogLines logs={all ? logs : logs.filter(wire)} />
      {logs.some((l) => !wire(l)) && (
        <button className="btn-ghost log-toggle" onClick={() => setAll(!all)}>
          {all ? "Show the exchange only" : "Show every log line"}
        </button>
      )}
      {!call && !sending && logs.length === 0 && <p className="body-note">Nothing was sent.</p>}
    </div>
  );
}
