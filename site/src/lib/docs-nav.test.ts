// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { buildNav, loadNav, parseCliBullets, parseReadmeTable, plainText, publishedSet, readerDescription } from "./docs-nav.ts";

const repo = new URL("../../../", import.meta.url).pathname;
const read = (rel: string) => readFileSync(`${repo}docs/${rel}`, "utf8");

test("the docs/README.md table parses, in order, with plain-text descriptions", () => {
  const rows = parseReadmeTable(read("README.md"));
  assert.equal(rows[0].rel, "getting-started.md");
  const cli = rows.find((r) => r.rel === "cli/README.md");
  assert.equal(cli?.description, "Every command and flag, one page each");
  const streaming = rows.find((r) => r.rel === "guides/streaming.md");
  assert.equal(streaming?.description, "Server-Sent Events and WebSocket tests in .sonde files");
  assert.ok(rows.every((r) => r.description.length > 0 && !r.description.includes("`")));
});

test("the CLI bullets parse with their descriptions", () => {
  const rows = parseCliBullets(read("cli/README.md"));
  assert.equal(rows[0].rel, "cli/sonde.md");
  const check = rows.find((r) => r.rel === "cli/sonde_check.md");
  assert.equal(check?.description, "Check request files for syntax errors");
  assert.equal(rows.find((r) => r.rel === "cli/sonde_run.md")?.description, "Run request files (same as sonde FILE...)");
});

test("the nav publishes user docs and benchmarks, not internal docs", () => {
  const nav = loadNav(repo);
  assert.deepEqual(nav.map((s) => s.title), ["Get started", "Guides", "CLI reference", "Reference"]);
  const published = publishedSet(nav);
  for (const rel of ["getting-started.md", "desktop.md", "guides/openapi.md", "cli/README.md", "cli/sonde_check.md", "benchmarks.md", "compat.md"]) {
    assert.ok(published.has(rel), rel);
  }
  for (const rel of ["architecture.md", "conformance.md", "release.md", "decisions/0001-openapi-library.md"]) {
    assert.ok(!published.has(rel), rel);
  }
  assert.equal(nav.find((s) => s.folder === "cli")?.collapsed, true);
});

test("a listed doc without a README row fails", () => {
  assert.throws(() => buildNav([], []), /no table row/);
  assert.throws(() => parseReadmeTable("# Docs\n\nno table"), /no table rows/);
  assert.throws(() => parseCliBullets("nothing"), /no bullets/);
});

test("markdown to plain text", () => {
  assert.equal(plainText("See [x](x.md) and `--jobs` **now**"), "See x and --jobs now");
  assert.equal(readerDescription("Every command (generated, `make docs`)"), "Every command");
});
