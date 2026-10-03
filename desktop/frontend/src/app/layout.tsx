// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The window: title bar, rail, side panel, main area (tabs, toolbar,
// editor), results and status bar. At 1024px and below the side panel
// becomes an overlay and the results a toggled, narrower pane.

import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { EditorHost, ResultsHost } from "../features/shell/hosts";
import { NoFile, Welcome } from "../features/shell/empty-states";
import { Rail } from "../features/shell/rail";
import { StatusBar } from "../features/shell/status-bar";
import { TabsBar } from "../features/shell/tabs-bar";
import { TitleBar } from "../features/shell/title-bar";
import { Toolbar } from "../features/shell/toolbar";
import { TrustBar } from "../features/shell/trust-bar";
import { appError } from "../lib/api";
import { useSettings } from "../state/settings";
import { useTabs } from "../state/tabs";
import { useUI } from "../state/ui";
import { useWorkspace } from "../state/workspace";
import { registry, useRegistry } from "./registry";
import { isRequestPath } from "../lib/files";

const NARROW = "(max-width: 1024px)";

export function Layout() {
  useRegistry();
  const project = useWorkspace((s) => s.project);
  const loaded = useWorkspace((s) => s.loaded);
  const panelId = useUI((s) => s.panel);
  const narrow = useUI((s) => s.narrow);
  const resultsOpen = useUI((s) => s.resultsOpen);
  const active = useTabs((s) => s.active);
  const appearance = useSettings((s) => s.value?.appearance);
  const panel = panelId ? registry.getPanel(panelId) : undefined;

  useEffect(() => {
    const mq = window.matchMedia(NARROW);
    const apply = () => {
      useUI.getState().setNarrow(mq.matches);
      // Collapsing to the rail closes the side panel; it opens as an overlay.
      if (mq.matches) useUI.getState().setPanel(null);
      else if (!useUI.getState().panel) useUI.getState().setPanel("files");
    };
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, []);

  // Narrow: the side panel is an overlay, closed by Escape, a click
  // outside it or opening a file; it takes the focus when it opens.
  const aside = useRef<HTMLElement>(null);
  const overlay = narrow && !!panel;
  useEffect(() => {
    if (!overlay) return;
    aside.current?.querySelector<HTMLElement>("input, button, [tabindex]")?.focus();
    const close = () => useUI.getState().setPanel(null);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !document.querySelector('[role="dialog"][data-state="open"]') && close();
    const onDown = (e: PointerEvent) => {
      const t = e.target as Node;
      if (!aside.current?.contains(t) && !(t as Element).closest?.(".rail, [role='menu'], [role='dialog']")) close();
    };
    const offTabs = useTabs.subscribe((s, prev) => s.active !== prev.active && close());
    window.addEventListener("keydown", onKey);
    window.addEventListener("pointerdown", onDown);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("pointerdown", onDown);
      offTabs();
    };
  }, [overlay]);

  const side = appearance?.sideWidth ?? 248;
  const results = appearance?.resultsWidth ?? 440;
  // A panel with a main view shows it in place of the tabs and editor.
  const PanelMain = project ? panel?.main : undefined;
  const welcome = loaded && !project;
  // Results are a request file's: a data or YAML file has none.
  const showResults = !!project && isRequestPath(active) && (!narrow || resultsOpen) && !(PanelMain && panel?.wide);
  // Narrow: the side panel is an overlay (no column); results are docked
  // at 400px and toggled from the toolbar.
  const sideCol = project && panel && !narrow ? side : 0;
  const resultsCol = !showResults ? 0 : narrow ? 400 : results;

  return (
    <div
      className="window"
      data-narrow={narrow ? "yes" : "no"}
      data-welcome={welcome || undefined}
      style={{ ["--side-w" as string]: `${sideCol}px`, ["--results-w" as string]: `${resultsCol}px` }}
    >
      <TitleBar />
      {/* No folder open: the welcome alone, under the title bar. */}
      {!welcome && <Rail />}
      {!loaded ? (
        <div style={{ gridColumn: "2 / -1" }} />
      ) : !project ? (
        <Welcome />
      ) : (
        <>
          {panel && (
            <aside className="side" aria-label={panel.title} ref={aside}>
              <panel.render />
              <Resizer label="Resize the side panel" edge="right" cssVar="--side-w" width={side} min={180} max={480} onDone={(w) => saveWidth("sideWidth", w)} />
            </aside>
          )}
          <main className="main">
            <TrustBar />
            {PanelMain ? (
              <div className="panel-main">
                <PanelMain />
              </div>
            ) : (
              <>
                <TabsBar />
                {active ? (
                  <>
                    <Toolbar file={active} />
                    <div className="editor-host">
                      <EditorHost file={active} />
                    </div>
                  </>
                ) : (
                  <NoFile />
                )}
              </>
            )}
          </main>
          {showResults && (
            <section className="results" aria-label="Results">
              <Resizer label="Resize the results" edge="left" cssVar="--results-w" width={results} min={320} max={900} onDone={(w) => saveWidth("resultsWidth", w)} />
              <ResultsHost file={active!} />
            </section>
          )}
        </>
      )}
      {!welcome && <StatusBar />}
    </div>
  );
}

/** Saves a pane width in the settings (a failure is shown). */
function saveWidth(key: "sideWidth" | "resultsWidth", w: number) {
  const s = useSettings.getState();
  const v = s.value;
  if (!v) return;
  s.save({ ...v, appearance: { ...v.appearance, [key]: Math.round(w) } }).catch((err) =>
    useUI.getState().toast({ kind: "error", text: appError(err).message }),
  );
}

interface ResizerProps {
  label: string;
  edge: "left" | "right";
  /** The window's CSS variable holding the pane's width. */
  cssVar: "--side-w" | "--results-w";
  width: number;
  min: number;
  max: number;
  onDone(w: number): void;
}

/** A drag handle on a pane's edge; the pane grows away from the edge.
 * While dragging it sets the width on the window directly (no render);
 * the width is saved on release. Arrow keys resize by 16px. */
function Resizer({ label, edge, cssVar, width, min, max, onDone }: ResizerProps) {
  const start = useRef<{ x: number; w: number; last: number; win: HTMLElement | null } | null>(null);
  const [dragging, setDragging] = useState(false);
  const clamp = (w: number) => Math.min(max, Math.max(min, w));
  const down = (e: ReactPointerEvent<HTMLDivElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId);
    start.current = { x: e.clientX, w: width, last: width, win: e.currentTarget.closest<HTMLElement>(".window") };
    setDragging(true);
  };
  const move = (e: ReactPointerEvent<HTMLDivElement>) => {
    const s = start.current;
    if (!s) return;
    const dx = e.clientX - s.x;
    s.last = clamp(s.w + (edge === "right" ? dx : -dx));
    s.win?.style.setProperty(cssVar, `${s.last}px`);
  };
  const up = () => {
    const s = start.current;
    start.current = null;
    setDragging(false);
    if (s && s.last !== s.w) onDone(s.last);
  };
  return (
    <div
      className="resizer"
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation="vertical"
      aria-valuenow={width}
      aria-valuemin={min}
      aria-valuemax={max}
      data-dragging={dragging || undefined}
      style={edge === "right" ? { right: -3 } : { left: -3 }}
      onPointerDown={down}
      onPointerMove={move}
      onPointerUp={up}
      onPointerCancel={up}
      onKeyDown={(e) => {
        if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
        e.preventDefault();
        const grow = (e.key === "ArrowRight") === (edge === "right");
        const next = clamp(width + (grow ? 16 : -16));
        if (next !== width) onDone(next);
      }}
    />
  );
}
