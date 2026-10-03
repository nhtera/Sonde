// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// ⌘K: files, the requests in them (once something is typed) and
// commands; a query starting with ">" lists commands only.

import * as Dialog from "@radix-ui/react-dialog";
import { Command } from "cmdk";
import { useMemo } from "react";
import { label } from "../../app/keymap/keymap-manager";
import { useKeys } from "../../app/keymap/use-keys";
import { registry, useRegistry } from "../../app/registry";
import { SearchIcon } from "../../components/icons";
import { displayPath } from "../../components/run/model";
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
  const index = useWorkspace((s) => s.index);
  const runs = useRuns((s) => s.runs);
  const keys = useKeys();
  const files = useMemo(() => openable(tree), [tree]);
  const commandsOnly = query.startsWith(">");
  const commands = registry.commands().filter((c) => !c.hidden && (!c.when || c.when()));
  const close = () => useUI.getState().closePalette();

  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog palette" aria-describedby={undefined}>
          <Dialog.Title className="sr-only">Search files and commands</Dialog.Title>
          <Command label="Search files and commands" shouldFilter loop filter={paletteFilter}>
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
                        <span className="title">
                          <Match text={name} query={query} />
                        </span>
                        <span className="hint mono">{dir}</span>
                        {run && run.outcome !== "passed" && <span className="state fail">✕ {run.outcome}</span>}
                      </Command.Item>
                    );
                  })}
                </Command.Group>
              )}
              {!commandsOnly && query.trim() && index.length > 0 && (
                <Command.Group heading="Requests">
                  {index.map((r) => {
                    const path = displayPath(r.url);
                    return (
                      <Command.Item
                        key={`${r.file}#${r.entry}`}
                        // What is matched, then (after a tab) what keeps it unique.
                        value={`${r.method} ${path} ${r.title ?? ""}\t${r.file}#${r.entry}`}
                        onSelect={() => {
                          close();
                          void registry.getCommand("editor.revealLine")?.run({ path: r.file, line: r.line });
                        }}
                      >
                        <span className={`mth m-${r.method}`}>{r.method}</span>
                        <span className="title mono">
                          <Match text={path} query={query} />
                        </span>
                        <span className="hint mono">
                          {r.file}:{r.line}
                        </span>
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
                    <span className="title">
                      <Match text={c.title} query={query} />
                      {c.hint && <span className="hint"> {c.hint}</span>}
                    </span>
                    {keys[c.id] && <kbd className="keys">{label(keys[c.id])}</kbd>}
                  </Command.Item>
                ))}
              </Command.Group>
            </Command.List>
            <div className="palette-foot">
              <span>↑↓ move</span>
              <span>↵ open</span>
              <span style={{ flex: 1 }} />
              <span>
                Type <b>&gt;</b> for commands
              </span>
            </div>
          </Command>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** A file, request or command matches when its text holds what is typed
 * (">" for commands aside; a request's file after a tab is not matched);
 * the earlier the match, the higher. */
export function paletteFilter(value: string, search: string): number {
  const q = search.replace(/^>/, "").trim().toLowerCase();
  if (!q) return 1;
  const at = value.split("\t")[0].replace(/^>/, "").toLowerCase().indexOf(q);
  return at < 0 ? 0 : 1 / (1 + at / 100);
}

/** text with the query's first match marked. */
export function Match({ text, query }: { text: string; query: string }) {
  const q = query.replace(/^>/, "").trim().toLowerCase();
  const at = q ? text.toLowerCase().indexOf(q) : -1;
  if (at < 0) return <>{text}</>;
  return (
    <>
      {text.slice(0, at)}
      <b className="match">{text.slice(at, at + q.length)}</b>
      {text.slice(at + q.length)}
    </>
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
