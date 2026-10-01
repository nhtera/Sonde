// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The performance tour of the shipped app (sonde-desktop --perf-trace):
// it measures the budgets in the native webview, where the browser
// tests' Event Timing and long tasks are not available: the time to the
// next frame stands for "painted", the longest gap between frames for the
// longest main-thread task. scripts/perf.mjs makes the project and reads
// the results.

import { EditorSelection } from "@codemirror/state";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { activeView } from "../editor/views";

/** The tour's parameters (scripts/perf.mjs). */
export interface TourPlan {
  /** Typed in the tree's filter, and the text that shows its result. */
  filter: string;
  filterDone: string;
  /** A large request file to type in, and one whose response is ~50 MB. */
  big: string;
  json: string;
}

export interface Recorder {
  interactive(epochMs: number): unknown;
  record(name: string, value: number, unit: string): unknown;
}

const frame = () => new Promise<number>((r) => requestAnimationFrame(r));

/** Resolves when check() holds, checked each frame; rejects after ms. */
export function until(check: () => boolean, ms = 30_000): Promise<void> {
  const end = performance.now() + ms;
  return new Promise((resolve, reject) => {
    const tick = () => {
      if (check()) resolve();
      else if (performance.now() > end) reject(new Error("timed out"));
      else requestAnimationFrame(tick);
    };
    tick();
  });
}

/** Percentile p (0–1) of values. */
export function percentile(values: number[], p: number): number {
  const s = [...values].sort((a, b) => a - b);
  return s.length ? s[Math.min(s.length - 1, Math.floor(s.length * p))] : NaN;
}

/** Watches the gaps between frames while it runs; stop() returns the
 * longest. */
function frameGaps() {
  let on = true;
  let last = performance.now();
  let longest = 0;
  const tick = (t: number) => {
    longest = Math.max(longest, t - last);
    last = t;
    if (on) requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);
  return () => {
    on = false;
    return longest;
  };
}

export async function runTour(plan: TourPlan, rec: Recorder) {
  // Cold start: the tree's first rows on screen.
  await until(() => !!document.querySelector(".tree-row"));
  await frame();
  await rec.interactive(Date.now());

  // Tree scroll: 60 frames of 900 px.
  const tree = document.querySelector<HTMLElement>('[role="tree"]')!;
  const times: number[] = [];
  let t = await frame();
  for (let i = 0; i < 60; i++) {
    tree.scrollTop += 900;
    const next = await frame();
    times.push(next - t);
    t = next;
  }
  await rec.record("tree scroll", 1000 / (times.reduce((a, b) => a + b, 0) / times.length), "fps");
  await rec.record("tree scroll longest frame", Math.max(...times), "ms");

  // Filter: from the input event to the painted result.
  const input = document.querySelector<HTMLInputElement>('input[aria-label="Filter requests in all files"]')!;
  const f0 = performance.now();
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, plan.filter);
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await until(() => !!document.body.textContent?.includes(plan.filterDone));
  await frame();
  await rec.record("tree filter", performance.now() - f0, "ms");
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, "");
  input.dispatchEvent(new Event("input", { bubbles: true }));

  // Keystroke to paint: 80 characters typed in the middle of a large file.
  await useTabs.getState().open(plan.big);
  await until(() => activeView()?.path === plan.big);
  const view = activeView()!.view;
  const mid = view.state.doc.line(Math.floor(view.state.doc.lines / 2)).from;
  view.dispatch({ selection: EditorSelection.cursor(mid) });
  await frame();
  const keys: number[] = [];
  for (let i = 0; i < 80; i++) {
    const k0 = performance.now();
    view.dispatch(view.state.replaceSelection("x"), { userEvent: "input.type" });
    await frame();
    keys.push(performance.now() - k0);
  }
  await rec.record("keystroke to paint p95", percentile(keys, 0.95), "ms");

  // A ~50 MB JSON response: run to the tree's first rows.
  await useTabs.getState().open(plan.json);
  const r0 = performance.now();
  const stop = frameGaps();
  void useRuns.getState().run(plan.json);
  const format = () => [...document.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent === "Format anyway");
  await until(() => !!format() || !!document.querySelector(".jt-row"), 60_000);
  format()?.click();
  await until(() => !!document.querySelector(".jt-row"), 60_000);
  await frame();
  await rec.record("50 MB JSON to the tree", performance.now() - r0, "ms");
  await rec.record("50 MB JSON longest frame", stop(), "ms");
}
