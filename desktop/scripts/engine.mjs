// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Prints the engine the desktop app is built on: the nearest CLI tag and
// the commit, "v1.3.1+a7404ac" (main.engine, the release notes' Engine
// line). The CLI's tags are v[0-9]*; desktop/* and editors/vscode/* never
// match. Prints "dev" outside a git checkout or without a CLI tag (a
// shallow clone: the release workflow fetches the full history).
//   node scripts/engine.mjs

import { execFileSync } from "node:child_process";

const git = (...args) => execFileSync("git", args, { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim();

let engine = "dev";
try {
  engine = `${git("describe", "--tags", "--abbrev=0", "--match", "v[0-9]*")}+${git("rev-parse", "--short=7", "HEAD")}`;
} catch {
  // Not a checkout, or no CLI tag reachable.
}
console.log(engine);
