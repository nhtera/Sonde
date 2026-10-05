// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute } from "@tanstack/react-router";
import browserCollections from "collections/browser";
import { useFumadocsLoader } from "fumadocs-core/source/client";
import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { DocsBody, DocsDescription, DocsPage, DocsTitle } from "fumadocs-ui/layouts/docs/page";
import { TOC, TOCProvider } from "fumadocs-ui/layouts/docs/page/slots/toc";
import { Suspense } from "react";
import { IndexCards } from "@/components/docs/index-cards";
import { PageFooter } from "@/components/docs/page-footer";
import { TocPopover } from "@/components/docs/toc-popover";
import { getMDXComponents } from "@/components/mdx";
import { strings } from "@/content/strings";
import { loadDocsPage } from "@/lib/docs-data";
import { baseOptions } from "@/lib/layout.shared";
import { pageHead } from "@/lib/seo";
import { doc } from "@/lib/urls";

export const Route = createFileRoute("/docs/$")({
  component: Page,
  loader: async ({ params }) => {
    const slugs = params._splat?.split("/").filter(Boolean) ?? [];
    const data = await loadDocsPage(slugs);
    await clientLoader.preload(data.path);
    return { ...data, slug: slugs.join("/") };
  },
  head: ({ loaderData }) =>
    loaderData
      ? pageHead({
          title: loaderData.slug ? `${loaderData.title}${strings.docs.titleSuffix}` : `${loaderData.title} · Sonde`,
          description: loaderData.description,
          path: doc(loaderData.slug),
        })
      : {},
});

const clientLoader = browserCollections.docs.createClientLoader({
  component({ toc, frontmatter, default: MDX }, { tree }: { tree?: Parameters<typeof IndexCards>[0]["tree"] }) {
    return (
      <DocsPage toc={toc} tableOfContent={{ container: { role: "complementary", "aria-label": strings.docs.toc } }}
        slots={{ toc: { provider: TOCProvider, main: TOC, popover: TocPopover } }}
      >
        <DocsTitle>{frontmatter.title}</DocsTitle>
        <DocsDescription>{frontmatter.description}</DocsDescription>
        <DocsBody>
          <MDX components={getMDXComponents()} />
          {tree ? <IndexCards tree={tree} /> : null}
        </DocsBody>
        <PageFooter source={frontmatter.source} lastUpdated={frontmatter.lastUpdated} />
      </DocsPage>
    );
  },
});

function Page() {
  const { pageTree, path } = useFumadocsLoader(Route.useLoaderData());
  return (
    <DocsLayout {...baseOptions()} tree={pageTree}>
      <Suspense>{clientLoader.useContent(path, path === "index.md" ? { tree: pageTree } : {})}</Suspense>
    </DocsLayout>
  );
}
