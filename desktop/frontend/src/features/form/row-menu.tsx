// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A row's "…" menu in the Form view: turn it off (a # comment) or on,
// duplicate it (added at the end of its section, off if it is off),
// delete it.

import * as Menu from "@radix-ui/react-dropdown-menu";
import { formEdit, type Op } from "./edit";
import type { Sec } from "./model";

export function RowMenu({ file, entry, section, index, count, label, disabled, copy }: {
  file: string;
  entry: number;
  section: Sec;
  index: number;
  /** The section's rows: a copy is added at this index. */
  count: number;
  /** What the row is, for the menu's name ("assert 2", "Accept"). */
  label: string;
  disabled: boolean;
  /** The op that adds a copy of the row. */
  copy: Op;
}) {
  // A copy of a row turned off stays off: added, then turned off.
  const duplicate = async () => {
    if ((await formEdit(file, copy)) && disabled) await formEdit(file, { kind: "toggleRow", entry, section, index: count });
  };
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button className="kv-remove row-menu" aria-label={`${label} actions`} title="More">
          ⋯
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="menu" align="end" sideOffset={4}>
          <Menu.Item className="menu-item" onSelect={() => void formEdit(file, { kind: "toggleRow", entry, section, index })}>
            {disabled ? "Enable" : "Disable"}
            <span className="hint">{disabled ? "uncomment" : "# comment"}</span>
          </Menu.Item>
          <Menu.Item className="menu-item" onSelect={() => void duplicate()}>
            Duplicate
          </Menu.Item>
          <Menu.Separator className="menu-sep" />
          <Menu.Item className="menu-item danger" onSelect={() => void formEdit(file, { kind: "removeRow", entry, section, index })}>
            Delete
          </Menu.Item>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
