// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { strings } from "@/content/strings";
import { Lines } from "./tokens";

const t = strings.quickstart;

/** Three steps beside one terminal session. */
export function Quickstart() {
  return (
    <section id="start" aria-labelledby="start-h">
      <div className="wrap">
        <div className="sec-head reveal">
          <h2 id="start-h">{t.title}</h2>
        </div>
        <div className="qs">
          <ol className="qs-steps">
            {t.steps.map((s) => (
              <li key={s.title} className="reveal">
                <h3>{s.title}</h3>
                <p>{s.body}</p>
              </li>
            ))}
          </ol>
          <pre className="term reveal" tabIndex={0} aria-label={t.terminalLabel}>
            <Lines lines={t.terminal} />
          </pre>
        </div>
      </div>
    </section>
  );
}
