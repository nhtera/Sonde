// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Link } from "@tanstack/react-router";
import type { CSSProperties } from "react";
import desktop from "../../../content/generated/desktop.json";
import { WindowFrame } from "@/components/ui/window-frame";
import { strings } from "@/content/strings";
import { InstallTabs } from "./install-tabs";
import { ThemedShot } from "./themed-shot";
import { Toks } from "./tokens";

const t = strings.hero;
const delay = (i: number) => ({ "--i": i }) as CSSProperties;

export function Hero() {
  return (
    <div className="hero">
      <div className="wrap">
        <div className="hero-copy">
          <div className="head">
            <p className="eyebrow reveal" style={delay(0)}>
              {t.eyebrow}
            </p>
            <h1 className="reveal" style={delay(1)}>
              {t.title}
            </h1>
          </div>
          <p className="sub reveal" style={delay(2)}>
            <Toks line={t.sub} />
          </p>
          <div className="install reveal" style={delay(3)}>
            <InstallTabs />
            <div className="ctas">
              <a className="btn primary" href={desktop.url}>
                {t.download}
              </a>
              <Link className="btn" to="/docs/$" params={{ _splat: "" }}>
                {t.readDocs}
              </Link>
            </div>
          </div>
        </div>
        <WindowFrame className="reveal" style={delay(2)}>
          <ThemedShot id="hero" alt={t.shotAlt} altLight={t.shotAltLight} eager sizes="(max-width: 1200px) calc(100vw - 48px), 1152px" />
        </WindowFrame>
      </div>
    </div>
  );
}
