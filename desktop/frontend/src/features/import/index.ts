// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Import: curl, Postman, Bruno (OpenCollection), .http and OpenAPI, as
// commands (the tree's Import button opens them in the palette) and the
// dialog they open.

import { registry } from "../../app/registry";
import { useWorkspace } from "../../state/workspace";
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

for (const k of kinds) {
  registry.command({
    id: `import.${k.kind}`,
    title: titles[k.kind],
    group: "Import",
    when: () => !!useWorkspace.getState().project,
    run: () => useImport.getState().show(k.kind),
  });
}
