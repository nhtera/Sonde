// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A release binary must not hold the test-only harness (built with the
// e2eharness tag): its files and tag leave their names in the binary.
// Usage: node scripts/check-no-harness.mjs BINARY...

import { readFileSync } from "node:fs";

const markers = ["e2eharness", "harness_service.go", "main_harness.go"];
let bad = false;
for (const file of process.argv.slice(2)) {
  const data = readFileSync(file);
  for (const m of markers) {
    if (data.includes(Buffer.from(m))) {
      console.error(`${file}: holds ${m}: a harness build, never released`);
      bad = true;
    }
  }
}
if (process.argv.length < 3) {
  console.error("usage: node scripts/check-no-harness.mjs BINARY...");
  bad = true;
}
if (bad) process.exit(1);
console.log("no harness in", process.argv.slice(2).join(", "));
