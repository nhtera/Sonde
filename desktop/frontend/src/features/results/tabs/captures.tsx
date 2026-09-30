// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { Entry } from "../../../lib/view";

const show = (v: unknown) => (typeof v === "string" ? v : JSON.stringify(v));

/** The variables the request captured (a redacted one shows ***). */
export function CapturesTab({ entry }: { entry: Entry }) {
  const caps = entry.captures ?? [];
  if (caps.length === 0) return <p className="body-note tab-pad">This request captures nothing.</p>;
  return (
    <div className="tab-pad">
      <dl className="kv mono">
        {caps.map((c) => (
          <div key={c.name}>
            <dt className="cap-name">→ {c.name}</dt>
            <dd>{show(c.value)}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
