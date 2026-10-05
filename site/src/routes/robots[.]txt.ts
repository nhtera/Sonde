// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute } from "@tanstack/react-router";
import { absolute } from "@/lib/urls";

export const Route = createFileRoute("/robots.txt")({
  server: {
    handlers: {
      GET: () =>
        new Response(`User-agent: *\nAllow: /\n\nSitemap: ${absolute("/sitemap.xml")}\n`, {
          headers: { "content-type": "text/plain; charset=utf-8" },
        }),
    },
  },
});
