// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createServerFn } from "@tanstack/react-start";
import { notFound } from "@tanstack/react-router";
import type { source as Source } from "./source";

type SerializedPageTree = Awaited<ReturnType<typeof Source.serializePageTree>>;

// What the docs route needs for one page. The page tree is the same for
// every page; it is serialized (names and icons as HTML strings) so it
// survives JSON.
export interface DocsPageData {
  path: string;
  title: string;
  description: string;
  pageTree: SerializedPageTree;
}

export interface DocsIndex {
  pageTree: SerializedPageTree;
  pages: Record<string, { path: string; title: string; description: string }>;
}

export const DOCS_INDEX_URL = "/api/docs-tree.json";

/** Build the docs index from the content source. Server only. */
export async function buildDocsIndex(): Promise<DocsIndex> {
  const { source } = await import("./source");
  const pages: DocsIndex["pages"] = {};
  for (const page of source.getPages()) {
    pages[page.slugs.join("/")] = {
      path: page.path,
      title: page.data.title ?? "",
      description: page.data.description ?? "",
    };
  }
  return { pageTree: await source.serializePageTree(source.getPageTree()), pages };
}

function pick(index: DocsIndex, slugs: string[]): DocsPageData {
  const page = index.pages[slugs.join("/")];
  if (!page) throw notFound();
  return { ...page, pageTree: index.pageTree };
}

// Runs during prerender (SSR), in-process.
const serverDocsPage = createServerFn({ method: "GET" })
  .validator((slugs: string[]) => slugs)
  .handler(async ({ data: slugs }) => pick(await buildDocsIndex(), slugs));

let clientIndex: Promise<DocsIndex> | undefined;

// In the browser the same data comes from the prerendered static index, so
// client-side navigation never calls a server function (there is none in
// production: every page is a static asset).
export async function loadDocsPage(slugs: string[]): Promise<DocsPageData> {
  if (import.meta.env.SSR) return serverDocsPage({ data: slugs });
  clientIndex ??= fetch(DOCS_INDEX_URL).then((r) => {
    if (!r.ok) throw new Error(`${DOCS_INDEX_URL}: ${r.status}`);
    return r.json() as Promise<DocsIndex>;
  });
  return pick(await clientIndex, slugs);
}
