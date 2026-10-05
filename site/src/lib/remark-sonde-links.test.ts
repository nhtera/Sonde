// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { evaluate } from "@mdx-js/mdx";
import { createElement } from "react";
import * as runtime from "react/jsx-runtime";
import { renderToStaticMarkup } from "react-dom/server";
import remarkGfm from "remark-gfm";
import remarkSondeLinks, { rewriteUrl, type SondeLinksOptions } from "./remark-sonde-links.ts";

const repoRoot = new URL("../../..", import.meta.url).pathname.replace(/\/$/, "");
const contentDir = `${repoRoot}/site/content/docs`;
const opts: SondeLinksOptions = {
  repoRoot,
  contentDir,
  published: new Set(["README.md", "getting-started.md", "sonde-yaml.md", "guides/openapi.md", "guides/mock-server.md", "cli/README.md", "cli/sonde_check.md"]),
};

/** Compile Markdown the way the site does (format md + GFM + the plugin) and render it. */
async function render(md: string, contentRel = "guides/openapi.md"): Promise<string> {
  const { default: Content } = await evaluate(
    { value: md, path: `${contentDir}/${contentRel}` },
    { ...runtime, format: "md", remarkPlugins: [[remarkSondeLinks, opts], remarkGfm] },
  );
  return renderToStaticMarkup(createElement(Content));
}

test("links to published docs become site links", () => {
  assert.equal(rewriteUrl("mock-server.md", "guides/openapi.md", opts), "/docs/guides/mock-server");
  assert.equal(rewriteUrl("../sonde-yaml.md#environments", "guides/openapi.md", opts), "/docs/sonde-yaml#environments");
  assert.equal(rewriteUrl("./guides/openapi.md", "getting-started.md", opts), "/docs/guides/openapi");
  assert.equal(rewriteUrl("cli/README.md", "README.md", opts), "/docs/cli");
  assert.equal(rewriteUrl("README.md", "cli/README.md", opts), "/docs/cli");
  assert.equal(rewriteUrl("../README.md", "guides/openapi.md", opts), "/docs");
  assert.equal(rewriteUrl("sonde_check.md", "cli/README.md", opts), "/docs/cli/sonde_check");
});

test("links to unpublished docs and repository files go to GitHub", () => {
  assert.equal(rewriteUrl("../architecture.md", "guides/openapi.md", opts), "https://github.com/nhtera/sonde/blob/main/docs/architecture.md");
  assert.equal(rewriteUrl("../README.md#install", "getting-started.md", opts), "https://github.com/nhtera/sonde/blob/main/README.md#install");
  assert.equal(rewriteUrl("../../desktop/MANUAL-TEST.md", "guides/openapi.md", opts), "https://github.com/nhtera/sonde/blob/main/desktop/MANUAL-TEST.md");
  assert.equal(rewriteUrl("decisions/", "README.md", opts), "https://github.com/nhtera/sonde/tree/main/docs/decisions");
});

test("anchors and allowed schemes pass; other schemes and missing files fail", () => {
  assert.equal(rewriteUrl("#flags", "cli/sonde_check.md", opts), "#flags");
  assert.equal(rewriteUrl("https://example.com/a", "getting-started.md", opts), "https://example.com/a");
  assert.equal(rewriteUrl("mailto:security@example.com", "getting-started.md", opts), "mailto:security@example.com");
  for (const bad of ["javascript:alert(1)", "JavaScript:alert(1)", "data:text/html,x", "http://example.com", "//evil.example/x", "vbscript:x"]) {
    assert.throws(() => rewriteUrl(bad, "getting-started.md", opts), /only https: and mailto:/, bad);
  }
  assert.throws(() => rewriteUrl("nope.md", "getting-started.md", opts), /does not exist/);
  assert.throws(() => rewriteUrl("../../../../etc/passwd", "getting-started.md", opts), /outside the repository/);
});

test("links are rewritten on the tree, never inside code", async () => {
  const html = await render("See [mock](mock-server.md) and `[x](mock-server.md)`.\n\n```md\n[y](mock-server.md)\n```\n\n[ref][r]\n\n[r]: ../sonde-yaml.md\n");
  assert.match(html, /href="\/docs\/guides\/mock-server"/);
  assert.match(html, /<code>\[x\]\(mock-server\.md\)<\/code>/);
  assert.match(html, /\[y\]\(mock-server\.md\)/);
  assert.match(html, /href="\/docs\/sonde-yaml"/);
});

test("the hostile fixture renders inert", async () => {
  const fixture = readFileSync(new URL("../../test/fixtures/hostile.md", import.meta.url), "utf8").replace(/\[javascript link\]\(javascript:alert\(1\)\)\n/, "");
  const html = await render(fixture, "hostile.md");
  assert.match(html, /\{process\.env\.HOME\}/);
  assert.match(html, /\{\{base_url\}\}\/users\/\{\{id\}\}/);
  assert.match(html, /Map&lt;string, number&gt;/);
  for (const bad of ["<script", "onerror", "onclick", "<img", "<div", "a comment"]) assert.ok(!html.includes(bad), bad);
  await assert.rejects(render("[x](javascript:alert(1))", "hostile.md"), /only https: and mailto:/);
});

test("images fail the build", async () => {
  await assert.rejects(render("![logo](logo.png)", "getting-started.md"), /images are not supported/);
});

test("files outside content/docs are left alone", async () => {
  const { default: Content } = await evaluate({ value: "[x](nope.md)", path: "/elsewhere/page.md" }, { ...runtime, format: "md", remarkPlugins: [[remarkSondeLinks, opts]] });
  assert.match(renderToStaticMarkup(createElement(Content)), /href="nope\.md"/);
});
