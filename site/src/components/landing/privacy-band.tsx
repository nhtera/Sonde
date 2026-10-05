// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { strings } from "@/content/strings";

const t = strings.privacy;

/** Full-bleed band: no account, no telemetry, local history, optional update check. */
export function PrivacyBand() {
  return (
    <div className="band" aria-labelledby="priv-h">
      <div className="wrap band-grid">
        <h2 id="priv-h" className="reveal">
          {t.title}
        </h2>
        {t.items.map((item) => (
          <div key={item.title} className="reveal">
            <b>{item.title}</b>
            <p>{item.body}</p>
          </div>
        ))}
      </div>
    </div>
  );
}
