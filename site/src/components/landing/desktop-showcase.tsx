// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useRef, useState, useSyncExternalStore, type KeyboardEvent } from "react";
import desktop from "../../../content/generated/desktop.json";
import { ButtonLink } from "@/components/ui/button";
import { strings } from "@/content/strings";
import { nextTab } from "./tab-keys";
import { ThemedShot, type ShotId } from "./themed-shot";

const t = strings.showcase;

/** Whether a media query matches; the prerendered page assumes the desktop layout. */
function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const m = matchMedia(query);
      m.addEventListener("change", onChange);
      return () => m.removeEventListener("change", onChange);
    },
    () => matchMedia(query).matches,
    () => true,
  );
}

/** Real app screens in vertical tabs (a row of tabs under 980px); arrow keys, Home and End move between them. */
export function DesktopShowcase() {
  const [current, setCurrent] = useState(0);
  const vertical = useMediaQuery("(min-width: 981px)");
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);
  const onKey = (e: KeyboardEvent, i: number) => {
    const n = nextTab(e, i, t.tabs.length);
    if (n === undefined) return;
    e.preventDefault();
    setCurrent(n);
    tabs.current[n]?.focus();
  };
  return (
    <section id="desktop" aria-labelledby="desk-h">
      <div className="wrap">
        <div className="sec-head reveal">
          <h2 id="desk-h">{t.title}</h2>
          <p>{t.body}</p>
        </div>
        <div className="show">
          <div className="show-tabs" role="tablist" aria-label={t.tabsLabel} aria-orientation={vertical ? "vertical" : "horizontal"}>
            {t.tabs.map((tab, i) => (
              <button
                key={tab.id}
                ref={(el) => {
                  tabs.current[i] = el;
                }}
                role="tab"
                id={`t-${tab.id}`}
                aria-selected={i === current}
                aria-controls={`p-${tab.id}`}
                tabIndex={i === current ? 0 : -1}
                onClick={() => setCurrent(i)}
                onKeyDown={(e) => onKey(e, i)}
              >
                {tab.title}
                <span>{tab.sub}</span>
              </button>
            ))}
          </div>
          <div>
            {t.tabs.map((tab, i) => (
              <div key={tab.id} className="show-panel frame" id={`p-${tab.id}`} role="tabpanel" aria-labelledby={`t-${tab.id}`} hidden={i !== current}>
                <ThemedShot id={tab.id as ShotId} alt={tab.alt} sizes="(max-width: 980px) calc(100vw - 48px), 840px" />
              </div>
            ))}
            <div className="show-cta">
              <ButtonLink variant="primary" href={desktop.url}>
                {t.download}
              </ButtonLink>
              <small>{t.note}</small>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
