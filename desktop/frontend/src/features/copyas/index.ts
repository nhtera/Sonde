// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Copy as: the toolbar menu, and its commands in the palette.

import { registry } from "../../app/registry";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { CopyAsMenu, copyItem, itemsFor } from "./copy-as-menu";
import "./copyas.css";

registry.toolbarItem({ id: "copyas.menu", order: 45, render: CopyAsMenu });

const active = () => useTabs.getState().active;

for (const [id, title] of [
  ["curl.request", "Copy as curl: this request"],
  ["curl.file", "Copy as curl: whole file"],
  ["sonde.file", "Copy as sonde: run this file"],
  ["sonde.to", "Copy as sonde: run to this request"],
  ["sonde.test", "Copy as sonde: test run"],
] as const) {
  registry.command({
    id: `copyas.${id}`,
    title,
    group: "Copy as",
    when: () => !!active(),
    // arg: a file of the tree (its sonde commands need no open tab).
    run: async (arg) => {
      const file = typeof arg === "string" ? arg : active();
      if (!file) return;
      const item = (await itemsFor(file)).find((i) => i.id === id);
      if (item) await copyItem(item);
      else useUI.getState().toast({ kind: "info", text: "Put the cursor in a request first" });
    },
  });
}
