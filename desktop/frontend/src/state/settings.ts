// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { create } from "zustand";
import { Settings, type SettingsValue } from "../lib/api";
import { on } from "../lib/events";

export type Theme = "system" | "light" | "dark";

interface SettingsState {
  value: SettingsValue | null;
  load(): Promise<void>;
  save(next: SettingsValue): Promise<void>;
  setTheme(theme: Theme): Promise<void>;
  setShortcut(command: string, keys: string | null): Promise<void>;
}

export const useSettings = create<SettingsState>((set, get) => ({
  value: null,
  load: async () => {
    const value = await Settings.Get();
    set({ value });
    applyTheme(value.appearance.theme as Theme);
  },
  save: async (next) => {
    const value = await Settings.Set(next);
    set({ value });
    applyTheme(value.appearance.theme as Theme);
  },
  setTheme: async (theme) => {
    const v = get().value;
    if (v) await get().save({ ...v, appearance: { ...v.appearance, theme } });
  },
  setShortcut: async (command, keys) => {
    const v = get().value;
    if (!v) return;
    const shortcuts = { ...(v.shortcuts ?? {}) };
    if (keys === null) delete shortcuts[command];
    else shortcuts[command] = keys;
    await get().save({ ...v, shortcuts });
  },
}));

on("settings:changed", (data) => {
  const value = data as SettingsValue;
  useSettings.setState({ value });
  applyTheme(value.appearance.theme as Theme);
});

/** The theme in effect ("system" follows the OS). */
export function resolvedTheme(theme: Theme): "light" | "dark" {
  if (theme === "system") return window.matchMedia?.("(prefers-color-scheme: light)").matches ? "light" : "dark";
  return theme;
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = resolvedTheme(theme);
}

// Until the settings load, and whenever the OS switches, "system" follows
// the OS (no dark flash on a light system).
applyTheme("system");
window.matchMedia?.("(prefers-color-scheme: light)").addEventListener?.("change", () => {
  const theme = (useSettings.getState().value?.appearance.theme ?? "system") as Theme;
  if (theme === "system") applyTheme(theme);
});
