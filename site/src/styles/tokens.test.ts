// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Every desktop token the site uses must exist in tokens.css. The site
// imports the file as it is, so a rename there breaks the site here first.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const here = new URL(".", import.meta.url);
const read = (rel: string) => readFileSync(new URL(rel, here), "utf8");
const readIfPresent = (rel: string): string | null => {
  try {
    return read(rel);
  } catch {
    return null;
  }
};

const tokens = read("../../../desktop/frontend/src/app/theme/tokens.css");
const defined = new Set([...tokens.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1]));

// Variables the site defines itself (spacing, type scale, motion, layout)
// or that Tailwind and Fumadocs define.
const own = /^--(s\d+|shot-dark|shot-light|t-(display|h2|h3|body|small|micro)|ease|wrap|i|color-|text-|radius-|font-sans|font-mono|spacing|shiki-|fd-|tw-)/;

function used(source: string): string[] {
  return [...new Set([...source.matchAll(/var\((--[a-z0-9-]+)/g)].map((m) => m[1]))];
}

for (const file of ["./sonde-site.css", "./components.css", "./landing.css", "./docs.css", "../lib/shiki.ts"]) {
  test(`tokens used by ${file} exist in tokens.css`, (t) => {
    const source = readIfPresent(file);
    if (source === null) {
      t.skip("not present");
      return;
    }
    const missing = used(source).filter((name) => !defined.has(name) && !own.test(name));
    assert.deepEqual(missing, [], `not defined in desktop tokens.css: ${missing.join(", ")}`);
  });
}

test("tokens.css still defines both themes", () => {
  assert.match(tokens, /:root,\s*\[data-theme="dark"\]\s*\{/);
  assert.match(tokens, /\[data-theme="light"\]\s*\{/);
});

test("the site copies no palette: no hex colors outside the logo", () => {
  for (const file of ["./sonde-site.css", "./components.css", "./landing.css", "./docs.css"]) {
    const source = readIfPresent(file);
    if (source === null) continue;
    assert.deepEqual(source.match(/#[0-9a-fA-F]{3,8}\b/g) ?? [], [], file);
  }
});
