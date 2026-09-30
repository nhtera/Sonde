// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A body rendered as the browser would: HTML in a frame without scripts,
// forms or remote loads (an empty sandbox, and the body's own CSP
// sandbox), images as images, PDFs where the webview shows them.

import type { BodyKind } from "../../model";
import { bodyURL } from "./body-fetch";

export function Preview({ id, kind, onOpenExternally }: { id: string; kind: BodyKind; onOpenExternally?(): void }) {
  if (kind === "image") {
    return (
      <div className="preview preview-image">
        <img src={bodyURL(id)} alt="Response image" />
      </div>
    );
  }
  if (kind === "html" || kind === "pdf") {
    return (
      <div className="preview">
        <p className="preview-note">🔒 Sandboxed preview · scripts, forms and remote loads are blocked</p>
        {/* An empty sandbox: no scripts, no same origin, no forms. */}
        <iframe className="preview-frame" sandbox="" referrerPolicy="no-referrer" src={bodyURL(id)} title="Response preview" />
        {kind === "pdf" && onOpenExternally && (
          <p className="preview-note">
            Blank? <button className="btn-ghost" onClick={onOpenExternally}>Open in default app</button>
          </p>
        )}
      </div>
    );
  }
  return <div className="body-note">No preview for this type: use Pretty or Raw.</div>;
}
