// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { ListIcon, XIcon } from "@phosphor-icons/react/ssr";
import { Link } from "@tanstack/react-router";
import { useSearchContext } from "fumadocs-ui/contexts/search";
import { useEffect, useRef, useState } from "react";
import { Logo } from "@/components/ui/logo";
import { ThemeToggle } from "@/components/ui/theme-toggle";
import { strings } from "@/content/strings";
import { REPO_URL } from "@/lib/urls";

const t = strings.nav;

function SearchPill() {
  const { setOpenSearch } = useSearchContext();
  return (
    <button className="search" type="button" onClick={() => setOpenSearch(true)}>
      {t.search}{" "}
      <span className="kbd" aria-hidden="true">
        ⌘K
      </span>
    </button>
  );
}

/** Sticky header: logo, links, ⌘K search, theme; a disclosure menu under 820px. */
export function Nav() {
  const [open, setOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const sentinel = useRef<HTMLDivElement>(null);
  const menuBtn = useRef<HTMLButtonElement>(null);
  const firstLink = useRef<HTMLAnchorElement>(null);

  // Hairline under the header once the page scrolls.
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !("IntersectionObserver" in window)) return;
    const io = new IntersectionObserver(([e]) => setScrolled(!e.isIntersecting));
    io.observe(el);
    return () => io.disconnect();
  }, []);

  // Esc closes the menu and returns focus to its button.
  useEffect(() => {
    if (!open) return;
    firstLink.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        menuBtn.current?.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  const links = (first?: typeof firstLink) => (
    <>
      <Link to="/docs/$" params={{ _splat: "" }} ref={first}>
        {t.docs}
      </Link>
      <a href="#desktop">{t.desktop}</a>
      <a href={REPO_URL}>{t.github}</a>
    </>
  );

  return (
    <>
      <div ref={sentinel} aria-hidden="true" className="top-sentinel" />
      <a className="skip" href="#main">
        {t.skip}
      </a>
      <header className={scrolled ? "nav scrolled" : "nav"}>
        <div className="wrap">
          <Link className="brand" to="/" aria-label={t.home}>
            <Logo />
            {strings.site.name}
          </Link>
          <nav className="nav-links" aria-label={t.main}>
            {links()}
          </nav>
          <div className="nav-right">
            <SearchPill />
            <button
              ref={menuBtn}
              className="icon-btn menu-btn"
              type="button"
              aria-expanded={open}
              aria-controls="mnav"
              aria-label={open ? t.closeMenu : t.openMenu}
              onClick={() => setOpen((o) => !o)}
            >
              {open ? <XIcon aria-hidden="true" /> : <ListIcon aria-hidden="true" />}
            </button>
            <ThemeToggle />
          </div>
        </div>
        <div className="mnav" id="mnav" hidden={!open}>
          {/* A tap on a link closes the menu. */}
          <nav className="wrap" aria-label={t.mobile} onClick={(e) => (e.target as HTMLElement).closest("a") && setOpen(false)}>
            {links(firstLink)}
            <SearchPill />
          </nav>
        </div>
      </header>
    </>
  );
}
