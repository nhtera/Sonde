// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from "react";

export type GutterMark = "pass" | "fail" | "capture" | "request" | "warn" | undefined;

export interface CodeLine {
  n: number;
  mark?: GutterMark;
  failed?: boolean;
  content: ReactNode;
  /** Inline message after the line, e.g. `got "pending"`. */
  error?: string;
}

const MARK: Record<Exclude<GutterMark, undefined>, { glyph: string; className: string; label: string }> = {
  pass: { glyph: "✓", className: "g ok", label: "passed" },
  fail: { glyph: "✕", className: "g bad", label: "failed" },
  capture: { glyph: "◆", className: "g t-var", label: "captured" },
  request: { glyph: "▸", className: "g", label: "" },
  warn: { glyph: "⚠", className: "g warn", label: "warning" },
};

/** A source file as the editor shows it: line numbers, gutter marks, the failed-line wash. */
export function CodeCard({ file, badge, lines, label }: { file: string; badge?: ReactNode; lines: CodeLine[]; label: string }) {
  return (
    <div className="file">
      <div className="file-head">
        <span className="mono">{file}</span>
        {badge}
      </div>
      <pre className="code" tabIndex={0} aria-label={label}>
        {lines.map((l) => {
          const m = l.mark ? MARK[l.mark] : undefined;
          return (
            <span key={l.n} className={l.failed ? "l failed" : "l"}>
              <span className="n">{l.n}</span>
              <span className={m?.className ?? "g"}>
                {m?.glyph}
                {m?.label ? <span className="sr"> {m.label}</span> : null}
              </span>
              {l.content}
              {l.error ? <span className="inline-err">{l.error}</span> : null}
            </span>
          );
        })}
      </pre>
    </div>
  );
}
