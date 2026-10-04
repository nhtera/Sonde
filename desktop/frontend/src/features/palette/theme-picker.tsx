// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Select theme…: the themes, Light then Dark. The highlighted one shows
// at once; Enter keeps it, Esc puts the theme back.

import * as Dialog from "@radix-ui/react-dialog";
import { Command } from "cmdk";
import { useEffect, useRef, useState, type PointerEvent } from "react";
import { themes } from "../../app/theme/themes";
import { SearchIcon } from "../../components/icons";
import { appError } from "../../lib/api";
import { applyTheme, resolvedTheme, useSettings, type ThemePrefs } from "../../state/settings";
import { useUI } from "../../state/ui";

const defaults: ThemePrefs = { theme: "system", dayTheme: "light", nightTheme: "dark" };
const prefs = () => useSettings.getState().value?.appearance ?? defaults;
const close = () => useUI.getState().setThemePickerOpen(false);

/** A theme matches when its name or id has the text (not cmdk's fuzzy
 * match: "gru" is Gruvbox, not GitHub Dark). */
export const themeFilter = (id: string, search: string, keywords: string[] = []) => {
  const q = search.trim().toLowerCase();
  return !q || [id, ...keywords].some((s) => s.toLowerCase().includes(q)) ? 1 : 0;
};

export function ThemePicker() {
  const open = useUI((s) => s.themePickerOpen);
  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog palette" aria-describedby={undefined}>
          <Dialog.Title className="sr-only">Select theme</Dialog.Title>
          <ThemeList />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function ThemeList() {
  // The theme in effect when the list opened: highlighted first, and
  // shown again if the list closes without a pick.
  const [current] = useState(() => resolvedTheme(prefs()).id);
  const [value, setValue] = useState(current);
  const picked = useRef(false);
  useEffect(
    () => () => {
      if (!picked.current) applyTheme(prefs());
    },
    [],
  );
  // The pointer picks once it moves: opening scrolls the current theme
  // into view, and the webview then reports a move under a still pointer,
  // which would preview the theme under it. Until then, its moves stop
  // before they reach the items.
  const still = useRef<string | null>("");
  const moved = (e: PointerEvent) => {
    if (still.current === null) return;
    const p = `${e.screenX},${e.screenY}`;
    if (still.current && still.current !== p) {
      still.current = null;
      return;
    }
    still.current = p;
    e.stopPropagation();
  };
  const preview = (id: string) => {
    setValue(id);
    if (themes.some((t) => t.id === id)) document.documentElement.dataset.theme = id;
  };
  const pick = (id: string) => {
    picked.current = true;
    close();
    useSettings
      .getState()
      .pickTheme(id)
      .catch((err) => useUI.getState().toast({ kind: "error", text: appError(err).message }));
  };
  return (
    <Command
      label="Select theme"
      loop
      filter={themeFilter}
      value={value}
      onValueChange={preview}
      onPointerMoveCapture={moved}
    >
      <div className="palette-input">
        <SearchIcon size={14} />
        <Command.Input autoFocus placeholder="Select theme" />
        <kbd>Esc</kbd>
      </div>
      <Command.List className="palette-list">
        <Command.Empty className="palette-empty">No matches</Command.Empty>
        {(["light", "dark"] as const).map((kind) => (
          <Command.Group key={kind} heading={kind === "light" ? "Light" : "Dark"}>
            {themes
              .filter((t) => t.kind === kind)
              .map((t) => (
                <Command.Item key={t.id} value={t.id} keywords={[t.label]} onSelect={() => pick(t.id)}>
                  <span className="title">{t.label}</span>
                  {t.id === current && <span className="hint">current</span>}
                </Command.Item>
              ))}
          </Command.Group>
        ))}
      </Command.List>
    </Command>
  );
}
