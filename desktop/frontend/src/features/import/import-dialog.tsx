// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Import dialog: a kind (curl, Postman, Bruno, .http, OpenAPI), its
// input and options with a live preview, then the result, then (a collection)
// the optional suggestions. Nothing is sent: importing only writes files.

import * as Dialog from "@radix-ui/react-dialog";
import { ImportResult } from "./result-tile";
import { SourceForm } from "./source-form";
import { collectionPicked, kinds, useImport } from "./state";
import { SuggestionsStep } from "./suggestions-step";

export function ImportDialog() {
  const open = useImport((s) => s.open);
  const step = useImport((s) => s.step);
  const kind = useImport((s) => s.req.kind);
  // A collection picked: its preview, without the source choices.
  const picked = useImport(collectionPicked);
  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && useImport.getState().close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className={`dialog import-dialog${step === "suggestions" ? " wide" : step === "result" ? " result" : step === "form" && picked ? " compact" : ""}`} aria-describedby={undefined}>
          {step === "form" && (
            <>
              <ImportTitle />
              {!picked && (
                <div className="import-kinds" role="tablist" aria-label="Import from">
                  {kinds.map((k) => (
                    <button key={k.kind} role="tab" aria-selected={kind === k.kind} onClick={() => useImport.getState().update({ kind: k.kind })}>
                      {k.title}
                    </button>
                  ))}
                </div>
              )}
              <SourceForm />
            </>
          )}
          {step === "result" && (
            <div className="import-step">
              <ImportResult />
            </div>
          )}
          {step === "suggestions" && (
            <div className="import-step">
              <SuggestionsStep />
            </div>
          )}
          <Dialog.Close className="close">Esc</Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** "Import", or for a collection "Import “Shop API” from Postman" with
 * what it holds. */
function ImportTitle() {
  const preview = useImport((s) => s.preview);
  const req = useImport((s) => s.req);
  const names = useImport((s) => s.names);
  const name = req.kind === "postman" ? preview?.name : "";
  const c = preview?.counts;
  if (!name || !c) {
    return (
      <>
        <div className="import-head">
          <Dialog.Title className="dialog-title">Import</Dialog.Title>
        </div>
      </>
    );
  }
  const from = req.input ? `${names[req.input] ?? "the file picked"} · ` : "";
  const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? "" : "s"}`;
  return (
    <>
      <div className="import-head">
        <Dialog.Title className="dialog-title">{`Import \u201c${name}\u201d from Postman`}</Dialog.Title>
        <p>
          {from}
          {plural(c.requests, "request")} in {plural(c.folders, "folder")} · {plural(c.environments, "environment")}. Nothing is sent.{" "}
          {req.input && (
            <button className="btn-ghost accent import-change" onClick={() => useImport.getState().update({ input: "" })}>
              Change file
            </button>
          )}
        </p>
      </div>
    </>
  );
}

