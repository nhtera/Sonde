// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Every form change is a structured edit Go makes on the tab's text
// (editsvc: comments and layout kept), applied to the editor as one change
// (⌘Z undoes it). A form action of several edits (a body kind, an auth
// type) is one batch. Ops name rows by index in the model the form shows:
// when the text has moved on since (a quick second click, typing in
// Text), the edit is refused rather than sent to the wrong row.

import { appError, EditSvc, type EditOp } from "../../lib/api";
import { applyEdit, type EditResult } from "../../state/edits";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { loadModel, useForm, type Sec } from "./model";

export type Op = Partial<Omit<EditOp, "kind" | "section">> & { kind: string; section?: Sec };

const full = (op: Op): EditOp => ({ entry: 0, index: 0, key: "", value: "", ...op, section: op.section ?? "" }) as EditOp;

/** Edits still being made, per file (Send, Run and Save wait for them). */
const pending = new Map<string, Set<Promise<boolean>>>();

/** Runs ops on file as one change; false (with a message) when refused. */
export function formEdit(file: string, ...ops: Op[]): Promise<boolean> {
  if (ops.length === 0) return Promise.resolve(true);
  // The model the ops were built from: the one the form shows now. Only
  // ops that name a row by index depend on it; the others wait for the
  // edits before them and apply to the text as it is then.
  const built = ops.some((o) => indexed.has(o.kind)) ? useForm.getState().models[file]?.version : undefined;
  const before = [...(pending.get(file) ?? [])];
  const p = (built === undefined ? Promise.all(before) : Promise.resolve()).then(() => run(file, built, ops));
  const set = pending.get(file) ?? new Set();
  pending.set(file, set);
  set.add(p);
  void p.finally(() => set.delete(p));
  return p;
}

/** Ops that name a row by its index in the model. */
const indexed = new Set(["setRow", "removeRow", "toggleRow"]);

async function run(file: string, built: number | undefined, ops: Op[]): Promise<boolean> {
  try {
    const tab = useTabs.getState().tabs.find((t) => t.path === file);
    if (!tab) return false;
    if (built !== undefined && built !== tab.version) {
      await loadModel(file);
      useUI.getState().toast({ kind: "warn", text: "The form was catching up with the file: try again" });
      return false;
    }
    const b = { file, text: tab.text, version: tab.version };
    const res = (ops.length === 1 ? await EditSvc.Apply(b, full(ops[0])) : await EditSvc.Batch(b, ops.map(full))) as EditResult | null;
    if (!res) return false;
    if (res.text === tab.text || applyEdit(file, res)) {
      // The form reads the new text at once, not after a pause.
      await loadModel(file);
      return true;
    }
    await loadModel(file);
    useUI.getState().toast({ kind: "warn", text: "The file changed while the edit was prepared: try again" });
  } catch (err) {
    useUI.getState().toast({ kind: "error", text: appError(err).message });
  }
  return false;
}

/** Commits the field being edited in the form (it commits on blur) and
 * waits for every edit of file: what runs or is saved is what the form
 * shows. */
export function flushForm(file: string): Promise<void> | null {
  const active = document.activeElement as HTMLElement | null;
  if (active?.closest(".form-view")) active.blur();
  // A blur's commit starts its edit at once; wait for all of them.
  if (!pending.get(file)?.size) return null;
  return (async () => {
    for (let i = 0; i < 3 && pending.get(file)?.size; i++) await Promise.all(pending.get(file)!);
  })();
}

/** Sets row i of sec (or adds it, i < 0); a keyed row whose key is
 * cleared is removed. Asserts have no key: they are set as text. */
export function setRow(file: string, entry: number, sec: Sec, i: number, key: string, value: string) {
  const keyed = sec !== "asserts";
  if (i < 0) return key || !keyed ? formEdit(file, { kind: "addRow", entry, section: sec, key, value }) : Promise.resolve(true);
  if (!key && keyed) return formEdit(file, { kind: "removeRow", entry, section: sec, index: i });
  return formEdit(file, { kind: "setRow", entry, section: sec, index: i, key, value });
}
