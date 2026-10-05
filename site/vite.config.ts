// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { cloudflare } from "@cloudflare/vite-plugin";
import tailwindcss from "@tailwindcss/vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import react from "@vitejs/plugin-react";
import { fumadocsMdx } from "fumadocs-mdx/vite";
import { randomUUID } from "node:crypto";
import { defineConfig } from "vite";

// Prerender requests carry this per-build token; the deployed Worker renders
// nothing else (src/server.ts). Kept in the environment so every load of this
// config in one build sees the same value.
const prerenderToken = (process.env.SONDE_PRERENDER_TOKEN ??= randomUUID());

export default defineConfig({
  server: { port: 3000 },
  define: { __SONDE_PRERENDER_TOKEN__: JSON.stringify(prerenderToken) },
  plugins: [
    fumadocsMdx(),
    tailwindcss(),
    cloudflare({ viteEnvironment: { name: "ssr" } }),
    tanstackStart({
      prerender: {
        enabled: true,
        crawlLinks: true,
        failOnError: true,
        headers: { "x-sonde-prerender": prerenderToken },
        // A link with an anchor is the same page; render it once.
        filter: (page) => !page.path.includes("#"),
      },
      // Routes no page links to: the search and docs indexes, the 404 page, sitemap and robots.
      pages: [{ path: "/api/search.json" }, { path: "/api/docs-tree.json" }, { path: "/404.html" }, { path: "/sitemap.xml" }, { path: "/robots.txt" }],
    }),
    react(),
  ],
  resolve: {
    tsconfigPaths: true,
  },
});
