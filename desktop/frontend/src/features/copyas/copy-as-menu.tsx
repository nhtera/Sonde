// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Copy as: curl for a request or the file, and the sonde commands that run
// the file as the app does (--file-root ., the env, a flag per override).
// Secrets are references to environment variables; holding ⌥ (desktop
// only) copies them with their values, after a warning, through a
// concealed clipboard Go clears after 60 s.

import * as Menu from "@radix-ui/react-dropdown-menu";
import { useEffect, useState } from "react";
import { confirm } from "../../components/ask";
import { appError, CopyAs, CopyAsReveal } from "../../lib/api";
import { serverMode } from "../../lib/mode";
import { useEnv } from "../../state/env";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { entryAt } from "../editor/entry-at";
import { activeView } from "../editor/views";
import { copyItems, shellOf, type CopyItem } from "./model";

const toast = (kind: "success" | "error" | "info", text: string) => useUI.getState().toast({ kind, text });

/** The items for file, the request at the cursor included. */
export async function itemsFor(file: string): Promise<CopyItem[]> {
  const tab = useTabs.getState().tabs.find((t) => t.path === file);
  const a = activeView();
  let entry = 0;
  if (tab && a?.path === file) entry = await entryAt({ file, text: tab.text, version: tab.version }, a.view.state.selection.main.head);
  return copyItems({
    file,
    source: tab?.text ?? "",
    env: useEnv.getState().current,
    entry,
    run: useRuns.getState().runs[file],
    shell: shellOf(navigator.platform || navigator.userAgent),
  });
}

/** Copies item: its text (secrets as references), or with reveal its
 * secret values, after the warning. */
export async function copyItem(item: CopyItem, reveal = false) {
  try {
    if (reveal) {
      const ok = await confirm({
        title: "Copy with secret values?",
        message: "Real tokens and passwords go to the clipboard, hidden from clipboard history. Sonde clears it after 60 seconds if it still holds them. Paste it only where you would type them.",
        submit: "Copy with secrets",
      });
      if (!ok) return;
      const note = await (item.tool === "curl" ? CopyAsReveal.Curl(item.req) : CopyAsReveal.Sonde(item.req));
      toast("success", `Copied with secret values; cleared in 60 s. ${note ?? ""}`.trim());
      return;
    }
    const t = await (item.tool === "curl" ? CopyAs.Curl(item.req) : CopyAs.Sonde(item.req));
    if (!t) return;
    await navigator.clipboard.writeText(t.text);
    toast("success", `Copied. ${t.note ?? ""}`.trim());
  } catch (err) {
    toast("error", appError(err).message);
  }
}

/** Whether ⌥ (Alt) is held, while on (keys, and the pointer in the menu:
 * held before it opened). */
function useAlt(on: boolean) {
  const [alt, setAlt] = useState(false);
  useEffect(() => {
    if (!on) return;
    const f = (e: KeyboardEvent) => setAlt(e.altKey);
    window.addEventListener("keydown", f);
    window.addEventListener("keyup", f);
    return () => {
      window.removeEventListener("keydown", f);
      window.removeEventListener("keyup", f);
      setAlt(false);
    };
  }, [on]);
  return [alt, setAlt] as const;
}

export function CopyAsMenu({ file }: { file: string }) {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<CopyItem[]>([]);
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const overrides = useEnv((s) => s.overrides.count);
  const [held, setHeld] = useAlt(open);
  const alt = held && !serverMode;
  useEffect(() => {
    if (!open) return;
    let live = true;
    void itemsFor(file).then(async (list) => {
      if (!live) return;
      setItems(list);
      const texts = await Promise.all(list.map((i) => (i.tool === "curl" ? CopyAs.Curl(i.req) : CopyAs.Sonde(i.req)).then((t) => t?.text ?? "", () => "")));
      if (live) setPreviews(Object.fromEntries(list.map((i, n) => [i.id, texts[n].split("\n").filter((l) => !l.startsWith("#")).join(" ")])));
    });
    return () => {
      live = false;
    };
  }, [open, file]);
  return (
    <Menu.Root open={open} onOpenChange={setOpen}>
      <Menu.Trigger asChild>
        <button className="btn-ghost" aria-label="Copy as" title="Copy as curl or sonde">
          ⧉ ▾
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="menu copy-as" align="end" sideOffset={4} aria-label="Copy as" onPointerMove={(e) => held !== e.altKey && setHeld(e.altKey)}>
          <Menu.Label className="menu-label">{alt ? "Copy with secret values" : "Copy as · secrets as variable references"}</Menu.Label>
          {items.map((i) => (
            <Menu.Item key={i.id} className="menu-item tall" onSelect={() => void copyItem(i, alt)}>
              <b>
                {i.title}
                {alt && " · with secrets"}
              </b>
              <span className="sub mono copy-preview">{previews[i.id] ?? i.sub ?? "…"}</span>
            </Menu.Item>
          ))}
          {!serverMode && (
            <>
              <Menu.Separator className="menu-sep" />
              <div className="menu-note">
                <b>Copy with secret values</b> <span className="hint">hold ⌥</span>
                <div className="warn small">Puts real tokens and passwords on the clipboard.</div>
              </div>
            </>
          )}
          {overrides > 0 && (
            <div className="menu-note small">
              <span className="overrides-chip">{overrides} override{overrides === 1 ? "" : "s"}</span> sonde commands add --file-root . and a flag for each override, so CI runs the same thing.
            </div>
          )}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
