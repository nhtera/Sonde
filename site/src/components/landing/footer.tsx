// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { GithubLogoIcon } from "@phosphor-icons/react/ssr";
import { Link } from "@tanstack/react-router";
import { Logo } from "@/components/ui/logo";
import { strings } from "@/content/strings";
import { doc, REPO_URL, repoFile } from "@/lib/urls";

const t = strings.footer;

function Column({ title, links }: { title: string; links: { label: string; href: string }[] }) {
  return (
    <nav aria-label={title}>
      <h2 className="foot-h">{title}</h2>
      <ul>
        {links.map((l) => (
          <li key={l.href}>{l.href.startsWith("/") ? <Link to={l.href}>{l.label}</Link> : <a href={l.href}>{l.label}</a>}</li>
        ))}
      </ul>
    </nav>
  );
}

export function Footer() {
  return (
    <footer className="site-footer">
      <div className="wrap">
        <div className="foot">
          <div className="foot-brand">
            <Link className="brand" to="/">
              <Logo size={22} />
              {strings.site.name}
            </Link>
            <p>{t.tagline}</p>
            <a className="foot-gh" href={REPO_URL}>
              <GithubLogoIcon aria-hidden="true" />
              {t.star}
            </a>
          </div>
          <Column
            title={t.docs.title}
            links={[
              { label: t.docs.links.start, href: doc("getting-started") },
              { label: t.docs.links.desktop, href: doc("desktop") },
              { label: t.docs.links.guides, href: doc("") },
              { label: t.docs.links.cli, href: doc("cli") },
            ]}
          />
          <Column
            title={t.project.title}
            links={[
              { label: t.project.links.releases, href: `${REPO_URL}/releases` },
              { label: t.project.links.changelog, href: `${REPO_URL}/releases/latest` },
              { label: t.project.links.contributing, href: repoFile("CONTRIBUTING.md") },
            ]}
          />
          <Column
            title={t.trust.title}
            links={[
              { label: t.trust.links.security, href: doc("security") },
              { label: t.trust.links.stability, href: doc("stability") },
              { label: t.trust.links.trademarks, href: repoFile("TRADEMARKS.md") },
            ]}
          />
        </div>
        <div className="foot-legal">
          <span>{t.legal}</span>
          <span className="mono">{strings.site.domain}</span>
        </div>
      </div>
    </footer>
  );
}
