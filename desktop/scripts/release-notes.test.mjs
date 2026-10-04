// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { test } from "node:test";
import assert from "node:assert";
import { releaseNotes } from "./release-notes.mjs";

test("the engine, the updates line and the repository", () => {
  const n = releaseNotes("0.2.1", "v1.3.1+a7404ac", "nhtera/Sonde");
  assert.match(n, /^Sonde Desktop 0\.2\.1: /);
  assert.match(n, /\nEngine: Sonde CLI v1\.3\.1 \+ a7404ac\n/);
  assert.match(n, /in place is tested by automated tests only/);
  assert.match(n, /--repo nhtera\/Sonde\)/);
  assert.match(n, /Sonde-Desktop-0\.2\.1\.update\.json/);
  assert.doesNotMatch(n, /0\.1\.0 has no updater/, "only 0.2.0 says so");
});

test("0.2.0 and its candidates ask 0.1.0 users to download once", () => {
  for (const v of ["0.2.0", "0.2.0-rc.1"]) {
    assert.match(releaseNotes(v, "v1.3.1+a7404ac", "r"), /0\.1\.0 has no updater: download this release once/, v);
  }
  assert.doesNotMatch(releaseNotes("0.2.10", "dev", "r"), /0\.1\.0 has no updater/);
});
