// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The built-in themes' names. Go allows these ids, in this order
// (internal/settings/themes.go); each has a [data-theme] token set
// (tokens.css, themes/*.css).

export interface ThemeInfo {
  id: string;
  label: string;
  kind: "light" | "dark";
}

const light = (id: string, label: string): ThemeInfo => ({ id, label, kind: "light" });
const dark = (id: string, label: string): ThemeInfo => ({ id, label, kind: "dark" });

export const themes: ThemeInfo[] = [
  light("light", "Light"),
  light("solarized-light", "Solarized Light"),
  light("ayu-light", "Ayu Light"),
  light("github-light", "GitHub Light"),
  light("catppuccin-latte", "Catppuccin Latte"),
  light("tokyo-night-light", "Tokyo Night Light"),
  light("vscode-light", "VS Code Light"),
  light("gruvbox-light", "Gruvbox Light"),
  light("rose-pine-dawn", "Rosé Pine Dawn"),
  dark("dark", "Dark"),
  dark("hc-dark", "High Contrast Dark"),
  dark("ayu-dark", "Ayu Dark"),
  dark("dracula", "Dracula"),
  dark("monokai", "Monokai"),
  dark("night-owl", "Night Owl"),
  dark("solarized-dark", "Solarized Dark"),
  dark("github-dark", "GitHub Dark"),
  dark("catppuccin-mocha", "Catppuccin Mocha"),
  dark("tokyo-night", "Tokyo Night"),
  dark("vscode-dark", "VS Code Dark"),
  dark("one-dark-pro", "One Dark Pro"),
  dark("nord", "Nord"),
  dark("gruvbox-dark", "Gruvbox Dark"),
  dark("rose-pine", "Rosé Pine"),
];

export const themeById = (id: string) => themes.find((t) => t.id === id);
