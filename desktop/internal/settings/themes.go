// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package settings

// Theme is a built-in theme: its id, light or dark, and the panel color
// the window shows before the page paints (the theme's --panel token).
type Theme struct {
	ID         string
	Kind       string // "light" or "dark"
	Background uint32 // 0xRRGGBB
}

// Themes are the built-in themes, light first (the page lists them in
// this order: frontend/src/app/theme/themes.ts).
var Themes = []Theme{
	{"light", "light", 0xf6f8fb},
	{"solarized-light", "light", 0xf5efdc},
	{"ayu-light", "light", 0xfafafa},
	{"github-light", "light", 0xf6f8fa},
	{"catppuccin-latte", "light", 0xe6e9ef},
	{"tokyo-night-light", "light", 0xd6d8df},
	{"vscode-light", "light", 0xfafafd},
	{"gruvbox-light", "light", 0xf2e5bc},
	{"rose-pine-dawn", "light", 0xf4ede8},
	{"dark", "dark", 0x101826},
	{"hc-dark", "dark", 0x0b111c},
	{"ayu-dark", "dark", 0x141821},
	{"dracula", "dark", 0x21222c},
	{"monokai", "dark", 0x2d2e27},
	{"night-owl", "dark", 0x01111d},
	{"solarized-dark", "dark", 0x04323e},
	{"github-dark", "dark", 0x151b23},
	{"catppuccin-mocha", "dark", 0x262637},
	{"tokyo-night", "dark", 0x1e202e},
	{"vscode-dark", "dark", 0x191a1b},
	{"one-dark-pro", "dark", 0x2c313c},
	{"nord", "dark", 0x3b4252},
	{"gruvbox-dark", "dark", 0x32302f},
	{"rose-pine", "dark", 0x1f1d2e},
}

// ThemeByID returns the built-in theme id.
func ThemeByID(id string) (Theme, bool) {
	for _, t := range Themes {
		if t.ID == id {
			return t, true
		}
	}
	return Theme{}, false
}
