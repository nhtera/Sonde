// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import assert from "node:assert/strict";
import { test } from "node:test";
import { matches, sitePaths } from "./ci-paths.mjs";

test("the workflow's path filter is read without a YAML parser", () => {
  const globs = sitePaths("env:\n  NODE: x\n  SITE_PATHS: |\n    site/**\n    # a note\n    README.md\n\njobs:\n  x: y\n");
  assert.deepEqual(globs, ["site/**", "README.md"]);
  assert.ok(sitePaths().includes("docs/**"));
});

test("globs match folders and exact files", () => {
  const globs = ["site/**", "README.md", "desktop/frontend/src/app/theme/**"];
  assert.ok(matches("site/src/x.ts", globs));
  assert.ok(matches("desktop/frontend/src/app/theme/tokens.css", globs));
  assert.ok(matches("README.md", globs));
  assert.ok(!matches("docs/README.md", globs));
  assert.ok(!matches("sites/x", globs));
  assert.ok(!matches("internal/cli/run.go", globs));
});
