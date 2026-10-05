// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import assert from "node:assert/strict";
import { test } from "node:test";
import { checkDocName, contentPathOf, docUrl, editUrl, githubUrl, slugOf, sourceRelOf, splitAnchor } from "./doc-paths.ts";
import { absolute, doc } from "./urls.ts";

test("slugs: README.md is the folder index, no trailing slash", () => {
  assert.equal(slugOf("getting-started.md"), "getting-started");
  assert.equal(slugOf("guides/openapi.md"), "guides/openapi");
  assert.equal(slugOf("README.md"), "");
  assert.equal(slugOf("cli/README.md"), "cli");
  assert.equal(docUrl("cli/README.md"), "/docs/cli");
  assert.equal(docUrl("README.md"), "/docs");
  assert.equal(docUrl("guides/openapi.md", "mock"), "/docs/guides/openapi#mock");
  assert.equal(doc("x/", "#a"), "/docs/x#a");
  assert.equal(absolute("/docs/x/"), "https://sonde.erai.dev/docs/x");
  assert.equal(absolute("/"), "https://sonde.erai.dev/");
});

test("content paths round-trip", () => {
  for (const rel of ["README.md", "cli/README.md", "guides/openapi.md", "cli/sonde_check.md"]) {
    assert.equal(sourceRelOf(contentPathOf(rel)), rel);
  }
  assert.equal(contentPathOf("cli/README.md"), "cli/index.md");
});

test("file names are checked", () => {
  checkDocName("guides/data-driven.md");
  checkDocName("cli/sonde_import_curl.md");
  checkDocName("cli/README.md");
  assert.throws(() => checkDocName("Guide.md"), /file name/);
  assert.throws(() => checkDocName("guides/a b.md"), /file name/);
  assert.throws(() => checkDocName("Guides/x.md"), /folder name/);
});

test("GitHub URLs and the edit URL", () => {
  assert.equal(githubUrl("SECURITY.md"), "https://github.com/nhtera/sonde/blob/main/SECURITY.md");
  assert.equal(githubUrl("README.md", { anchor: "install" }), "https://github.com/nhtera/sonde/blob/main/README.md#install");
  assert.equal(githubUrl("docs/decisions", { dir: true }), "https://github.com/nhtera/sonde/tree/main/docs/decisions");
  assert.equal(editUrl("docs/guides/openapi.md"), "https://github.com/nhtera/sonde/edit/main/docs/guides/openapi.md");
  assert.throws(() => editUrl("docs/../../etc/passwd"), /not a docs source path/);
  assert.throws(() => editUrl("https://evil.example/x.md"), /not a docs source path/);
});

test("anchors split off", () => {
  assert.deepEqual(splitAnchor("x.md#a"), { path: "x.md", anchor: "a" });
  assert.deepEqual(splitAnchor("x.md"), { path: "x.md" });
  assert.deepEqual(splitAnchor("x.md#"), { path: "x.md", anchor: undefined });
});
