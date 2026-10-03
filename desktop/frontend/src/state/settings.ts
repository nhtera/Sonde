// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useSyncExternalStore } from "react";
import { create } from "zustand";
import { themeById, type ThemeInfo } from "../app/theme/themes";
import { Settings, type SettingsValue } from "../lib/api";
import { on } from "../lib/events";

type Appearance = SettingsValue["appearance"];

/** The theme settings: theme is "system" (the day theme while the OS is
 * light, the night theme while it is dark) or a theme id. */
export type ThemePrefs = Pick<Appearance, "theme" | "dayTheme" | "nightTheme">;

interface SettingsState {
  value: SettingsValue | null;
  load(): Promise<void>;
  save(next: SettingsValue): Promise<void>;
  /** Sets the theme: "system" (Sync) or a theme id (Manual). */
  setTheme(theme: string): Promise<void>;
  /** Sets the day or night theme (used with Sync). */
  setSlot(slot: "day" | "night", id: string): Promise<void>;
  /** Keeps a theme picked from the list: Manual's theme, or with Sync the
   * slot the OS uses now. */
  pickTheme(id: string): Promise<void>;
  /** Switches between the day and night themes (toggled). */
  toggleTheme(): Promise<void>;
  setShortcut(command: string, keys: string | null): Promise<void>;
}

/** The write being sent, if any (settings:changed waits for it). */
let writing: Promise<void> | null = null;
/** A change was made while it was sent: the settings go once more. */
let again = false;

export const useSettings = create<SettingsState>((set, get) => {
  const show = (value: SettingsValue) => {
    set({ value });
    applyTheme(value.appearance);
    applyAppearance(value.appearance);
  };
  // One write at a time, of the settings as they are when it is sent:
  // changes made meanwhile go together in the next one, and an older
  // reply never undoes a newer change.
  const write = (): Promise<void> => {
    if (writing) {
      again = true;
      return writing;
    }
    writing = (async () => {
      try {
        do {
          again = false;
          const value = await Settings.Set(get().value!);
          if (!again) show(value);
        } while (again);
      } catch (err) {
        again = false;
        await get().load().catch(() => {});
        throw err;
      } finally {
        writing = null;
      }
    })();
    return writing;
  };
  // Theme changes build on the settings as they are now, not as a render
  // saw them.
  const changeAppearance = async (change: (a: Appearance) => Partial<Appearance>) => {
    const v = get().value;
    if (v) await get().save({ ...v, appearance: { ...v.appearance, ...change(v.appearance) } });
  };
  return {
    value: null,
    load: async () => show(await Settings.Get()),
    // Shown at once; on failure the stored settings come back.
    save: async (next) => {
      show(next);
      await write();
    },
    setTheme: (theme) => changeAppearance(() => ({ theme })),
    setSlot: (slot, id) => changeAppearance(() => (slot === "day" ? { dayTheme: id } : { nightTheme: id })),
    pickTheme: (id) =>
      changeAppearance((a) => (a.theme !== "system" ? { theme: id } : prefersLight() ? { dayTheme: id } : { nightTheme: id })),
    toggleTheme: () => changeAppearance((a) => toggled(a, prefersLight())),
    setShortcut: async (command, keys) => {
      const v = get().value;
      if (!v) return;
      const shortcuts = { ...(v.shortcuts ?? {}) };
      if (keys === null) delete shortcuts[command];
      else shortcuts[command] = keys;
      await get().save({ ...v, shortcuts });
    },
  };
});

on("settings:changed", (data) => {
  // While this page writes, its own write's reply has the last word.
  if (writing) return;
  const value = data as SettingsValue;
  useSettings.setState({ value });
  applyTheme(value.appearance);
  applyAppearance(value.appearance);
});

const lightQuery = () => window.matchMedia?.("(prefers-color-scheme: light)");

/** Whether the OS look is light now. */
export const prefersLight = () => !!lightQuery()?.matches;

const subscribeOS = (changed: () => void) => {
  const q = lightQuery();
  q?.addEventListener?.("change", changed);
  return () => q?.removeEventListener?.("change", changed);
};

/** Whether the OS look is light, updated when it switches. */
export function useOsLight() {
  return useSyncExternalStore(subscribeOS, prefersLight);
}

/** The theme in effect; an unknown id is Light or Dark, as the OS. */
export function resolvedTheme(a: ThemePrefs, osLight = prefersLight()): ThemeInfo {
  const id = a.theme !== "system" ? a.theme : osLight ? a.dayTheme : a.nightTheme;
  return themeById(id) ?? themeById(osLight ? "light" : "dark")!;
}

/** The theme settings after the toggle: with Sync, Manual with the other
 * slot's theme; with Manual, the day theme from a dark one, the night
 * theme from a light one. The default slots flip Light and Dark. */
export function toggled(a: ThemePrefs, osLight: boolean): Pick<Appearance, "theme"> {
  if (a.theme === "system") return { theme: osLight ? a.nightTheme : a.dayTheme };
  return { theme: resolvedTheme(a, osLight).kind === "dark" ? a.dayTheme : a.nightTheme };
}

/** The editor's font size and ligatures (ligatures only in the editor).
 * The UI font size is the window's zoom, set by the window app. */
export function applyAppearance(a: { codeFontSize: number; ligatures: boolean }) {
  const root = document.documentElement;
  if (a.codeFontSize) root.style.setProperty("--code-size", `${a.codeFontSize}px`);
  else root.style.removeProperty("--code-size");
  root.dataset.ligatures = a.ligatures ? "on" : "off";
}

export function applyTheme(a: ThemePrefs) {
  document.documentElement.dataset.theme = resolvedTheme(a).id;
}

/** The theme settings the app's server wrote on <html> (themedIndex),
 * until the settings load. */
function pagePrefs(): ThemePrefs {
  const d = document.documentElement.dataset;
  return { theme: d.themePref || "system", dayTheme: d.themeDay || "light", nightTheme: d.themeNight || "dark" };
}

// public/theme-boot.js set the theme before the first paint; if it did
// not run, now. With Sync the theme follows the OS as it switches.
if (!document.documentElement.dataset.theme) applyTheme(pagePrefs());
lightQuery()?.addEventListener?.("change", () => {
  const a = useSettings.getState().value?.appearance ?? pagePrefs();
  if (a.theme === "system") applyTheme(a);
});
