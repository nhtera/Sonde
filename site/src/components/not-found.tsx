// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Link } from "@tanstack/react-router";
import { Logo } from "@/components/ui/logo";
import { strings } from "@/content/strings";

const t = strings.notFound;

/** The 404 page: prerendered as /404 and served by the Worker for any miss. */
export function NotFound() {
  return (
    <main id="main" className="not-found">
      <Link className="brand" to="/" aria-label={strings.nav.home}>
        <Logo />
        {strings.site.name}
      </Link>
      <p className="mono not-found-code">404</p>
      <h1>{t.title}</h1>
      <p>{t.body}</p>
      <div className="ctas">
        <Link className="btn primary" to="/">
          {t.home}
        </Link>
        <Link className="btn" to="/docs/$" params={{ _splat: "" }}>
          {t.docs}
        </Link>
      </div>
    </main>
  );
}
