// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What the docs site publishes, and in which order. The section lists below
// are the only hand-kept part; guide order and every description come from
// docs/README.md (its table) and docs/cli/README.md (its bullets), so the
// repository stays the one place they are written.

import { readFileSync } from "node:fs";
import { join } from "node:path";

export interface DocEntry {
  /** Path relative to docs/, e.g. `guides/openapi.md`. */
  rel: string;
  description: string;
}

export interface NavSection {
  title: string;
  /** Top-level pages under a separator, or a folder of the same name. */
  folder?: "guides" | "cli";
  collapsed?: boolean;
  docs: DocEntry[];
}

const GET_STARTED = ["getting-started.md", "desktop.md", "file-format.md", "sonde-yaml.md"];
const REFERENCE = ["report-json.md", "compat.md", "stability.md", "security.md", "benchmarks.md"];

/** Markdown inline text → plain text: links keep their text, code loses its backticks. */
export function plainText(md: string): string {
  return md
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/\s+/g, " ")
    .trim();
}

/** Contributor notes such as "(generated, `make docs`)" are not reader copy. */
export function readerDescription(md: string): string {
  return plainText(md.replace(/\s*\(generated[^)]*\)\s*$/i, ""));
}

/** Rows of the docs/README.md table: `| [x.md](x.md) | Contents |`. */
export function parseReadmeTable(md: string): DocEntry[] {
  const rows: DocEntry[] = [];
  for (const line of md.split("\n")) {
    const m = line.match(/^\|\s*\[[^\]]+\]\(([^)#\s]+)\)\s*\|\s*(.+?)\s*\|\s*$/);
    if (m) rows.push({ rel: m[1], description: readerDescription(m[2]) });
  }
  if (rows.length === 0) throw new Error("docs/README.md: no table rows found (`| [file.md](file.md) | Contents |`)");
  return rows;
}

/** Bullets of docs/cli/README.md: `- [sonde check](sonde_check.md) — Description`. Paths are relative to docs/. */
export function parseCliBullets(md: string): DocEntry[] {
  const rows: DocEntry[] = [];
  for (const line of md.split("\n")) {
    const m = line.match(/^\s*[-*]\s+\[[^\]]+\]\(([^)#\s]+)\)\s+—\s+(.+?)\s*$/);
    if (m) rows.push({ rel: `cli/${m[1]}`, description: readerDescription(m[2]) });
  }
  if (rows.length === 0) throw new Error("docs/cli/README.md: no bullets found (`- [sonde x](sonde_x.md) — Description`)");
  return rows;
}

/** The published docs, in sections. Throws when a listed doc has no description. */
export function buildNav(readme: DocEntry[], cli: DocEntry[]): NavSection[] {
  const described = new Map(readme.map((r) => [r.rel, r.description]));
  const pick = (rel: string): DocEntry => {
    const description = described.get(rel);
    if (!description) throw new Error(`docs/README.md: no table row for ${rel}`);
    return { rel, description };
  };
  return [
    { title: "Get started", docs: GET_STARTED.map(pick) },
    { title: "Guides", folder: "guides", docs: readme.filter((r) => r.rel.startsWith("guides/")) },
    { title: "CLI reference", folder: "cli", collapsed: true, docs: [pick("cli/README.md"), ...cli] },
    { title: "Reference", docs: REFERENCE.map(pick) },
  ];
}

/** Read both index files from a repository checkout and build the nav. */
export function loadNav(repoRoot: string): NavSection[] {
  const read = (rel: string) => readFileSync(join(repoRoot, "docs", rel), "utf8");
  return buildNav(parseReadmeTable(read("README.md")), parseCliBullets(read("cli/README.md")));
}

/** Every published doc (relative to docs/), plus docs/README.md, which becomes /docs. */
export function publishedSet(nav: NavSection[]): Set<string> {
  return new Set(["README.md", ...nav.flatMap((s) => s.docs.map((d) => d.rel))]);
}
