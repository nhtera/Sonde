// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Import dialog: a kind (curl, Postman, Bruno, .http, OpenAPI), its
// input and options with a live preview, then the result, then (Postman)
// the optional suggestions. Nothing is sent: importing only writes files.

import * as Dialog from "@radix-ui/react-dialog";
import { ImportResult } from "./result-tile";
import { SourceForm } from "./source-form";
import { kinds, useImport } from "./state";
import { SuggestionsStep } from "./suggestions-step";

export function ImportDialog() {
  const open = useImport((s) => s.open);
  const step = useImport((s) => s.step);
  const kind = useImport((s) => s.req.kind);
  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && useImport.getState().close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className={`dialog dialog-pad import-dialog${step === "suggestions" ? " wide" : ""}`} aria-describedby={undefined}>
          {step === "form" && (
            <>
              <Dialog.Title className="dialog-title">Import</Dialog.Title>
              <p className="muted small">Converted the way `sonde import` does. Nothing is sent.</p>
              <div className="segmented import-kinds" role="tablist" aria-label="Import from">
                {kinds.map((k) => (
                  <button key={k.kind} role="tab" aria-selected={kind === k.kind} onClick={() => useImport.getState().update({ kind: k.kind })}>
                    {k.title}
                  </button>
                ))}
              </div>
              <SourceForm />
            </>
          )}
          {step === "result" && <ImportResult />}
          {step === "suggestions" && <SuggestionsStep />}
          <Dialog.Close className="btn-ghost close">Esc</Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
