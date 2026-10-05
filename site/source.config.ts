// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { resolve } from "node:path";
import { pageSchema } from "fumadocs-core/source/schema";
import { defineConfig, defineDocs } from "fumadocs-mdx/config";
import { z } from "zod";
import { loadNav, publishedSet } from "./src/lib/docs-nav.ts";
import remarkSondeLinks from "./src/lib/remark-sonde-links.ts";
import { hurlLanguage, sondeTheme } from "./src/lib/shiki.ts";

// docs/ is the source; content/docs is the copy `npm run sync` writes. Files
// are .md, which fumadocs-mdx compiles as Markdown (CommonMark + GFM), never
// MDX: `{…}` is text, not code, and raw HTML is not rendered.
// Paths are relative to site/ (npm scripts and Vite run there).
const siteRoot = process.cwd();
const repoRoot = resolve(siteRoot, "..");
const contentDir = resolve(siteRoot, "content/docs");

export const docs = defineDocs({
  dir: "content/docs",
  docs: {
    schema: pageSchema.extend({
      description: z.string().min(1),
      // The repository path the page comes from; the Edit link is built from it.
      source: z.string().regex(/^docs\/[a-z0-9_/.-]+\.md$|^docs\/(?:[a-z0-9_-]+\/)*README\.md$/),
      lastUpdated: z.string().regex(/^\d{4}-\d{2}-\d{2}$/).optional(),
    }),
  },
});

export default defineConfig({
  mdxOptions: {
    remarkPlugins: (defaults) => [
      [remarkSondeLinks, { repoRoot, contentDir, published: publishedSet(loadNav(repoRoot)) }],
      ...defaults,
    ],
    rehypeCodeOptions: {
      themes: { light: sondeTheme, dark: sondeTheme },
      langs: [hurlLanguage],
    },
  },
});
