// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { Link } from "@tanstack/react-router";
import type * as PageTree from "fumadocs-core/page-tree";
import type { ReactNode } from "react";

interface Section {
  name: ReactNode;
  pages: PageTree.Item[];
}

/**
 * Sections of the docs index, read from the page tree: each separator or
 * folder starts one. A folder with an index page (the CLI reference) shows
 * only that page; its index lists the rest.
 */
function sections(tree: PageTree.Root): Section[] {
  const out: Section[] = [];
  for (const node of tree.children) {
    if (node.type === "separator") out.push({ name: node.name, pages: [] });
    else if (node.type === "folder") {
      const pages = node.index ? [node.index] : node.children.filter((n): n is PageTree.Item => n.type === "page");
      out.push({ name: node.name, pages });
    } else if (node.type === "page" && out.length > 0) out.at(-1)?.pages.push(node);
  }
  return out.filter((s) => s.pages.length > 0);
}

/** /docs: one card per page, grouped like the sidebar. Titles and descriptions come from docs/README.md. */
export function IndexCards({ tree }: { tree: PageTree.Root }) {
  return (
    <div className="docs-index not-prose">
      {sections(tree).map((s, i) => (
        <section key={i} aria-labelledby={`docs-sec-${i}`}>
          <h2 id={`docs-sec-${i}`}>{s.name}</h2>
          <ul>
            {s.pages.map((p) => (
              <li key={p.url}>
                <Link to={p.url}>
                  <b>{p.name}</b>
                  {p.description ? <span>{p.description}</span> : null}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
