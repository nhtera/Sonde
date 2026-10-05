// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Remark plugin for the docs: links written for GitHub become site links,
// and content from a docs pull request cannot inject anything.
//
// - A link to a published doc → /docs/<slug>#anchor.
// - A link to any other repository file (or a folder) → its GitHub page.
// - `#anchor` stays; https: and mailto: stay; any other scheme fails.
// - A link to a file that does not exist fails, naming the doc and link.
// - Raw HTML (and HTML comments) is dropped; images fail.
// Code is never touched: link nodes do not occur inside code.

import { existsSync, statSync } from "node:fs";
import { posix, relative, sep } from "node:path";
import type { Definition, Link, Nodes, Parent, Root } from "mdast";
import { docUrl, githubUrl, sourceRelOf, splitAnchor } from "./doc-paths.ts";

export interface SondeLinksOptions {
  /** Absolute path of the repository root. */
  repoRoot: string;
  /** Absolute path of the generated content/docs folder. */
  contentDir: string;
  /** Published docs, relative to docs/ (`guides/openapi.md`). */
  published: Set<string>;
}

const ALLOWED_SCHEMES = new Set(["https:", "mailto:"]);

interface VFileLike {
  path?: string;
  history?: string[];
}

/** Rewrite one URL found in the doc at `sourceRel` (relative to docs/). */
export function rewriteUrl(url: string, sourceRel: string, opts: SondeLinksOptions): string {
  const where = `docs/${sourceRel}: link "${url}"`;
  if (url.startsWith("#")) return url;
  const scheme = url.match(/^([a-z][a-z0-9+.-]*):/i)?.[1];
  if (scheme || url.startsWith("//")) {
    if (scheme && ALLOWED_SCHEMES.has(`${scheme.toLowerCase()}:`)) return url;
    throw new Error(`${where}: only https: and mailto: links are allowed`);
  }
  const { path, anchor } = splitAnchor(url);
  const repoPath = posix.normalize(posix.join("docs", posix.dirname(sourceRel), decodeURI(path)));
  if (repoPath.startsWith("../") || repoPath === "..") throw new Error(`${where}: points outside the repository`);
  const abs = `${opts.repoRoot}/${repoPath}`;
  if (!existsSync(abs)) throw new Error(`${where}: ${repoPath} does not exist`);
  const dir = statSync(abs).isDirectory();
  if (!dir && repoPath.startsWith("docs/")) {
    const rel = repoPath.slice("docs/".length);
    if (opts.published.has(rel)) return docUrl(rel, anchor);
  }
  return githubUrl(repoPath.replace(/\/$/, ""), { anchor, dir });
}

function visit(node: Nodes, sourceRel: string, opts: SondeLinksOptions): void {
  if (!("children" in node)) return;
  const parent = node as Parent;
  for (let i = parent.children.length - 1; i >= 0; i--) {
    const child = parent.children[i] as Nodes;
    switch (child.type) {
      case "html":
        parent.children.splice(i, 1);
        continue;
      case "image":
      case "imageReference":
        throw new Error(`docs/${sourceRel}: images are not supported on the site yet`);
      case "link":
      case "definition":
        (child as Link | Definition).url = rewriteUrl((child as Link | Definition).url, sourceRel, opts);
        break;
    }
    visit(child, sourceRel, opts);
  }
}

/** Transform a parsed doc. `contentFile` is the absolute path of the file in content/docs. */
export function transformDoc(tree: Root, contentFile: string, opts: SondeLinksOptions): void {
  const contentRel = relative(opts.contentDir, contentFile).split(sep).join("/");
  visit(tree, sourceRelOf(contentRel), opts);
}

export default function remarkSondeLinks(opts: SondeLinksOptions) {
  return (tree: Root, file: VFileLike) => {
    const path = file.path ?? file.history?.at(-1);
    // Only generated docs are rewritten; anything else compiled with this
    // config is not a repository doc.
    if (!path || !path.startsWith(opts.contentDir)) return;
    transformDoc(tree, path, opts);
  };
}
