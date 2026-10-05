// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createRootRoute, HeadContent, Outlet, Scripts } from "@tanstack/react-router";
import { RootProvider } from "fumadocs-ui/provider/tanstack";
import { lazy, type ReactNode } from "react";
import { JS_CLASS_SCRIPT } from "@/components/landing/reveal";
import { strings } from "@/content/strings";
import { fontPreloads } from "@/styles/fonts";
import globalCss from "@/styles/global.css?url";

// The search dialog (and its index client) loads when first opened, not with the page.
const SearchDialog = lazy(() => import("@/components/search"));

export const Route = createRootRoute({
  head: ({ matches }) => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      // The not-found state (a docs link to a page that does not exist, on
      // client navigation) gets the 404 page's title and noindex.
      ...(matches.some((m) => m.status === "notFound" || ("globalNotFound" in m && m.globalNotFound))
        ? [{ title: strings.notFound.metaTitle }, { name: "robots", content: "noindex" }]
        : [{ title: strings.site.title }, { name: "description", content: strings.site.description }]),
    ],
    links: [
      ...fontPreloads,
      { rel: "stylesheet", href: globalCss },
      { rel: "icon", href: "/favicon.svg", type: "image/svg+xml" },
    ],
    // Before first paint: lets CSS hide not-yet-revealed sections only when
    // a script will reveal them.
    scripts: [{ children: JS_CLASS_SCRIPT }],
  }),
  component: RootComponent,
});

function RootComponent() {
  return (
    <RootDocument>
      <Outlet />
    </RootDocument>
  );
}

function RootDocument({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <HeadContent />
      </head>
      <body>
        <RootProvider
          search={{ SearchDialog, preload: false }}
          // One toggle drives Fumadocs (.dark) and the desktop tokens
          // (data-theme). Dark is the default; the OS setting is ignored
          // until the reader picks.
          theme={{ attribute: ["class", "data-theme"], defaultTheme: "dark", enableSystem: false, storageKey: "sonde-site-theme" }}
        >
          {children}
        </RootProvider>
        <Scripts />
      </body>
    </html>
  );
}
