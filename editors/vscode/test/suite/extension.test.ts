// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import * as assert from "node:assert";
import * as path from "node:path";
import * as vscode from "vscode";

// Smoke test against the real `sonde lsp` binary. Which binary to run is
// set via "sonde.path" in test/fixtures/.vscode/settings.json, written by
// test/setup-vscode-settings.mjs (see there for why this can't happen here,
// at test run time, instead).

function fixture(name: string): string {
  return path.join(__dirname, "..", "..", "..", "test", "fixtures", name);
}

async function waitFor(
  predicate: () => boolean | Promise<boolean>,
  timeoutMs: number,
  message: string,
): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (await predicate()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`timed out waiting for: ${message}`);
}

suite("Sonde extension", () => {
  suiteSetup(async function () {
    this.timeout(30000);
    const ext = vscode.extensions.getExtension("nhtera.sonde");
    assert.ok(ext, "extension nhtera.sonde was not found by the test host");
    // Idempotent: workspaceContains:**/*.sonde likely already activated it.
    await ext!.activate();
  });

  test("activates on a .sonde file", async () => {
    const doc = await vscode.workspace.openTextDocument(fixture("unformatted.sonde"));
    await vscode.window.showTextDocument(doc);
    assert.strictEqual(doc.languageId, "sonde");
  });

  test("reports a diagnostic for a broken file", async function () {
    this.timeout(20000);
    const doc = await vscode.workspace.openTextDocument(fixture("broken.sonde"));
    await vscode.window.showTextDocument(doc);

    await waitFor(
      () => vscode.languages.getDiagnostics(doc.uri).length > 0,
      15000,
      "a diagnostic on broken.sonde",
    );

    const diagnostics = vscode.languages.getDiagnostics(doc.uri);
    assert.ok(diagnostics.length > 0, "expected at least one diagnostic");
    assert.strictEqual(diagnostics[0].severity, vscode.DiagnosticSeverity.Error);
    assert.ok(diagnostics[0].message.length > 0);
  });

  test("formatting returns an edit", async function () {
    this.timeout(20000);
    const doc = await vscode.workspace.openTextDocument(fixture("unformatted.sonde"));
    await vscode.window.showTextDocument(doc);

    let edits: vscode.TextEdit[] | undefined;
    await waitFor(
      async () => {
        edits = await vscode.commands.executeCommand<vscode.TextEdit[]>(
          "vscode.executeFormatDocumentProvider",
          doc.uri,
          { tabSize: 2, insertSpaces: true },
        );
        return !!edits && edits.length > 0;
      },
      15000,
      "a formatting edit for unformatted.sonde",
    );

    assert.ok(edits && edits.length > 0, "expected at least one formatting edit");
  });
});
