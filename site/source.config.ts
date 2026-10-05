// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { defineConfig, defineDocs } from "fumadocs-mdx/config";
import { hurlLanguage, sondeTheme } from "./src/lib/shiki";

// docs/ is the source; content/docs is a generated copy. Files are .md,
// which fumadocs-mdx compiles as Markdown (CommonMark + GFM), never MDX:
// `{…}` is text, not code, and raw HTML is not rendered.
export const docs = defineDocs({
  dir: "content/docs",
});

export default defineConfig({
  mdxOptions: {
    rehypeCodeOptions: {
      themes: { light: sondeTheme, dark: sondeTheme },
      langs: [hurlLanguage],
    },
  },
});
