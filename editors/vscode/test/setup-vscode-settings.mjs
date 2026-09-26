// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Pre-seeds test/fixtures/.vscode/settings.json with "sonde.path" from
// SONDE_TEST_BINARY (or SONDE_PATH), so the workspace opens with the right
// setting from the start. This matters because the extension activates on
// workspaceContains:**/*.sonde as soon as the test workspace is opened,
// before the test suite itself runs — updating the setting afterwards would
// be too late for that first activation.

import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const settingsDir = join(here, "fixtures", ".vscode");
const settingsFile = join(settingsDir, "settings.json");

const binary = process.env.SONDE_TEST_BINARY ?? process.env.SONDE_PATH;

if (binary) {
  mkdirSync(settingsDir, { recursive: true });
  writeFileSync(settingsFile, JSON.stringify({ "sonde.path": binary }, null, 2) + "\n");
} else {
  // No override: fall back to the default "sonde.path" ("sonde" on PATH),
  // as CI does after building the binary and adding it to PATH.
  rmSync(settingsFile, { force: true });
}
