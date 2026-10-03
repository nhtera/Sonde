// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { registry, useRegistry } from "../../app/registry";
import { MoonIcon, SunIcon } from "../../components/icons";
import { serverMode } from "../../lib/mode";
import { themeById } from "../../app/theme/themes";
import { resolvedTheme, toggled, useOsLight, useSettings } from "../../state/settings";
import { useUI } from "../../state/ui";

export function Rail() {
  useRegistry();
  const panel = useUI((s) => s.panel);
  // With the side closed, the panel it would open stays marked.
  const marked = useUI((s) => s.panel ?? s.lastPanel);
  const appearance = useSettings((s) => s.value?.appearance);
  const osLight = useOsLight();
  const prefs = appearance ?? { theme: "system", dayTheme: "light", nightTheme: "dark" };
  const dark = resolvedTheme(prefs, osLight).kind === "dark";
  const next = themeById(toggled(prefs, osLight).theme);
  const panels = registry.panels().filter((p) => !(serverMode && p.windowOnly));
  const top = panels.filter((p) => p.id !== "settings");
  const settings = panels.find((p) => p.id === "settings");
  return (
    <nav className="rail" aria-label="Panels">
      {top.map((p) => (
        <button key={p.id} className={marked === p.id ? "current" : undefined} title={p.title} aria-label={p.title} aria-pressed={panel === p.id} onClick={() => useUI.getState().togglePanel(p.id)}>
          <p.icon />
        </button>
      ))}
      <div className="grow" />
      {settings && (
        <button className={marked === "settings" ? "current" : undefined} title={settings.title} aria-label={settings.title} aria-pressed={panel === "settings"} onClick={() => useUI.getState().togglePanel("settings")}>
          <settings.icon />
        </button>
      )}
      <button title={next ? `Switch to ${next.label}` : "Toggle theme"} aria-label="Toggle theme" onClick={() => void registry.getCommand("theme.toggle")?.run()}>
        {dark ? <MoonIcon /> : <SunIcon />}
      </button>
    </nav>
  );
}
