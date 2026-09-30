// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The language client's transport over the app's LSP session: messages go
// out with Lsp.Send and come back on the "lsp:<id>" event.

import type { Transport } from "@codemirror/lsp-client";
import { Lsp } from "../../../lib/api";
import { on } from "../../../lib/events";

export class WailsTransport implements Transport {
  private handlers = new Set<(message: string) => void>();
  private off: () => void;
  private closed = false;
  /** Sends go one after the other: concurrent binding calls may arrive in
   * any order, and the server takes whole documents (an older text must
   * never land last). */
  private chain: Promise<unknown> = Promise.resolve();

  constructor(private session: string) {
    this.off = on(`lsp:${session}`, (data) => {
      const message = typeof data === "string" ? data : JSON.stringify(data);
      this.handlers.forEach((h) => h(message));
    });
  }

  send(message: string): void {
    if (this.closed) throw new Error("the language server session ended");
    // A failed send (the session ended) surfaces as a request timeout;
    // the session manager opens a new one.
    this.chain = this.chain.then(() => Lsp.Send(this.session, message)).catch(() => undefined);
  }

  subscribe(handler: (message: string) => void): void {
    this.handlers.add(handler);
  }

  unsubscribe(handler: (message: string) => void): void {
    this.handlers.delete(handler);
  }

  close(): void {
    this.closed = true;
    this.off();
    this.handlers.clear();
  }
}
