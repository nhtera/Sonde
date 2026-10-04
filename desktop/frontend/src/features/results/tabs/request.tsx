// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { Entry, ReportRequest } from "../../../lib/view";
import { useUI } from "../../../state/ui";
import { NameValues } from "./headers";
import { copyText } from "../../../lib/clipboard";

/** The request as sent (redacted): line, query, headers, cookies, curl. */
export function RequestTab({ entry, sending }: { entry?: Entry; sending?: ReportRequest }) {
  const req = entry?.calls?.at(-1)?.request ?? sending;
  const curl = entry?.curl_cmd;
  if (!req) return <p className="body-note tab-pad">Nothing was sent.</p>;
  return (
    <div className="tab-pad">
      <p className="req-line mono">
        <b className={`m-${req.method}`}>{req.method}</b> {req.url}
      </p>
      {(req.query_string?.length ?? 0) > 0 && <NameValues title="Query" rows={req.query_string ?? []} />}
      <NameValues title="Headers" rows={req.headers ?? []} />
      {(req.cookies?.length ?? 0) > 0 && <NameValues title="Cookies" rows={req.cookies ?? []} />}
      {curl && (
        <section className="kv-section">
          <h3>
            curl{" "}
            <button
              className="btn-ghost"
              onClick={() =>
                void copyText(curl).then(() => useUI.getState().toast({ kind: "success", text: "curl command copied" }))
              }
            >
              Copy
            </button>
          </h3>
          <pre className="log-lines mono">{curl}</pre>
        </section>
      )}
    </div>
  );
}
