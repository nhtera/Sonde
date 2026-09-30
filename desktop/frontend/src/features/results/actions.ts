// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What the Results panel's buttons do to the file: asserts and captures
// from a response (Go renders their text), max-time, a retry, the mock.
// Every edit applies to the tab's text as one undoable change.

import { registry } from "../../app/registry";
import { ask } from "../../components/ask";
import { appError, EditSvc, Mocks } from "../../lib/api";
import { applyEdit, hasEditTarget, lineAt, undoEdit, type EditResult } from "../../state/edits";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";

const toast = (kind: "success" | "error" | "warn" | "info", text: string, action?: { label: string; run: () => void }) =>
  useUI.getState().toast({ kind, text, action });

/** The buffer of an open file. */
function buffer(file: string) {
  const tab = useTabs.getState().tabs.find((t) => t.path === file);
  return tab ? { file, text: tab.text, version: tab.version } : null;
}

/** Runs an edit computed from the tab's current text and applies it; one
 * retry when the text changed meanwhile. Returns the result applied. */
async function edit(file: string, compute: (b: { file: string; text: string; version: number }) => Promise<EditResult | null>) {
  for (let attempt = 0; attempt < 2; attempt++) {
    const b = buffer(file);
    if (!b) return null;
    const res = await compute(b);
    if (res && applyEdit(file, res)) return res;
  }
  toast("warn", "The file changed while the edit was prepared; try again");
  return null;
}

/** The line the edit's new text starts on. */
function editLine(res: EditResult): number {
  const e = res.edits[0];
  if (!e) return 0;
  const lead = /^\s*/.exec(e.NewText)?.[0].length ?? 0;
  return lineAt(res.text, e.Range.Start + lead);
}

const tabText = (file: string) => useTabs.getState().tabs.find((t) => t.path === file)?.text;

/** Reports an applied edit, with an Undo of that edit only: once the file
 * changed again, ⌘Z in the editor is the way back. */
function done(file: string, what: string, res: EditResult | null) {
  if (!res) return;
  const after = tabText(file);
  const undo = () => {
    if (tabText(file) === after) undoEdit(file);
    else toast("info", "The file changed since: undo in the editor with ⌘Z");
  };
  toast("success", `${what} at line ${editLine(res)}`, hasEditTarget(file) ? { label: "Undo ⌘Z", run: undo } : undefined);
}

const failed = (err: unknown) => toast("error", appError(err).message);

/** Adds an assert on the value at path of a response body; with capture,
 * a capture of that name instead (asked when "?"). */
export async function addFromBody(file: string, entry: number, bodyId: string, path: string, capture?: string) {
  try {
    let name = capture ?? "";
    if (capture === "?") {
      const answer = await ask({ title: "Capture this value", label: "Variable name", submit: "Add capture" });
      if (!answer) return;
      name = answer.trim();
    }
    const res = await edit(file, (b) => EditSvc.AssertValue(b, { entry, bodyId, path, capture: name }) as Promise<EditResult | null>);
    done(file, name ? "Capture added" : "Assert added", res);
  } catch (err) {
    failed(err);
  }
}

/** Adds an assert written as text (e.g. `cookie "sid" exists`). */
export async function addAssert(file: string, entry: number, text: string) {
  try {
    const res = await edit(file, (b) => EditSvc.Apply(b, { kind: "addAssert", entry, section: "", index: 0, key: "", value: text }) as Promise<EditResult | null>);
    done(file, "Assert added", res);
  } catch (err) {
    failed(err);
  }
}

/** Sets the entry's max-time option (adds it, or changes it). */
export async function setMaxTime(file: string, entry: number, value: string) {
  try {
    const res = await edit(file, async (b) => {
      const model = (await EditSvc.Model(b)) ?? [];
      const rows = model.find((m) => m.Index === entry)?.Rows?.options ?? [];
      const index = rows.findIndex((r) => r.Key === "max-time");
      const op =
        index >= 0
          ? { kind: "setRow", entry, section: "options", index, key: "max-time", value }
          : { kind: "addRow", entry, section: "options", index: 0, key: "max-time", value };
      return EditSvc.Apply(b, op) as Promise<EditResult | null>;
    });
    done(file, `max-time: ${value} set`, res);
  } catch (err) {
    failed(err);
  }
}

/** Runs the entry again the way it ran: a Send again, else the file. */
export function retry(file: string, entry: number) {
  const run = useRuns.getState().runs[file];
  if (run?.kind === "send") void useRuns.getState().send(file, entry);
  else void useRuns.getState().run(file);
}

let specFile: Promise<string> | null = null;

/** The project's OpenAPI spec file ("" for none), asked once per page
 * (a new project reloads it). */
export function mockSpec(): Promise<string> {
  specFile ??= Mocks.Spec().then(
    (s) => s?.file ?? "",
    () => "",
  );
  return specFile;
}

/** Forgets the spec (another project, sonde.yaml edited). */
export function resetMockSpec() {
  specFile = null;
}

/** Starts the mock (base_url then points at it) and runs the entry again;
 * a busy port moves to the next free one. */
export async function startMock(file: string, entry: number, port = 4010) {
  try {
    let st = await Mocks.Status();
    if (st?.running) {
      retry(file, entry);
      return;
    }
    try {
      st = await Mocks.Start(port);
    } catch (err) {
      const e = appError(err);
      const next = (e.data as { next?: number } | undefined)?.next;
      if (e.code !== "busy" || !next) throw err;
      st = await Mocks.Start(next);
    }
    toast("info", `Mock running at ${st?.url ?? ""}; base_url points to it`);
    retry(file, entry);
  } catch (err) {
    failed(err);
  }
}

/** Runs a command another feature registered (a no-op until it does). */
export function runCommand(id: string, arg?: unknown) {
  void registry.getCommand(id)?.run(arg);
}

/** Whether a feature registered command id (its button shows only then). */
export const hasCommand = (id: string) => !!registry.getCommand(id);
