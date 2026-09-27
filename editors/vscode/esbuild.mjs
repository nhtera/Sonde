// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Bundles src/extension.ts (plus vscode-languageclient) into a single
// dist/extension.js so the packaged .vsix ships one file instead of the
// full node_modules + tsc output tree. Separate from `tsc -p .` (still
// used for type-checking and to compile the mocha test suite into
// out/test).

import * as esbuild from "esbuild";

const watch = process.argv.includes("--watch");
const production = process.argv.includes("--production");

const ctx = await esbuild.context({
  entryPoints: ["src/extension.ts"],
  bundle: true,
  outfile: "dist/extension.js",
  external: ["vscode"],
  format: "cjs",
  platform: "node",
  target: "node20",
  sourcemap: !production,
  minify: production,
  logLevel: "info",
});

if (watch) {
  await ctx.watch();
} else {
  await ctx.rebuild();
  await ctx.dispose();
}
