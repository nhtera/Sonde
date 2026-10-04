// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { test } from "node:test";
import assert from "node:assert";

const script = join(dirname(fileURLToPath(import.meta.url)), "engine.mjs");
const engine = (cwd, env) => execFileSync(process.execPath, [script], { cwd, encoding: "utf8", env }).trim();

test("the nearest CLI tag and the commit; other trains' tags never match", (t) => {
  const dir = mkdtempSync(join(tmpdir(), "sonde-engine-test-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  // Only this repository: no GIT_DIR or the like from a hook, no signing.
  const env = Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith("GIT_")));
  const git = (...args) =>
    execFileSync("git", ["-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", ...args], { cwd: dir, encoding: "utf8", env }).trim();
  git("init", "-q");
  assert.equal(engine(dir, env), "dev", "no commit yet");
  writeFileSync(join(dir, "f"), "1");
  git("add", "f");
  git("commit", "-qm", "one");
  assert.equal(engine(dir, env), "dev", "no CLI tag");
  git("tag", "v1.3.1");
  writeFileSync(join(dir, "f"), "2");
  git("commit", "-qam", "two");
  git("tag", "desktop/v0.2.0");
  git("tag", "editors/vscode/v1.0.4");
  assert.equal(engine(dir, env), `v1.3.1+${git("rev-parse", "--short=7", "HEAD")}`);
});
