// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The npm packages the app ships (frontend/package-lock.json, dev
// dependencies left out: they build and test it, and are not bundled)
// must use an allowed license. Run from desktop/.

import { readFileSync } from "node:fs";

// Permissive code licenses, and OFL-1.1 for the bundled fonts.
const allowed = new Set(["MIT", "ISC", "0BSD", "BSD-2-Clause", "BSD-3-Clause", "Apache-2.0", "OFL-1.1"]);

const packages = JSON.parse(readFileSync("frontend/package-lock.json", "utf8")).packages;
let bad = false;
let n = 0;
for (const [path, p] of Object.entries(packages)) {
  if (path === "" || p.dev) continue;
  n++;
  if (!allowed.has(p.license)) {
    console.error(`${path.replace(/^.*node_modules\//, "")}: license ${p.license ?? "not stated"} is not allowed`);
    bad = true;
  }
}
if (bad) process.exit(1);
console.log(`npm licenses ok (${n} shipped packages)`);
