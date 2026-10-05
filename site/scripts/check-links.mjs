// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Checks the built site as it will be deployed (dist/client by default):
// every internal href/src resolves to a file (`/x` → x/index.html), every
// anchor names an element id on its page, and the docs content
// (`<article id="nd-page">`) carries no <script>, <iframe> or on*= handler.
//
//   node scripts/check-links.mjs [dir]

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const decode = (s) => s.replace(/&amp;/g, "&").replace(/&#x27;/g, "'").replace(/&quot;/g, '"');

function htmlFiles(dir) {
  const out = [];
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, e.name);
    if (e.isDirectory()) out.push(...htmlFiles(p));
    else if (e.name.endsWith(".html")) out.push(p);
  }
  return out;
}

/** The URL path a file is served at: docs/x/index.html → /docs/x. */
export function pagePath(root, file) {
  const rel = relative(root, file).split(sep).join("/");
  const path = `/${rel.replace(/(^|\/)index\.html$/, "").replace(/\.html$/, "")}`;
  return path.length > 1 ? path.replace(/\/$/, "") : "/";
}

/** The file a site path is served from, or undefined. */
export function resolveFile(root, path) {
  const base = join(root, decodeURIComponent(path));
  for (const c of [base, join(base, "index.html"), `${base}.html`]) {
    if (existsSync(c) && statSync(c).isFile()) return c;
  }
  return undefined;
}

const ids = (html) => new Set([...html.matchAll(/\sid="([^"]+)"/g)].map((m) => decode(m[1])));

/** The docs content of a page: the <article id="nd-page"> element. */
export function docsContent(html) {
  const start = html.indexOf('<article id="nd-page"');
  if (start < 0) return "";
  const end = html.indexOf("</article>", start);
  return html.slice(start, end < 0 ? undefined : end);
}

/** Problems in one built site directory, as strings. */
export function checkSite(root) {
  const problems = [];
  const files = htmlFiles(root);
  const idCache = new Map();
  const idsOf = (file) => {
    if (!idCache.has(file)) idCache.set(file, ids(readFileSync(file, "utf8")));
    return idCache.get(file);
  };
  for (const file of files) {
    const html = readFileSync(file, "utf8");
    const from = pagePath(root, file);
    const content = docsContent(html);
    if (/<script\b|<iframe\b|\son[a-z]+\s*=|javascript:/i.test(content)) {
      problems.push(`${from}: active content (script, iframe, on*= or javascript:) inside the docs content`);
    }
    for (const m of html.matchAll(/\s(href|src)="([^"]*)"/g)) {
      const url = decode(m[2]);
      if (/^(https?:|mailto:|data:)/i.test(url)) continue;
      if (/^javascript:/i.test(url)) {
        problems.push(`${from}: javascript: URL`);
        continue;
      }
      const target = new URL(url, `http://site${from === "/" ? "/" : `${from}/`}`);
      if (url.startsWith("#")) target.pathname = from;
      const path = target.pathname.length > 1 ? target.pathname.replace(/\/$/, "") : "/";
      const targetFile = resolveFile(root, path);
      if (!targetFile) {
        problems.push(`${from}: ${m[1]}="${url}" → ${path} does not exist`);
        continue;
      }
      const anchor = decodeURIComponent(target.hash.slice(1));
      if (anchor && targetFile.endsWith(".html") && !idsOf(targetFile).has(anchor)) {
        problems.push(`${from}: "${url}" → no element with id "${anchor}" on ${path}`);
      }
    }
  }
  return { problems, pages: files.length };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(process.argv[2] ?? join(fileURLToPath(new URL("..", import.meta.url)), "dist/client"));
  const { problems, pages } = checkSite(root);
  if (problems.length) {
    console.error(`check-links: ${problems.length} problem(s) in ${root}:\n  ${problems.join("\n  ")}`);
    process.exit(1);
  }
  console.log(`links ok (${pages} pages in ${relative(process.cwd(), root) || "."})`);
}
