// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The npm packages the site ships to browsers or the Worker (site/package-lock.json,
// dev dependencies left out: they build and test it) must use an allowed
// license.

import { readFileSync } from "node:fs";

// Permissive code licenses, Unlicense (public domain; isbot, which the
// router ships to the Worker), and OFL-1.1 for the bundled fonts.
const allowed = new Set(["MIT", "ISC", "0BSD", "BSD-2-Clause", "BSD-3-Clause", "Apache-2.0", "Unlicense", "OFL-1.1"]);

// Build tools that arrive through runtime packages (the TanStack Start
// Vite plugin, browserslist data). They run at build time and are in no
// bundle: `grep -rl <name> dist/` finds nothing.
const buildOnly = [/^lightningcss(-|$)/, /^argparse$/, /^caniuse-lite$/];

const lockPath = new URL("../package-lock.json", import.meta.url);
const packages = JSON.parse(readFileSync(lockPath, "utf8")).packages;
let bad = false;
let n = 0;
for (const [path, p] of Object.entries(packages)) {
  if (path === "" || p.dev) continue;
  const name = path.replace(/^.*node_modules\//, "");
  if (buildOnly.some((re) => re.test(name))) continue;
  n++;
  if (!allowed.has(p.license)) {
    console.error(`${name}: license ${p.license ?? "not stated"} is not allowed`);
    bad = true;
  }
}
if (bad) process.exit(1);
console.log(`npm licenses ok (${n} shipped packages)`);
