// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute } from "@tanstack/react-router";
import { NotFound } from "@/components/not-found";

// Prerendered as 404/index.html: the page served, with status 404, for any
// path that has no asset.
export const Route = createFileRoute("/404")({
  head: () => ({ meta: [{ title: "Page not found · Sonde" }, { name: "robots", content: "noindex" }] }),
  component: NotFound,
});
