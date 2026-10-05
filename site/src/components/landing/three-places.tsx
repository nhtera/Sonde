// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { CodeCard, type GutterMark } from "@/components/ui/code-card";
import { Kbd } from "@/components/ui/kbd";
import { strings } from "@/content/strings";
import { ResultRow } from "./result-row";
import { Lines, Toks } from "./tokens";

const t = strings.three;

/** One file fanning out to the three places it runs: the app, CI and an agent. */
export function ThreePlaces() {
  return (
    <section id="three" aria-labelledby="three-h">
      <div className="wrap">
        <div className="sec-head reveal">
          <h2 id="three-h">{t.title}</h2>
          <p>{t.body}</p>
        </div>
        <div className="fan">
          <CodeCard
              className="reveal"
              file={t.file}
              badge={<span className="chip">{t.fileBadge}</span>}
              label={t.fileLabel}
              lines={t.checkout.map((l) => ({
                n: l.n,
                mark: ("mark" in l ? l.mark : undefined) as GutterMark,
                failed: "failed" in l ? l.failed : false,
                error: "error" in l ? l.error : undefined,
                content: <Toks line={l.line} />,
              }))}
          />
          <div className="wires" aria-hidden="true">
            <svg viewBox="0 0 64 300" preserveAspectRatio="none">
              <path d="M0 150 C32 150 32 40 64 40" />
              <path d="M0 150 H64" />
              <path d="M0 150 C32 150 32 260 64 260" />
            </svg>
          </div>
          <div className="outs">
            <div className="out reveal">
              <div className="out-head">
                <b>{t.desktop.name}</b> {t.desktop.action} <Kbd>{t.desktop.kbd}</Kbd>
                <span className="mono">{t.desktop.where}</span>
              </div>
              <div className="rows">
                {t.desktop.rows.map((r) => (
                  <ResultRow key={r.i} row={r} showTime={false} />
                ))}
              </div>
            </div>
            <div className="out reveal">
              <div className="out-head">
                <b>{t.ci.name}</b> {t.ci.action}
                <span className="mono">{t.ci.where}</span>
              </div>
              <pre tabIndex={0} aria-label={t.ci.label}>
                <Lines lines={t.ci.output} />
              </pre>
            </div>
            <div className="out reveal">
              <div className="out-head">
                <b>{t.agent.name}</b> {t.agent.action}
                <span className="mono">{t.agent.where}</span>
              </div>
              <pre tabIndex={0} aria-label={t.agent.label}>
                <Lines lines={t.agent.output} />
              </pre>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
