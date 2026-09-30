// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// "Edited since this run · line 16 · Re-run": the file's text is no longer
// the one its results came from (a Send is refused when a request before
// the one sent changed).

/** The first line where two texts differ (1-based), or 0 when equal. */
export function firstChangedLine(a: string, b: string): number {
  if (a === b) return 0;
  let i = 0;
  const n = Math.min(a.length, b.length);
  while (i < n && a.charCodeAt(i) === b.charCodeAt(i)) i++;
  let line = 1;
  for (let k = 0; k < i; k++) if (a.charCodeAt(k) === 10) line++;
  return line;
}

/** line: the first edited line (firstChangedLine). */
export function StaleBanner({ line, onRun }: { line: number; onRun(): void }) {
  if (!line) return null;
  return (
    <div className="stale-banner" role="status">
      <span>
        Edited since this run · line {line}
      </span>
      <button className="btn" onClick={onRun}>
        Re-run
      </button>
    </div>
  );
}
