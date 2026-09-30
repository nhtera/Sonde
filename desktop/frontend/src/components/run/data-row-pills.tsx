// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

export interface DataRow {
  row: number;
  /** What names the row ("Grace Hopper"); the row number is shown before it. */
  label?: string;
  state: "passed" | "failed" | "running" | "pending";
}

export interface DataRowPillsProps {
  rows: DataRow[];
  selected?: number;
  onSelect?(row: number): void;
}

/** One pill per data row of a data-driven run. */
export function DataRowPills({ rows, selected, onSelect }: DataRowPillsProps) {
  return (
    <div className="row-pills" role="tablist" aria-label="Data rows">
      {rows.map((r) => (
        <button
          key={r.row}
          role="tab"
          className={`row-pill state-${r.state}`}
          aria-selected={selected === r.row}
          onClick={() => onSelect?.(r.row)}
        >
          <i className="dot" />
          Row {r.row}
          {r.label && ` · ${r.label}`}
        </button>
      ))}
    </div>
  );
}
