// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// One language client for the page, connected to the app's LSP session
// (reconnected when the session is replaced) and rooted at the project; a
// new project gets a new session (a server is initialized once). The
// environment and the extra names the server must treat as defined go
// through Lsp.Configure once the server is initialized.

import { LSPClient, serverDiagnostics } from "@codemirror/lsp-client";
import { Lsp } from "../../../lib/api";
import { onLspSession, restartLspSession } from "../../../lib/lsp-session";
import { useEnv } from "../../../state/env";
import { useWorkspace } from "../../../state/workspace";
import { sanitizeHTML } from "./sanitize";
import { WailsTransport } from "./wails-transport";

let client: LSPClient | null = null;
let root: string | null = null;
let session: string | null = null;
let transport: WailsTransport | null = null;
/** The client's server answered initialize (on the current session). */
let ready = false;
/** The client has a transport (the library's own flag stays set after a
 * disconnect, and disconnecting twice would orphan its pending requests). */
let connected = false;

/** The file URI of a project path. */
export function fileURI(path: string): string {
  const dir = useWorkspace.getState().project?.dir ?? "";
  const abs = `${dir.replace(/\\/g, "/").replace(/\/$/, "")}/${path}`;
  return "file://" + (abs.startsWith("/") ? "" : "/") + abs.split("/").map(encodeURIComponent).join("/");
}

const diagnostics = serverDiagnostics();

/** Whether the language server can answer now. */
export function lspReady(): boolean {
  return ready;
}

/** The client for the open project (created on first use). */
export function lspClient(): LSPClient {
  const dir = useWorkspace.getState().project?.dir ?? "";
  if (client && root === dir) return client;
  const hadProject = root !== null;
  disconnect();
  root = dir;
  client = new LSPClient({
    rootUri: fileURI("").replace(/\/$/, ""),
    timeout: 5000,
    sanitizeHTML,
    // Diagnostics without the library's 500 ms sync: sync.ts syncs after
    // 200 ms of idle.
    extensions: [{ clientCapabilities: diagnostics.clientCapabilities, notificationHandlers: diagnostics.notificationHandlers }],
  });
  // Another project: its own server (the session's is initialized for the
  // previous one). The new session connects this client.
  if (hadProject && session) restartLspSession();
  else if (session) connect(session);
  return client;
}

function disconnect() {
  ready = false;
  transport?.close();
  transport = null;
  if (connected) client?.disconnect();
  connected = false;
}

function connect(id: string) {
  if (!client) return;
  disconnect();
  transport = new WailsTransport(id);
  const c = client.connect(transport);
  connected = true;
  lastConfig = "";
  c.initializing.then(
    () => {
      if (client !== c || session !== id) return;
      ready = true;
      void configure();
    },
    () => undefined,
  );
}

/** Names the server treats as defined beyond the files: the session's
 * overrides. (A file's own captures it knows; another file's run does not
 * share its captures: runs and sessions are per file.) */
export function extraVariables(): string[] {
  const names = new Set<string>();
  for (const o of useEnv.getState().overrides.items ?? []) if (o.name) names.add(o.name);
  return [...names].sort();
}

let lastConfig = "";

/** Sends the environment and the extra names, when they changed. */
export async function configure(): Promise<void> {
  if (!session || !ready) return;
  const env = useEnv.getState().current;
  const extra = extraVariables();
  const key = `${session}\n${env}\n${extra.join(",")}`;
  if (key === lastConfig) return;
  lastConfig = key;
  await Lsp.Configure(session, env, extra).catch(() => {
    lastConfig = "";
  });
}

onLspSession((id) => {
  session = id;
  if (id && client) connect(id);
  else if (!id) disconnect();
});

// The environment and overrides change what is defined.
useEnv.subscribe((s, prev) => {
  if (s.current !== prev.current || s.overrides !== prev.overrides) void configure();
});
