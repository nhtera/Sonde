// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Fails when the site lockfile has a package with an install script that is
// not on the allowlist. Dependencies are installed with
// `npm ci --ignore-scripts`, so a new install script never runs; this makes
// one visible in review before anyone relies on it.

import { readFileSync } from "node:fs";

// Packages whose install scripts are known. None is needed: each ships its
// platform binary as an optional dependency, which npm installs without
// running scripts.
const allowed = new Set([
  "node_modules/esbuild", // checks its platform binary
  "node_modules/wrangler/node_modules/esbuild",
  "node_modules/fsevents", // optional macOS file watcher; ships prebuilt
  "node_modules/workerd", // checks its platform binary
]);

const lockPath = new URL("../package-lock.json", import.meta.url);
const lock = JSON.parse(readFileSync(lockPath, "utf8"));
const unexpected = Object.entries(lock.packages ?? {})
  .filter(([path, pkg]) => pkg.hasInstallScript && !allowed.has(path))
  .map(([path]) => path);

if (unexpected.length > 0) {
  console.error(`install scripts not on the allowlist (site/scripts/check-install-scripts.mjs):\n  ${unexpected.join("\n  ")}`);
  process.exit(1);
}
console.log("install scripts ok");
