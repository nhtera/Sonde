// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { execFile } from "node:child_process";
import * as vscode from "vscode";
import {
  DocumentSelector,
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;
let output: vscode.LogOutputChannel | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  output = vscode.window.createOutputChannel("Sonde Language Server", { log: true });
  context.subscriptions.push(output);

  context.subscriptions.push(
    vscode.commands.registerCommand("sonde.restartLanguageServer", () => restartClient()),
  );

  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (
        e.affectsConfiguration("sonde.path") ||
        e.affectsConfiguration("sonde.associateHurlFiles")
      ) {
        void restartClient();
      }
      // sonde.env (and any other "sonde.*" setting) is kept in sync with the
      // running server automatically: the client's `synchronize.configurationSection`
      // below makes vscode-languageclient send workspace/didChangeConfiguration
      // on every change, no extra code needed.
    }),
  );

  await startClient();
}

export async function deactivate(): Promise<void> {
  await stopClient();
}

async function restartClient(): Promise<void> {
  await stopClient();
  await startClient();
}

async function stopClient(): Promise<void> {
  if (!client) {
    return;
  }
  const c = client;
  client = undefined;
  await c.stop();
}

async function startClient(): Promise<void> {
  const config = vscode.workspace.getConfiguration("sonde");
  const command = config.get<string>("path", "sonde");
  const env = config.get<string | null>("env", null) ?? undefined;

  if (!(await binaryIsRunnable(command))) {
    // Shown without blocking activation on the user dismissing it: nothing
    // else here depends on that happening.
    void showBinaryNotFoundError(command);
    return;
  }

  const documentSelector: DocumentSelector = [
    { scheme: "file", language: "sonde" },
    { scheme: "untitled", language: "sonde" },
  ];
  if (config.get<boolean>("associateHurlFiles", false)) {
    documentSelector.push({ scheme: "file", pattern: "**/*.hurl" });
  }

  const serverOptions: ServerOptions = {
    command,
    args: ["lsp"],
    transport: TransportKind.stdio,
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector,
    outputChannel: output,
    initializationOptions: { env },
    synchronize: {
      configurationSection: "sonde",
      fileEvents: vscode.workspace.createFileSystemWatcher("**/sonde.yaml"),
    },
  };

  client = new LanguageClient("sonde", "Sonde Language Server", serverOptions, clientOptions);
  try {
    await client.start();
  } catch (err) {
    client = undefined;
    void vscode.window.showErrorMessage(
      `Sonde: the language server failed to start: ${String(err)}`,
    );
  }
}

/** Resolves quickly and without a shell so a bad `sonde.path` fails fast with a clear message. */
function binaryIsRunnable(command: string): Promise<boolean> {
  return new Promise((resolve) => {
    execFile(command, ["--version"], { timeout: 5000 }, (err) => {
      // ENOENT means the binary was not found; any other outcome (including
      // a non-zero exit, which "--version" should not produce) means a
      // program did run.
      resolve(!(err && (err as NodeJS.ErrnoException).code === "ENOENT"));
    });
  });
}

async function showBinaryNotFoundError(command: string): Promise<void> {
  const openSettings = "Open Settings";
  const choice = await vscode.window.showErrorMessage(
    `Sonde: could not run "${command}". Install the sonde CLI, or set "sonde.path" to its location.`,
    openSettings,
  );
  if (choice === openSettings) {
    await vscode.commands.executeCommand("workbench.action.openSettings", "sonde.path");
  }
}
