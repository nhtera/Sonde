// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import "./theme-preview.css";

/** A small drawing of the app in a theme: [data-theme] sets the theme's
 * tokens on any element, so it shows next to another theme's. */
export function ThemePreview({ id }: { id: string }) {
  return (
    <div className="theme-preview" data-theme={id} aria-hidden>
      <div className="tp-side">
        <b />
        <b className="on" />
        <b />
      </div>
      <div className="tp-main">
        <div className="tp-url">
          <span className="tp-method">POST</span>
          <b />
          <span className="tp-send" />
        </div>
        <div className="tp-line">
          <b className="key" />
          <b className="str" />
        </div>
        <div className="tp-line">
          <b className="key short" />
          <b className="num" />
          <b className="var" />
        </div>
        <div className="tp-line">
          <b className="fail" />
        </div>
      </div>
    </div>
  );
}
