// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Import: curl, Postman, Bruno (OpenCollection), .http and OpenAPI, as
// commands (the tree's Import button opens them in the palette) and the
// dialog they open.

import { registry } from "../../app/registry";
import { serverMode } from "../../lib/mode";
import { useWorkspace } from "../../state/workspace";
import { newProject } from "../shell/new-project";
import { ImportDialog } from "./import-dialog";
import { kinds, useImport } from "./state";
import "./import.css";

registry.slot("app.overlays", { id: "import.dialog", order: 1, render: ImportDialog });

const titles: Record<string, string> = {
  curl: "Import curl command…",
  postman: "Import Postman collection…",
  opencollection: "Import Bruno / OpenCollection…",
  http: "Import .http file…",
  openapi: "Import OpenAPI spec…",
};

// ⌘I and the welcome screen: the dialog (a project first: an import
// writes into one; with none open, one is made, or an existing folder
// picked).
registry.command({
  id: "import.open",
  title: "Import…",
  group: "Import",
  run: async () => {
    if (!useWorkspace.getState().project && !serverMode) await newProject(true);
    if (useWorkspace.getState().project) useImport.getState().show("postman");
  },
});

// A curl command from the clipboard, into the open curl form.
registry.command({
  id: "import.paste",
  title: "Import pasted curl",
  hidden: true,
  run: (text) => {
    if (typeof text === "string" && useImport.getState().open) useImport.getState().update({ text });
  },
});

for (const k of kinds) {
  registry.command({
    id: `import.${k.kind}`,
    title: titles[k.kind],
    group: "Import",
    when: () => !!useWorkspace.getState().project,
    run: () => useImport.getState().show(k.kind),
  });
}
