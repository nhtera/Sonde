// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { Failure } from "./model";

export interface FailureBoxProps {
  failure: Failure;
  onGoto?(line: number): void;
  onShowResponse?(): void;
  /** The keys of Go to line ("⌘G"), shown on its button. */
  gotoKeys?: string;
}

/** A failed check explained: the line, expected and actual values. */
export function FailureBox({ failure: f, onGoto, onShowResponse, gotoKeys }: FailureBoxProps) {
  return (
    <section className="failure-box" role="alert">
      <div className="failure-title">
        <b>{f.title}</b>
        <span>line {f.line}</span>
      </div>
      {f.code && <code className="failure-code mono">{f.code}</code>}
      {f.expected !== undefined || f.actual !== undefined ? (
        <dl className="failure-values">
          {f.expected !== undefined && (
            <>
              <dt>Expected</dt>
              <dd className="mono">{f.expected}</dd>
            </>
          )}
          {f.actual !== undefined && (
            <>
              <dt>Actual</dt>
              <dd className="mono fail">{f.actual}</dd>
            </>
          )}
        </dl>
      ) : (
        <p className="failure-message">{f.message}</p>
      )}
      {(onGoto || onShowResponse) && (
        <div className="failure-actions">
          {onGoto && (
            <button className="btn" onClick={() => onGoto(f.line)}>
              Go to line {f.line} {gotoKeys && <kbd>{gotoKeys}</kbd>}
            </button>
          )}
          {onShowResponse && (
            <button className="btn-ghost" onClick={onShowResponse}>
              Show response
            </button>
          )}
        </div>
      )}
    </section>
  );
}
