// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The shortcuts sheet (?): every command's keys, rebound by pressing the
// new keys. A combination another command uses is refused and reported.

import * as Dialog from "@radix-ui/react-dialog";
import { useEffect, useMemo, useState } from "react";
import { appError } from "../../lib/api";
import { isMac } from "../../lib/mode";
import { useSettings } from "../../state/settings";
import { useUI } from "../../state/ui";
import { registry, useRegistry } from "../registry";
import { defaultKeys } from "./defaults";
import { conflicts, fromEvent, isBindable, label, normalize } from "./keymap-manager";
import { useKeys } from "./use-keys";

export function ShortcutsSheet() {
  useRegistry();
  const open = useUI((s) => s.shortcutsOpen);
  const user = useSettings((s) => s.value?.shortcuts);
  const keys = useKeys();
  const clashes = useMemo(() => conflicts(keys), [keys]);
  const [editing, setEditing] = useState<string | null>(null);
  const [error, setError] = useState("");
  const commands = registry
    .commands()
    .filter((c) => keys[c.id] !== undefined || c.keys || defaultKeys[c.id] || user?.[c.id] === "")
    .sort((a, b) => a.title.localeCompare(b.title));

  useEffect(() => {
    if (!editing) return;
    const onKey = (e: KeyboardEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (e.key === "Escape") return setEditing(null);
      const k = fromEvent(e);
      if (!k) return;
      if (!isBindable(k)) {
        setError(`${label(k)} alone would be taken from the whole app: add ${isMac ? "⌘ or ⌥" : "Ctrl or Alt"}`);
        return;
      }
      const taken = Object.entries(keys).find(([id, v]) => id !== editing && normalize(v) === normalize(k));
      if (taken) {
        setError(`${label(k)} is used by ${registry.getCommand(taken[0])?.title ?? taken[0]}`);
        return;
      }
      setEditing(null);
      setError("");
      useSettings
        .getState()
        .setShortcut(editing, k)
        .catch((err) => setError(appError(err).message));
    };
    // Capture: the app's own bindings must not see the keys being recorded.
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [editing, keys]);

  const close = () => {
    setEditing(null);
    setError("");
    useUI.getState().setShortcutsOpen(false);
  };

  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content
          className="dialog shortcuts"
          aria-describedby={undefined}
          onEscapeKeyDown={(e) => editing && e.preventDefault()}
        >
          <Dialog.Title className="dialog-title">Keyboard shortcuts</Dialog.Title>
          <p className="dialog-note">Click a shortcut, then press the new keys. Esc cancels.</p>
          {error && (
            <p className="dialog-error" role="alert">
              {error}
            </p>
          )}
          <ul className="shortcut-list">
            {commands.map((c) => {
              const k = keys[c.id];
              const custom = user?.[c.id] !== undefined;
              return (
                <li key={c.id} className={clashes[k ? normalize(k) : ""] ? "clash" : ""}>
                  <span className="title">{c.title}</span>
                  <button
                    className="btn keycap"
                    aria-label={`Rebind ${c.title}`}
                    onClick={() => {
                      setError("");
                      setEditing(c.id);
                    }}
                  >
                    {editing === c.id ? "Press keys…" : k ? <kbd>{label(k)}</kbd> : "Not set"}
                  </button>
                  <button
                    className="btn-ghost"
                    disabled={!custom}
                    onClick={() => void useSettings.getState().setShortcut(c.id, null)}
                  >
                    Reset
                  </button>
                </li>
              );
            })}
          </ul>
          <div className="dialog-actions">
            <button className="btn" onClick={close}>
              Done
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
