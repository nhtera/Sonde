// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// ⌘K: files and commands; a query starting with ">" lists commands only.

import * as Dialog from "@radix-ui/react-dialog";
import { Command } from "cmdk";
import { useMemo } from "react";
import { label } from "../../app/keymap/keymap-manager";
import { useKeys } from "../../app/keymap/use-keys";
import { registry, useRegistry } from "../../app/registry";
import { SearchIcon } from "../../components/icons";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { useWorkspace } from "../../state/workspace";
import type { Node } from "../../lib/api";

export function Palette() {
  useRegistry();
  const open = useUI((s) => s.paletteOpen);
  const query = useUI((s) => s.paletteQuery);
  const tree = useWorkspace((s) => s.tree);
  const runs = useRuns((s) => s.runs);
  const keys = useKeys();
  const files = useMemo(() => openable(tree), [tree]);
  const commandsOnly = query.startsWith(">");
  const commands = registry.commands().filter((c) => !c.hidden && (!c.when || c.when()));
  const close = () => useUI.getState().closePalette();

  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" style={{ background: "transparent" }} />
        <Dialog.Content className="dialog palette" aria-describedby={undefined}>
          <Dialog.Title className="sr-only">Search files and commands</Dialog.Title>
          <Command label="Search files and commands" shouldFilter loop>
            <div className="palette-input">
              <SearchIcon size={14} />
              <Command.Input
                autoFocus
                value={query}
                onValueChange={(v) => useUI.setState({ paletteQuery: v })}
                placeholder="Search files and commands"
              />
              <kbd>Esc</kbd>
            </div>
            <Command.List className="palette-list">
              <Command.Empty className="palette-empty">No matches</Command.Empty>
              {!commandsOnly && files.length > 0 && (
                <Command.Group heading="Files">
                  {files.map((f) => {
                    const run = runs[f]?.summary;
                    const name = f.split("/").at(-1)!;
                    const dir = f.slice(0, f.length - name.length);
                    return (
                      <Command.Item
                        key={f}
                        value={f}
                        onSelect={() => {
                          close();
                          void useTabs.getState().open(f);
                        }}
                      >
                        <i className="ic" />
                        <span className="title">{name}</span>
                        <span className="hint mono">{dir}</span>
                        {run && run.outcome !== "passed" && <span className="state fail">✕ {run.outcome}</span>}
                      </Command.Item>
                    );
                  })}
                </Command.Group>
              )}
              <Command.Group heading="Commands">
                {commands.map((c) => (
                  <Command.Item
                    key={c.id}
                    value={`>${c.title} ${c.hint ?? ""}`}
                    onSelect={() => {
                      close();
                      void c.run();
                    }}
                  >
                    <span className="chev">›</span>
                    <span className="title">{c.title}</span>
                    {c.hint && <span className="hint">{c.hint}</span>}
                    {keys[c.id] && <kbd className="keys">{label(keys[c.id])}</kbd>}
                  </Command.Item>
                ))}
              </Command.Group>
            </Command.List>
            <div className="palette-foot">
              <span>↑↓ move</span>
              <span>↵ open</span>
              <span style={{ flex: 1 }} />
              <span>Type &gt; for commands</span>
            </div>
          </Command>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** The files the palette lists: every file but secrets files, by path. */
export function openable(tree: Node | null): string[] {
  const out: string[] = [];
  const walk = (n: Node | null) => {
    if (!n) return;
    if (n.kind !== "dir") {
      if (n.kind !== "secrets") out.push(n.path);
      return;
    }
    n.children?.forEach(walk);
  };
  tree?.children?.forEach(walk);
  return out.sort();
}
