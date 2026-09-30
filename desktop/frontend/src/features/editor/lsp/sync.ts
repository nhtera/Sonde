// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Sends edits to the language server after 200 ms without typing (the
// server takes whole documents on one loop, so every keystroke would queue
// work).

import { LSPPlugin } from "@codemirror/lsp-client";
import { ViewPlugin, type ViewUpdate } from "@codemirror/view";
import { lspReady } from "./client";

export const SYNC_DELAY = 200;

export const lspSync = ViewPlugin.fromClass(
  class {
    private timer: ReturnType<typeof setTimeout> | undefined;

    update(u: ViewUpdate) {
      if (!u.docChanged) return;
      clearTimeout(this.timer);
      this.timer = setTimeout(() => {
        const plugin = LSPPlugin.get(u.view);
        if (plugin && lspReady()) plugin.client.sync();
      }, SYNC_DELAY);
    }

    destroy() {
      clearTimeout(this.timer);
    }
  },
);
