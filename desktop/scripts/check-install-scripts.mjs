// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Fails when the frontend lockfile has a package with an install script
// that is not on the allowlist. Dependencies are installed with
// `npm ci --ignore-scripts`, so a new install script never runs; this
// makes one visible in review before anyone relies on it.

import { readFileSync } from "node:fs";

// Packages whose install scripts are known and not needed.
const allowed = new Set([
  "node_modules/fsevents", // optional macOS file watcher; ships prebuilt
]);

const lockPath = new URL("../frontend/package-lock.json", import.meta.url);
const lock = JSON.parse(readFileSync(lockPath, "utf8"));
const unexpected = Object.entries(lock.packages ?? {})
  .filter(([path, pkg]) => pkg.hasInstallScript && !allowed.has(path))
  .map(([path]) => path);

if (unexpected.length > 0) {
  console.error(`install scripts not on the allowlist (desktop/scripts/check-install-scripts.mjs):\n  ${unexpected.join("\n  ")}`);
  process.exit(1);
}
console.log("install scripts ok");
