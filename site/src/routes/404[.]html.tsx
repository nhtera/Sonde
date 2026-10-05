// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute } from "@tanstack/react-router";
import { NotFound } from "@/components/not-found";
import { strings } from "@/content/strings";

// Prerendered as 404.html at the root of the assets: Cloudflare serves it,
// with status 404, for any path that has no asset.
export const Route = createFileRoute("/404.html")({
  head: () => ({ meta: [{ title: strings.notFound.metaTitle }, { name: "robots", content: "noindex" }] }),
  component: NotFound,
});
