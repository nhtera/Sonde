// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Updates (the window app): the dialog and the palette's check.

import { registry } from "../../app/registry";
import { Update, appError } from "../../lib/api";
import { updatesOn } from "../../lib/mode";
import { useUI } from "../../state/ui";
import { UpdateDialog } from "./update-dialog";
import "./update.css";

if (updatesOn) {
  registry.slot("app.overlays", { id: "update.dialog", order: 2, render: UpdateDialog });
  registry.command({
    id: "app.checkUpdates",
    title: "Check for updates…",
    run: () => Update.Check().catch((err) => useUI.getState().toast({ kind: "error", text: appError(err).message })),
  });
}
