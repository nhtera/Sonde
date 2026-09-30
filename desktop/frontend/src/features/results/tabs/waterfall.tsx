// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { Timings } from "../../../lib/view";
import { waterfall } from "../model";

/** DNS, connect, TLS, waiting and download of a call, as bars. */
export function Waterfall({ timings, secure }: { timings: Timings; secure: boolean }) {
  const { segments, total } = waterfall(timings, secure);
  const pct = (ms: number) => (total > 0 ? (ms / total) * 100 : 0);
  return (
    <div className="waterfall" aria-label="Timing">
      {segments.map((s) => (
        <div key={s.name} className="wf-row">
          <span className="wf-name">{s.name}</span>
          <span className="wf-track">
            {!s.skipped && <i className={`wf-bar wf-${s.name.toLowerCase()}`} style={{ left: `${pct(s.start)}%`, width: `max(2px, ${pct(s.ms)}%)` }} />}
          </span>
          <span className="wf-ms mono">{s.skipped ? `– ${s.skipped}` : `${s.ms.toFixed(1)} ms`}</span>
        </div>
      ))}
      <div className="wf-row wf-total">
        <span className="wf-name">Total</span>
        <span />
        <span className="wf-ms mono">{total.toFixed(1)} ms</span>
      </div>
    </div>
  );
}
