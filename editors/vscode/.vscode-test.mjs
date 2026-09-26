// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { defineConfig } from "@vscode/test-cli";

export default defineConfig({
  files: "out/test/suite/**/*.test.js",
  workspaceFolder: "test/fixtures",
  // SONDE_VSCODE_TEST_VERSION pins the VS Code build @vscode/test-cli
  // downloads to run the smoke test; unset uses its default ("stable").
  version: process.env.SONDE_VSCODE_TEST_VERSION,
  // Disables VS Code's other built-in/bundled extensions (chat, telemetry,
  // etc.) so only the extension under test runs; it does not affect loading
  // that extension, which the test harness adds via --extensionDevelopmentPath.
  launchArgs: ["--disable-extensions", "--disable-telemetry"],
  mocha: {
    timeout: 30000,
  },
});
