// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createRootRoute, HeadContent, Outlet, Scripts } from "@tanstack/react-router";
import { RootProvider } from "fumadocs-ui/provider/tanstack";
import type { ReactNode } from "react";
import SearchDialog from "@/components/search";
import { strings } from "@/content/strings";
import { fontPreloads } from "@/styles/fonts";
import globalCss from "@/styles/global.css?url";

export const Route = createRootRoute({
  head: () => ({
    meta: [
      { charSet: "utf-8" },
      { name: "viewport", content: "width=device-width, initial-scale=1" },
      { title: strings.site.title },
      { name: "description", content: strings.site.description },
    ],
    links: [
      ...fontPreloads,
      { rel: "stylesheet", href: globalCss },
      { rel: "icon", href: "/favicon.svg", type: "image/svg+xml" },
    ],
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
          search={{ SearchDialog }}
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
