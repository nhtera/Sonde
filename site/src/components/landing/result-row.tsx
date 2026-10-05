// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { CaptureChip } from "@/components/ui/capture-chip";
import { MethodTag, type Method } from "@/components/ui/method-tag";
import { strings } from "@/content/strings";

export interface RowData {
  i: string;
  method: string;
  path: string;
  chips: readonly { name: string; dir: string }[];
  status: string;
  ms?: string;
  ok: boolean;
}

/** A request row of a run: n · METHOD · path + capture chips · status · time · ✓/✕. */
export function ResultRow({ row, showTime = true }: { row: RowData; showTime?: boolean }) {
  return (
    <div className={row.ok ? "row" : "row failed"}>
      <span className="i">{row.i}</span>
      <MethodTag method={row.method as Method} />
      <span>
        {row.path}{" "}
        {row.chips.map((c) => (
          <CaptureChip key={c.dir + c.name} name={c.name} direction={c.dir === "out" ? "out" : "in"} />
        ))}
      </span>
      <span className="ok">{row.status}</span>
      {showTime && row.ms ? <span className="muted">{row.ms}</span> : null}
      <span className={row.ok ? "ok" : "bad"} title={row.ok ? strings.loud.passed : strings.loud.failedLabel}>
        {row.ok ? "✓" : "✕"}
        <span className="sr">{row.ok ? strings.loud.passed : strings.loud.failedLabel}</span>
      </span>
    </div>
  );
}
