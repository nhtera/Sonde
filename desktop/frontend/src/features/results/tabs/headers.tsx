// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { Entry } from "../../../lib/view";

/** Name and value rows. */
export function NameValues({ title, rows, empty }: { title: string; rows: { name: string; value: string }[]; empty?: string }) {
  return (
    <section className="kv-section">
      <h3>
        {title} <span className="muted">{rows.length}</span>
      </h3>
      {rows.length === 0 ? (
        <p className="body-note">{empty ?? "None"}</p>
      ) : (
        <dl className="kv mono">
          {rows.map((r, i) => (
            <div key={i}>
              <dt>{r.name}</dt>
              <dd>{r.value}</dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}

/** The response's headers, then the request's. */
export function HeadersTab({ entry }: { entry: Entry }) {
  const call = entry.calls?.at(-1);
  if (!call) return null;
  return (
    <div className="tab-pad">
      <NameValues title="Response headers" rows={call.response.headers ?? []} />
      <NameValues title="Request headers" rows={call.request.headers ?? []} />
    </div>
  );
}
