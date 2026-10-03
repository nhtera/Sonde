// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The drag handle between the results' request list and the response
// below it: the list's height, saved in the settings (appearance).

import { useRef, useState, type PointerEvent as ReactPointerEvent, type RefObject } from "react";
import { appError } from "../../lib/api";
import { useSettings } from "../../state/settings";
import { useUI } from "../../state/ui";

const MIN = 80;

/** Saves the list's height (0: sized to its rows again). */
function save(h: number) {
  const s = useSettings.getState();
  const v = s.value;
  if (!v) return;
  s.save({ ...v, appearance: { ...v.appearance, resultsTop: Math.round(h) } }).catch((err) =>
    useUI.getState().toast({ kind: "error", text: appError(err).message }),
  );
}

/** A handle under top: dragging sets top's height directly (no render)
 * and saves it on release; arrow keys move it by 16px; a double click
 * sizes the list to its rows again. */
export function SplitHandle({ top }: { top: RefObject<HTMLDivElement | null> }) {
  const start = useRef<{ y: number; h: number; last: number; max: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const set = (el: HTMLElement, h: number) => Object.assign(el.style, { height: `${h}px`, maxHeight: "none" });
  const down = (e: ReactPointerEvent<HTMLDivElement>) => {
    const el = top.current;
    if (!el) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    // The response keeps at least 120px of the panel.
    const max = Math.max(MIN, (el.parentElement?.clientHeight ?? 600) - 120);
    const h = el.getBoundingClientRect().height;
    start.current = { y: e.clientY, h, last: h, max };
    setDragging(true);
  };
  const move = (e: ReactPointerEvent<HTMLDivElement>) => {
    const s = start.current;
    if (!s || !top.current) return;
    s.last = Math.min(s.max, Math.max(MIN, s.h + e.clientY - s.y));
    set(top.current, s.last);
  };
  const up = () => {
    const s = start.current;
    start.current = null;
    setDragging(false);
    if (s && s.last !== s.h) save(s.last);
  };
  return (
    <div
      className="results-split"
      role="separator"
      tabIndex={0}
      aria-label="Resize the request list"
      aria-orientation="horizontal"
      title="Drag to resize · double-click to fit the list"
      data-dragging={dragging || undefined}
      onPointerDown={down}
      onPointerMove={move}
      onPointerUp={up}
      onPointerCancel={up}
      onDoubleClick={() => {
        top.current?.style.removeProperty("height");
        top.current?.style.removeProperty("max-height");
        save(0);
      }}
      onKeyDown={(e) => {
        const el = top.current;
        if (!el || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return;
        e.preventDefault();
        const max = Math.max(MIN, (el.parentElement?.clientHeight ?? 600) - 120);
        const h = Math.min(max, Math.max(MIN, el.getBoundingClientRect().height + (e.key === "ArrowDown" ? 16 : -16)));
        set(el, h);
        save(h);
      }}
    />
  );
}
