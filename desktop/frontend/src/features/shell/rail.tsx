// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { registry, useRegistry } from "../../app/registry";
import { MoonIcon, SunIcon } from "../../components/icons";
import { serverMode } from "../../lib/mode";
import { resolvedTheme, useSettings, type Theme } from "../../state/settings";
import { useUI } from "../../state/ui";

export function Rail() {
  useRegistry();
  const panel = useUI((s) => s.panel);
  const theme = useSettings((s) => (s.value?.appearance.theme ?? "system") as Theme);
  const dark = resolvedTheme(theme) === "dark";
  const panels = registry.panels().filter((p) => !(serverMode && p.windowOnly));
  const top = panels.filter((p) => p.id !== "settings");
  const settings = panels.find((p) => p.id === "settings");
  return (
    <nav className="rail" aria-label="Panels">
      {top.map((p) => (
        <button key={p.id} title={p.title} aria-label={p.title} aria-pressed={panel === p.id} onClick={() => useUI.getState().togglePanel(p.id)}>
          <p.icon />
        </button>
      ))}
      <div className="grow" />
      {settings && (
        <button title={settings.title} aria-label={settings.title} aria-pressed={panel === "settings"} onClick={() => useUI.getState().togglePanel("settings")}>
          <settings.icon />
        </button>
      )}
      <button title={dark ? "Light theme" : "Dark theme"} aria-label="Toggle theme" onClick={() => void useSettings.getState().setTheme(dark ? "light" : "dark")}>
        {dark ? <MoonIcon /> : <SunIcon />}
      </button>
    </nav>
  );
}
