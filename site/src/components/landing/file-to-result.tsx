// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Pill } from "@/components/ui/pill";
import { strings } from "@/content/strings";
import { ResultRow } from "./result-row";
import { Toks } from "./tokens";

const t = strings.loud;

/** "Calm surfaces. Loud results.": a run result built from components, not an image. */
export function FileToResult() {
  return (
    <section id="loud" aria-labelledby="loud-h">
      <div className="wrap loud">
        <div className="statement reveal">
          <h2 id="loud-h">
            {t.title} <span>{t.titleLoud}</span>
          </h2>
          <p>{t.body}</p>
        </div>
        <figure className="result reveal" aria-label={t.figureLabel}>
          <div className="result-top">
            <b>{t.file}</b>
            <Pill tone="fail">{t.failed}</Pill>
            <span className="chip" style={{ marginLeft: "auto" }}>
              {t.where}
            </span>
          </div>
          <div className="counts">
            <span className="ok">{t.counts.passed}</span>
            <span className="bad">{t.counts.failed}</span>
            <span>{t.counts.time}</span>
            <span>{t.counts.ago}</span>
          </div>
          <div className="rows">
            {t.rows.map((r) => (
              <ResultRow key={r.i} row={r} />
            ))}
          </div>
          <div className="assert">
            <h3>
              {t.assertTitle}
              <span>{t.assertLine}</span>
            </h3>
            <div className="expr">
              <Toks line={t.assertExpr} />
            </div>
            <dl>
              <dt>{t.expected}</dt>
              <dd>{t.expectedValue}</dd>
              <dt>{t.actual}</dt>
              <dd className="bad">{t.actualValue}</dd>
            </dl>
            <span className="btn demo-btn" aria-hidden="true">
              {t.goTo} <span className="kbd">{t.goToKbd}</span>
            </span>
          </div>
          <div className="passed">
            {t.passedAsserts.map((a) => (
              <div key={a.ln}>
                <span className="ok">✓</span>
                <span className="ln">{a.ln}</span>
                {a.text}
              </div>
            ))}
          </div>
        </figure>
      </div>
    </section>
  );
}
