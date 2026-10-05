// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { strings } from "@/content/strings";
import { doc } from "@/lib/urls";
import { ShotCrop } from "./themed-shot";
import { Lines, Toks } from "./tokens";

const t = strings.features;

function Cell({ className, title, body, more, href, children }: { className: string; title: string; body: string; more?: string; href?: string; children?: ReactNode }) {
  return (
    <article className={`cell ${className} reveal`}>
      <h3>{title}</h3>
      <p>{body}</p>
      {children}
      {more && href ? (
        <Link className="more" to={href}>
          {more} →
        </Link>
      ) : null}
    </article>
  );
}

/** Eight features in an interlocking bento; each links to its docs. */
export function FeatureGrid() {
  return (
    <section id="features" aria-labelledby="feat-h">
      <div className="wrap">
        <div className="sec-head reveal">
          <h2 id="feat-h">{t.title}</h2>
          <p>{t.body}</p>
        </div>
        <div className="bento">
          <Cell className="c-git" title={t.git.title} body={t.git.body} more={t.git.more} href={doc("file-format")}>
            <div className="vis diff">
              <Lines lines={t.git.diff} />
            </div>
          </Cell>
          <Cell className="c-env" title={t.env.title} body={t.env.body} more={t.env.more} href={doc("sonde-yaml")}>
            <ShotCrop id="env" label={t.env.shotLabel} />
          </Cell>
          <Cell className="c-oas" title={t.oas.title} body={t.oas.body} more={t.oas.more} href={doc("guides/openapi")}>
            <ShotCrop id="coverage" label={t.oas.shotLabel} />
          </Cell>
          <Cell className="c-data" title={t.data.title} body={t.data.body} more={t.data.more} href={doc("guides/data-driven")}>
            <div className="vis">
              <Lines lines={t.data.rows} />
            </div>
          </Cell>
          <Cell className="c-stream" title={t.stream.title} body={t.stream.body} more={t.stream.more} href={doc("guides/streaming")}>
            <div className="vis">
              <Lines lines={t.stream.log} />
            </div>
          </Cell>
          <Cell className="c-imp" title={t.imports.title} body={t.imports.body} more={t.imports.more} href={doc("guides/import-export")}>
            <div className="vis">
              <Lines lines={t.imports.sample} />
            </div>
          </Cell>
          <Cell className="c-lsp" title={t.lsp.title} body={t.lsp.body} more={t.lsp.more} href={doc("guides/editors")}>
            <div className="vis">
              <Toks line={t.lsp.typed} />
              {"\n          "}
              <span className="lsp-pop">
                {t.lsp.completions.map((c) => (
                  <div key={c.name} className={c.on ? "on" : undefined}>
                    {c.name} <span className="muted">{c.detail}</span>
                  </div>
                ))}
              </span>
            </div>
          </Cell>
          <article className="cell c-rep reveal">
            <div>
              <h3>{t.reports.title}</h3>
              <p>{t.reports.body}</p>
            </div>
            <div className="fmts">
              {t.reports.formats.map((f) => (
                <span key={f} className="chip">
                  {f}
                </span>
              ))}
            </div>
          </article>
        </div>
      </div>
    </section>
  );
}
