// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A body too large to format (over 20 MB) or to show at all (over 50 MB):
// what it is, and the ways to get at it.

import { formatBytes, VIEW_LIMIT } from "./model";

export interface LargeBodyProps {
  size: number;
  contentType: string;
  onSave?(): void;
  onOpen?(): void;
  /** Formats it anyway (under 50 MB only). */
  onFormat?(): void;
  saveKeys?: string;
}

export function LargeBody({ size, contentType, onSave, onOpen, onFormat, saveKeys }: LargeBodyProps) {
  const viewable = size <= VIEW_LIMIT;
  return (
    <section className="large-body" aria-label="Large response">
      <div className="large-title">
        <b>{formatBytes(size)} response</b>
        <span className="mono">{contentType || "no content type"}</span>
      </div>
      <p>
        {viewable
          ? "Too large to format. Showing the first 1 MB as raw text. Asserts and captures ran on the full body."
          : "Too large to show here. Save it or open it in another app. Asserts and captures ran on the full body."}
      </p>
      <div className="large-actions">
        {onSave && (
          <button className="btn" onClick={onSave}>
            Save response to file… {saveKeys && <kbd>{saveKeys}</kbd>}
          </button>
        )}
        {onOpen && (
          <button className="btn" onClick={onOpen}>
            Open in default app
          </button>
        )}
        {viewable && onFormat && (
          <button className="btn-ghost" onClick={onFormat}>
            Format anyway
          </button>
        )}
      </div>
    </section>
  );
}
