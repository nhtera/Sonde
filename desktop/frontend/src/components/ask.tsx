// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Small in-app prompts: a line of text, or a yes/no question. Native
// prompt() and confirm() are not available in every webview (the macOS
// one answers confirm() with false at once).

import * as Dialog from "@radix-ui/react-dialog";
import { useState } from "react";
import { create } from "zustand";

interface AskOptions {
  title: string;
  /** The text field's label; a confirmation has none. */
  label?: string;
  value?: string;
  /** A confirmation's question. */
  message?: string;
  submit?: string;
}

interface AskState {
  current: (AskOptions & { resolve(v: string | null): void }) | null;
}

const useAsk = create<AskState>(() => ({ current: null }));

function open(o: AskOptions): Promise<string | null> {
  // A prompt still open is canceled by the new one.
  useAsk.getState().current?.resolve(null);
  return new Promise((resolve) => useAsk.setState({ current: { ...o, resolve } }));
}

/** Asks for a line of text; null when canceled. */
export function ask(o: AskOptions & { label: string }): Promise<string | null> {
  return open(o);
}

/** Asks a yes/no question; true when confirmed. */
export async function confirm(o: { title: string; message: string; submit: string }): Promise<boolean> {
  return (await open(o)) !== null;
}

export function AskHost() {
  const cur = useAsk((s) => s.current);
  if (!cur) return null;
  return <AskDialog key={cur.title + (cur.value ?? cur.message)} {...cur} />;
}

function AskDialog(o: AskOptions & { resolve(v: string | null): void }) {
  const [value, setValue] = useState(o.value ?? "");
  const close = (v: string | null) => {
    useAsk.setState({ current: null });
    o.resolve(v);
  };
  return (
    <Dialog.Root open onOpenChange={(open) => !open && close(null)}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog dialog-pad" style={{ width: 420 }} aria-describedby={undefined}>
          <Dialog.Title className="dialog-title">{o.title}</Dialog.Title>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              close(o.label === undefined ? "" : value.trim() || null);
            }}
            style={{ display: "flex", flexDirection: "column", gap: 12 }}
          >
            {o.label !== undefined ? (
              <label className="filter" style={{ margin: 0 }}>
                <input aria-label={o.label} autoFocus value={value} onChange={(e) => setValue(e.target.value)} spellCheck={false} />
              </label>
            ) : (
              <p className="dialog-note" style={{ margin: 0 }}>
                {o.message}
              </p>
            )}
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
              <button type="button" className="btn-ghost" onClick={() => close(null)}>
                Cancel
              </button>
              <button type="submit" className="btn-primary" style={{ paddingRight: 10 }} autoFocus={o.label === undefined}>
                {o.submit ?? "OK"}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
