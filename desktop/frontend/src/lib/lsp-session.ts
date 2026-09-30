// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The page's one language server session: opened at start, kept alive by a
// ping, closed when the page unloads, and reopened when it ends. The
// language client (src/lang) attaches with onLspSession.

import { Lsp } from "./api";
import { on } from "./events";

type Listener = (id: string | null) => void;

const PING_MS = 60_000;
let current: string | null = null;
const listeners = new Set<Listener>();

/** The open session's id, or null. */
export function lspSession(): string | null {
  return current;
}

/** Calls l with each new session's id, and null when one ends. */
export function onLspSession(l: Listener): () => void {
  listeners.add(l);
  if (current) l(current);
  return () => listeners.delete(l);
}

function set(id: string | null) {
  current = id;
  listeners.forEach((l) => l(id));
}

/** Opens the session and keeps it; returns the stop function. */
export function startLspSession(): () => void {
  let stopped = false;
  let retry = 1000;
  let offClosed: (() => void) | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const open = async () => {
    if (stopped) return;
    try {
      const id = await Lsp.Open();
      if (stopped) {
        void Lsp.Close(id);
        return;
      }
      retry = 1000;
      offClosed?.();
      offClosed = on(`lsp:${id}:closed`, () => {
        if (current === id) reopen();
      });
      set(id);
    } catch {
      reopen();
    }
  };
  const reopen = () => {
    set(null);
    clearTimeout(timer);
    timer = setTimeout(() => void open(), retry);
    retry = Math.min(retry * 2, 30_000);
  };
  const ping = setInterval(() => {
    if (current) Lsp.Ping(current).catch(() => reopen());
  }, PING_MS);
  const unload = () => {
    if (current) void Lsp.Close(current);
  };
  window.addEventListener("beforeunload", unload);
  void open();

  return () => {
    stopped = true;
    clearInterval(ping);
    clearTimeout(timer);
    offClosed?.();
    window.removeEventListener("beforeunload", unload);
    unload();
    set(null);
  };
}
