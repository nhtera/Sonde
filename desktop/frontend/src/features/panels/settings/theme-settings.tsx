// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Settings › Appearance › Theme: Sync with system (a Day and a Night
// theme) or Manual (one theme), each with a preview.

import type { ReactNode } from "react";
import { ThemePreview } from "../../../app/theme/theme-preview";
import { themes } from "../../../app/theme/themes";
import { ChevronDown, MoonIcon, SunIcon } from "../../../components/icons";
import { appError } from "../../../lib/api";
import { resolvedTheme, useOsLight, useSettings, type ThemePrefs } from "../../../state/settings";
import { useUI } from "../../../state/ui";

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

export function ThemeSettings({ prefs }: { prefs: ThemePrefs }) {
  const osLight = useOsLight();
  const sync = prefs.theme === "system";
  const s = useSettings.getState();
  return (
    <div className="theme-settings">
      <div className="set-row">
        <div>Theme</div>
        <div className="set-control theme-mode" role="radiogroup" aria-label="Theme selection">
          <label>
            <input type="radio" name="theme-mode" checked={sync} onChange={() => void s.setTheme("system").catch(fail)} />
            Sync with system
          </label>
          <label>
            {/* Manual starts with the theme in effect: nothing changes. */}
            <input type="radio" name="theme-mode" checked={!sync} onChange={() => void s.setTheme(resolvedTheme(prefs, osLight).id).catch(fail)} />
            Manual
          </label>
        </div>
      </div>
      <div className="theme-cards">
        {sync ? (
          <>
            <ThemeCard
              title="Day theme"
              note="Active when the system is light"
              icon={<SunIcon size={14} />}
              id={prefs.dayTheme}
              first="light"
              active={osLight}
              onPick={(id) => void s.setSlot("day", id).catch(fail)}
            />
            <ThemeCard
              title="Night theme"
              note="Active when the system is dark"
              icon={<MoonIcon size={14} />}
              id={prefs.nightTheme}
              first="dark"
              active={!osLight}
              onPick={(id) => void s.setSlot("night", id).catch(fail)}
            />
          </>
        ) : (
          <ThemeCard title="Theme" note="Always used" id={prefs.theme} first={resolvedTheme(prefs, osLight).kind} onPick={(id) => void s.setTheme(id).catch(fail)} />
        )}
      </div>
    </div>
  );
}

function ThemeCard(props: {
  title: string;
  note: string;
  icon?: ReactNode;
  id: string;
  /** The kind listed first. */
  first: "light" | "dark";
  active?: boolean;
  onPick(id: string): void;
}) {
  const kinds = props.first === "light" ? (["light", "dark"] as const) : (["dark", "light"] as const);
  return (
    <div className="theme-card">
      <div className="theme-card-head">
        {props.icon}
        <span>{props.title}</span>
        {props.active && <span className="theme-active">Active</span>}
      </div>
      <div className="muted small">{props.note}</div>
      <ThemePreview id={props.id} />
      <span className="theme-select">
        <select aria-label={props.title} value={props.id} onChange={(e) => props.onPick(e.target.value)}>
        {kinds.map((kind) => (
          <optgroup key={kind} label={kind === "light" ? "Light" : "Dark"}>
            {themes
              .filter((t) => t.kind === kind)
              .map((t) => (
                <option key={t.id} value={t.id}>
                  {t.label}
                </option>
              ))}
          </optgroup>
        ))}
        </select>
        <ChevronDown className="chev" />
      </span>
    </div>
  );
}
