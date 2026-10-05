// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute } from "@tanstack/react-router";
import { buildDocsIndex } from "@/lib/docs-data";
import { absolute, doc } from "@/lib/urls";

// Prerendered: the landing page and every docs page, canonical URLs only.
export const Route = createFileRoute("/sitemap.xml")({
  server: {
    handlers: {
      GET: async () => {
        const { pages } = await buildDocsIndex();
        const urls = ["/", ...Object.keys(pages).sort().map((slug) => doc(slug))];
        const body =
          `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n` +
          urls.map((u) => `  <url><loc>${absolute(u)}</loc></url>`).join("\n") +
          `\n</urlset>\n`;
        return new Response(body, { headers: { "content-type": "application/xml; charset=utf-8" } });
      },
    },
  },
});
