// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// "Data none ▾" in the editor's toolbar: picks one of the project's data
// files (.csv, .json); Run file then runs the file once per row, as
// `sonde --data FILE` does.

import { ChevronDown } from "../../components/icons";
import * as Menu from "@radix-ui/react-dropdown-menu";
import type { Node } from "../../lib/api";
import { useRuns } from "../../state/run";
import { useWorkspace } from "../../state/workspace";

/** The project's data files, by path. */
export function dataFiles(tree: Node | null): string[] {
  const out: string[] = [];
  const walk = (n: Node | null) => {
    if (!n) return;
    if (n.kind === "data") out.push(n.path);
    n.children?.forEach(walk);
  };
  walk(tree);
  return out.sort();
}

export function DataPicker({ file }: { file: string }) {
  const tree = useWorkspace((s) => s.tree);
  const data = useRuns((s) => s.dataFiles[file] ?? "");
  const files = dataFiles(tree);
  const pick = (path: string) => useRuns.getState().setDataFile(file, path);
  return (
    <Menu.Root>
      <Menu.Trigger className="btn-ghost data-picker" aria-label={`Data file: ${data || "none"}`} title="Run the file once per row of a data file">
        <span className="muted">Data</span>
        <span className={`value${data ? " mono" : ""}`}>{data ? data.split("/").at(-1) : "none"}</span>
        <ChevronDown />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="menu" align="start" sideOffset={4}>
          <Menu.Label className="menu-label">Run once per row of</Menu.Label>
          <Menu.RadioGroup value={data} onValueChange={pick}>
            <Menu.RadioItem className="menu-item" value="">
              none
            </Menu.RadioItem>
            {files.map((f) => (
              <Menu.RadioItem key={f} className="menu-item mono" value={f}>
                {f}
              </Menu.RadioItem>
            ))}
          </Menu.RadioGroup>
          {files.length === 0 && <div className="menu-note">No .csv or .json files in the project.</div>}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
