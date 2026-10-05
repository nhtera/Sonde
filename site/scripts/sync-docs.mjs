// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Copies the published docs from docs/ into site/content/docs (generated,
// git-ignored) as Markdown with frontmatter, and writes the generated data
// the site reads from the repository:
//
//   content/docs/**.md, meta.json   the docs, H1 moved into frontmatter
//   content/generated/install.json  install commands from README.md ## Install
//   content/generated/inputs.json   every repository file this step read
//
// Fails fast on a missing doc, a doc without an H1 or a description, a bad
// file name, or a shallow clone (dates). Runs no shell: git is called with
// execFileSync and fixed arguments.
//
//   node scripts/sync-docs.mjs            (SONDE_SITE_NO_DATES=1 omits dates)

import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { stringify } from "yaml";
import { checkDocName, contentPathOf, SOURCE_PATH, slugOf } from "../src/lib/doc-paths.ts";
import { loadNav, publishedSet } from "../src/lib/docs-nav.ts";

const here = dirname(fileURLToPath(import.meta.url));
export const SITE = resolve(here, "..");
export const REPO = resolve(SITE, "..");

/** First `# ` heading, after any leading HTML comments and blank lines. */
export function splitTitle(md, file = "doc") {
  const lines = md.split("\n");
  let i = 0;
  let inComment = false;
  for (; i < lines.length; i++) {
    const line = lines[i].trim();
    if (inComment) {
      if (line.includes("-->")) inComment = false;
      continue;
    }
    if (line === "") continue;
    if (line.startsWith("<!--")) {
      inComment = !line.includes("-->");
      continue;
    }
    break;
  }
  const m = lines[i]?.match(/^#\s+(.+?)\s*#*\s*$/);
  if (!m) throw new Error(`${file}: the first line (after comments) must be the "# Title" heading`);
  const body = lines.slice(i + 1).join("\n").replace(/^\n+/, "");
  return { title: m[1].replace(/`([^`]*)`/g, "$1"), body };
}

/** First paragraph of a body, as plain text (the description fallback). */
export function firstParagraph(body) {
  const para = [];
  let fence = false;
  for (const line of body.split("\n")) {
    if (line.trim().startsWith("```")) {
      if (para.length) break;
      fence = !fence;
      continue;
    }
    if (fence) continue;
    if (line.trim() === "") {
      if (para.length) break;
      continue;
    }
    if (/^(#|\||[-*] |>|<!--)/.test(line.trim())) {
      if (para.length) break;
      continue;
    }
    para.push(line.trim());
  }
  return para.join(" ").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1").replace(/`([^`]*)`/g, "$1").trim();
}

/** YAML frontmatter, serialized (never string-templated). */
export function frontmatter(data) {
  return `---\n${stringify(data, { lineWidth: 0 })}---\n\n`;
}

/** Commands under README.md "## Install", one line per tab (multi-line → `; `). */
export function parseInstall(readme) {
  const section = readme.match(/^## Install\n([\s\S]*?)(?=^## )/m)?.[1];
  if (!section) throw new Error("README.md: no ## Install section");
  const tabs = [];
  const re = /\*\*([^*]+?)\*\*[^\n]*\n+```sh\n([\s\S]*?)```/g;
  for (const m of section.matchAll(re)) {
    const label = m[1].replace(/:$/, "").trim();
    const command = m[2].trim().split("\n").map((l) => l.trim()).filter(Boolean).join("; ");
    tabs.push({ id: label.toLowerCase().replace(/[^a-z0-9]+/g, "-"), label, command });
  }
  if (tabs.length === 0) throw new Error("README.md ## Install: no **Label** + ```sh blocks found");
  return tabs;
}

function lastUpdated(repo, repoPath) {
  const out = execFileSync("git", ["log", "-1", "--format=%cs", "--", repoPath], { cwd: repo, encoding: "utf8" }).trim();
  return out || undefined;
}

function checkNotShallow(repo) {
  const shallow = execFileSync("git", ["rev-parse", "--is-shallow-repository"], { cwd: repo, encoding: "utf8" }).trim();
  if (shallow === "true") {
    throw new Error("shallow clone: last-updated dates need full history (fetch-depth: 0), or set SONDE_SITE_NO_DATES=1");
  }
}

function listDocs(dir, base = "") {
  const out = [];
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const rel = base ? `${base}/${e.name}` : e.name;
    if (e.isDirectory()) out.push(...listDocs(join(dir, e.name), rel));
    else if (e.name.endsWith(".md")) out.push(rel);
  }
  return out.sort();
}

export function sync({ repo = REPO, out = join(SITE, "content"), dates = !process.env.SONDE_SITE_NO_DATES, log = console.log } = {}) {
  const inputs = new Set(["README.md", "docs/README.md", "docs/cli/README.md"]);
  const nav = loadNav(repo);
  const published = publishedSet(nav);
  if (dates) checkNotShallow(repo);

  const docsOut = join(out, "docs");
  rmSync(out, { recursive: true, force: true });
  mkdirSync(docsOut, { recursive: true });

  const descriptions = new Map(nav.flatMap((s) => s.docs.map((d) => [d.rel, d.description])));
  for (const rel of published) {
    if (rel === "README.md") continue; // the index page is generated below
    checkDocName(rel);
    const source = `docs/${rel}`;
    if (!SOURCE_PATH.test(source)) throw new Error(`${source}: not a valid docs path`);
    const file = join(repo, source);
    if (!existsSync(file)) throw new Error(`${source}: listed in the docs nav but missing`);
    inputs.add(source);
    const md = readFileSync(file, "utf8");
    const { title, body } = splitTitle(md, source);
    if (/!\[[^\]]*\]\(/.test(body.replace(/```[\s\S]*?```/g, ""))) throw new Error(`${source}: images are not supported on the site yet`);
    const description = descriptions.get(rel) || firstParagraph(body);
    if (!description) throw new Error(`${source}: no description (docs/README.md table row or first paragraph)`);
    const data = { title, description, source };
    if (dates) {
      const date = lastUpdated(repo, source);
      if (date) data.lastUpdated = date;
    }
    const target = join(docsOut, contentPathOf(rel));
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, frontmatter(data) + body);
  }

  // /docs: the index. Its cards are rendered from the page tree.
  writeFileSync(
    join(docsOut, "index.md"),
    frontmatter({
      title: "Sonde documentation",
      description: "Guides and reference for the sonde CLI and Sonde Desktop.",
      source: "docs/README.md",
    }),
  );

  // Sidebar order: sections as separators, guides and the CLI as folders.
  const top = (rel) => slugOf(rel);
  const rootPages = ["index"];
  for (const s of nav) {
    if (s.folder) rootPages.push(s.folder);
    else rootPages.push(`---${s.title}---`, ...s.docs.map((d) => top(d.rel)));
  }
  writeFileSync(join(docsOut, "meta.json"), `${JSON.stringify({ title: "Docs", root: true, pages: rootPages }, null, 2)}\n`);
  for (const s of nav.filter((x) => x.folder)) {
    const pages = s.docs.filter((d) => !d.rel.endsWith("README.md")).map((d) => d.rel.slice(s.folder.length + 1).replace(/\.md$/, ""));
    const meta = { title: s.title, defaultOpen: !s.collapsed, pages: s.docs.some((d) => d.rel.endsWith("README.md")) ? ["index", ...pages] : pages };
    writeFileSync(join(docsOut, s.folder, "meta.json"), `${JSON.stringify(meta, null, 2)}\n`);
  }

  // Generated data for the landing page.
  const gen = join(out, "generated");
  mkdirSync(gen, { recursive: true });
  writeFileSync(join(gen, "install.json"), `${JSON.stringify(parseInstall(readFileSync(join(repo, "README.md"), "utf8")), null, 2)}\n`);

  const unpublished = listDocs(join(repo, "docs")).filter((rel) => !published.has(rel));
  if (unpublished.length) log(`unpublished docs: ${unpublished.join(", ")}`);
  writeFileSync(join(gen, "inputs.json"), `${JSON.stringify([...inputs].sort(), null, 2)}\n`);
  log(`synced ${published.size} docs into ${relative(SITE, docsOut)}`);
  return { published, unpublished, inputs };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    sync();
  } catch (err) {
    console.error(`sync-docs: ${err.message}`);
    process.exit(1);
  }
}
