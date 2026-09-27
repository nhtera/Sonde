// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { execFile } from "node:child_process";
import { constants as fsConstants, promises as fsp } from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
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

  const resolved = await resolveOnPath(command);
  if (!resolved || !(await binaryIsRunnable(resolved))) {
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
    command: resolved,
    args: ["lsp"],
    transport: TransportKind.stdio,
    // Never the workspace root: vscode-languageclient defaults to it, and
    // on Windows a bare command is searched for in the spawned process's
    // cwd before PATH, which `resolved` above already sidesteps — this is
    // belt-and-suspenders in case that assumption ever changes. The LSP
    // resolves workspace-relative paths from `rootUri`/`workspaceFolders`,
    // never from the server process's own cwd, so this is safe.
    options: { cwd: os.tmpdir() },
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

/**
 * Resolves `command` to an absolute path found on `PATH`, never considering
 * the current working directory or any workspace folder. This matters on
 * Windows: without an explicit `cwd`, `child_process` (and the libuv spawn
 * vscode-languageclient uses) searches a spawned process's cwd for a bare
 * command name before `PATH`. Left unresolved, a `sonde.exe` committed at
 * the root of an untrusted workspace would run instead of the real one on
 * `PATH`. An already-absolute `command` is returned unchanged: `sonde.path`
 * is a restricted configuration, so an untrusted workspace can't set it,
 * and an explicit absolute override should fail with a clear "not found"
 * if it's wrong rather than silently falling back to a `PATH` match.
 */
async function resolveOnPath(command: string): Promise<string | undefined> {
  if (path.isAbsolute(command)) {
    return command;
  }
  const dirs = (process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
  const names =
    process.platform === "win32"
      ? [command, ...(process.env.PATHEXT ?? ".EXE;.CMD;.BAT").split(";").map((ext) => command + ext)]
      : [command];
  for (const dir of dirs) {
    for (const name of names) {
      const candidate = path.join(dir, name);
      try {
        await fsp.access(candidate, fsConstants.X_OK);
        return candidate;
      } catch {
        // Not here; keep looking.
      }
    }
  }
  return undefined;
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
