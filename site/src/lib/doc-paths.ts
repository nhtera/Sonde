// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Paths of docs in the repository (`docs/guides/openapi.md`), in the
// generated content (`guides/openapi.md`) and on the site
// (`/docs/guides/openapi`). Pure functions: the sync script, the links
// plugin and the tests share them. Imports carry the .ts extension so Node
// can run them without a build step.

import { posix } from "node:path";
import { doc, REPO_URL } from "./urls.ts";

/** A doc file name: lowercase letters, digits, `-` and `_`. README.md is the folder index. */
export const DOC_NAME = /^[a-z0-9_-]+\.md$/;

/** Repository paths of docs are validated before they reach a URL. */
export const SOURCE_PATH = /^docs\/[a-z0-9_/.-]+\.md$|^docs\/(?:[a-z0-9_-]+\/)*README\.md$/;

export function isReadme(path: string): boolean {
  return posix.basename(path) === "README.md";
}

/** Throws unless every segment of a docs-relative path is a valid name. */
export function checkDocName(rel: string): void {
  const segments = rel.split("/");
  const file = segments.pop() ?? "";
  for (const dir of segments) {
    if (!/^[a-z0-9_-]+$/.test(dir)) throw new Error(`docs/${rel}: folder name "${dir}" must match [a-z0-9_-]+`);
  }
  if (file !== "README.md" && !DOC_NAME.test(file)) {
    throw new Error(`docs/${rel}: file name must match ${DOC_NAME} (or be README.md)`);
  }
}

/** `guides/openapi.md` → `guides/openapi`; `README.md` → ``; `cli/README.md` → `cli`. */
export function slugOf(rel: string): string {
  if (isReadme(rel)) {
    const dir = posix.dirname(rel);
    return dir === "." ? "" : dir;
  }
  return rel.replace(/\.md$/, "");
}

/** Where a doc lands in content/docs: README.md becomes the folder's index.md. */
export function contentPathOf(rel: string): string {
  if (isReadme(rel)) {
    const dir = posix.dirname(rel);
    return dir === "." ? "index.md" : `${dir}/index.md`;
  }
  return rel;
}

/** The reverse of contentPathOf. */
export function sourceRelOf(contentRel: string): string {
  if (posix.basename(contentRel) === "index.md") {
    const dir = posix.dirname(contentRel);
    return dir === "." ? "README.md" : `${dir}/README.md`;
  }
  return contentRel;
}

/** Site path of a docs-relative file, with an optional anchor. */
export function docUrl(rel: string, anchor?: string): string {
  return doc(slugOf(rel), anchor);
}

/** GitHub URL of a repository path: `blob` for files, `tree` for folders. */
export function githubUrl(repoPath: string, opts: { anchor?: string; dir?: boolean } = {}): string {
  const clean = repoPath.replace(/^\/+|\/+$/g, "");
  const anchor = opts.anchor ? `#${opts.anchor.replace(/^#/, "")}` : "";
  return `${REPO_URL}/${opts.dir ? "tree" : "blob"}/main/${clean}${anchor}`;
}

/** The "Edit on GitHub" URL of a validated source path. */
export function editUrl(source: string): string {
  if (!SOURCE_PATH.test(source)) throw new Error(`not a docs source path: ${source}`);
  return `${REPO_URL}/edit/main/${source}`;
}

/** Split `path#anchor` (a URL as written in Markdown). */
export function splitAnchor(url: string): { path: string; anchor?: string } {
  const i = url.indexOf("#");
  if (i < 0) return { path: url };
  return { path: url.slice(0, i), anchor: url.slice(i + 1) || undefined };
}
