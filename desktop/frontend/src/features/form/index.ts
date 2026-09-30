// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Form view: the second view of a file (⌘E switches), registered as
// an editor of the main area. While it shows, Send (⌘↵) and Run to (⇧⌘↵)
// act on the request it shows, and runs and saves first commit the field
// being typed in.

import { registry } from "../../app/registry";
import { registerFlusher } from "../../state/edits";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { flushForm } from "./edit";
import { FormView } from "./form-view";
import { shownEntry, useForm } from "./model";

registry.editor({ id: "form", title: "Form", order: 1, render: FormView });

registerFlusher(flushForm);

/** The file and request the Form shows, while it is the view. */
function formRequest(): { file: string; entry: number } | null {
  if (useUI.getState().editorView !== "form") return null;
  const file = useTabs.getState().active;
  if (!file) return null;
  const text = useTabs.getState().tabs.find((t) => t.path === file)?.text ?? "";
  const e = shownEntry(useForm.getState().models[file], useForm.getState().entry[file], text, useUI.getState().cursor?.line);
  return e ? { file, entry: e.Index } : null;
}

// Send and Run to act on the form's request in Form, the cursor's in Text.
for (const id of ["request.send", "request.runTo"]) {
  const text = registry.getCommand(id);
  if (!text) continue;
  registry.command({
    ...text,
    when: () => {
      const f = formRequest();
      return f ? !useRuns.getState().runs[f.file]?.running : (text.when?.() ?? true);
    },
    run: (arg) => {
      const f = formRequest();
      if (!f) return text.run(arg);
      return id === "request.send" ? useRuns.getState().send(f.file, f.entry) : useRuns.getState().run(f.file, f.entry);
    },
  });
}
