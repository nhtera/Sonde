// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { cloudflare } from "@cloudflare/vite-plugin";
import tailwindcss from "@tailwindcss/vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import react from "@vitejs/plugin-react";
import { fumadocsMdx } from "fumadocs-mdx/vite";
import { defineConfig } from "vite";

export default defineConfig({
  server: { port: 3000 },
  plugins: [
    fumadocsMdx(),
    tailwindcss(),
    cloudflare({ viteEnvironment: { name: "ssr" } }),
    tanstackStart({
      prerender: {
        enabled: true,
        crawlLinks: true,
        failOnError: false,
      },
      // Routes no page links to: the search index, the docs index and the 404 page.
      pages: [{ path: "/api/search.json" }, { path: "/api/docs-tree.json" }, { path: "/404" }],
    }),
    react(),
  ],
  resolve: {
    tsconfigPaths: true,
  },
});
