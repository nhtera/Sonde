// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";
import { counts } from "../../components/run/counts";
import { Vars } from "../../lib/api";
import { useEnv } from "../../state/env";
import { useRuns } from "../../state/run";
import { isDirty, useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { envColor } from "../../lib/env-color";

declare const __APP_VERSION__: string;

export function StatusBar() {
  const env = useEnv((s) => s.current);
  const active = useTabs((s) => s.active);
  const tab = useTabs((s) => s.tabs.find((t) => t.path === s.active));
  const run = useRuns((s) => (active ? s.runs[active] : undefined));
  const base = useBaseURL(active, env);
  const cursor = useUI((s) => s.cursor);
  const summary = run?.summary;
  const c = counts(Object.values(run?.entries ?? {}));
  return (
    <footer className="statusbar">
      {env && (
        <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <i className="dot" style={{ width: 6, height: 6, background: envColor(env) }} />
          {env}
        </span>
      )}
      {base && <span className="mono">{base}</span>}
      {active && <span>{active.endsWith(".sonde") ? ".sonde extensions" : "Hurl 8 syntax"}</span>}
      {tab && isDirty(tab) && <span style={{ color: "var(--warn)" }}>Unsaved</span>}
      <div className="grow" />
      {summary && (
        <span className="mono num">
          <span style={{ color: "var(--pass)" }}>✓ {c.passed}</span>
          {c.failed > 0 && <span style={{ color: "var(--fail)" }}> ✗ {c.failed}</span>} · <span data-volatile>{summary.durationMs} ms</span>
        </span>
      )}
      {cursor && active && (
        <span className="mono">
          Ln {cursor.line}, Col {cursor.col}
        </span>
      )}
      <button onClick={() => useUI.getState().setShortcutsOpen(true)}>? Shortcuts</button>
      <span className="mono" style={{ color: "var(--faint)" }}>
        sonde {__APP_VERSION__} · local only
      </span>
    </footer>
  );
}

/** The effective base_url of the active file (its env, overrides). */
function useBaseURL(file: string | null, env: string): string {
  // Keyed by what it was read for, so a stale answer is never shown.
  const [base, setBase] = useState({ key: "", value: "" });
  const overrides = useEnv((s) => s.overrides);
  const key = file ? `${file}\n${env}` : "";
  useEffect(() => {
    if (!file) return;
    let live = true;
    Vars.For(file, env)
      .then((vars) => live && setBase({ key, value: vars?.find((v) => v.name === "base_url")?.display ?? "" }))
      .catch(() => live && setBase({ key, value: "" }));
    return () => {
      live = false;
    };
  }, [file, env, key, overrides]);
  return base.key === key ? base.value : "";
}
