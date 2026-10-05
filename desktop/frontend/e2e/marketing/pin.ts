// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Marketing shots show times, not blanks: the values that change from run to
// run (durations, "12s ago", the stream's offsets) are replaced by fixed ones
// that read as a real run (the design's), where the visual baselines hide
// them (e2e/visual/visual.css).

import type { Page } from "@playwright/test";

/** Rewrites the volatile values on screen to fixed ones; call it on the
 * frame to capture, after the last change (React may render the real ones
 * again). */
export async function pinVolatile(page: Page) {
  await page.evaluate(() => {
    const ago = "12s ago";
    const requestMs = [29, 29, 20, 10, 28, 24, 18];
    const fileMs = ["31 ms", "116 ms", "1.20 s", "88 ms", "4 ms"];
    const set = (el: Element, text: string) => {
      if (el.textContent !== text) el.textContent = text;
    };
    // Per request in the run's list.
    document.querySelectorAll(".req-row .ms").forEach((el, i) => {
      if (el.textContent?.trim()) set(el, `${requestMs[i % requestMs.length]} ms`);
    });
    // The time each request took, after its status in the editor.
    let line = 0;
    document.querySelectorAll<HTMLElement>(".cm-ghosted[data-ghost]").forEach((el) => {
      const g = el.dataset.ghost ?? "";
      if (/^\d+ · \d+ ms$/.test(g)) el.dataset.ghost = `${g.split(" · ")[0]} · ${requestMs[line++ % requestMs.length]} ms`;
    });
    // Ids the fixture counts up (a cart, its order): the first run's.
    const ids = (t: string) => t.replace(/\bo-c\d+|\bc\d+\b/g, (m) => (m.startsWith("o-") ? "o-c1" : "c1"));
    document.querySelectorAll<HTMLElement>(".cm-ghosted[data-ghost]").forEach((el) => (el.dataset.ghost = ids(el.dataset.ghost ?? "")));
    const texts = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let n = texts.nextNode(); n; n = texts.nextNode()) {
      const v = n.nodeValue ?? "";
      if (ids(v) !== v) n.nodeValue = ids(v);
    }
    // The test summary's duration and rate (10 requests in 116 ms).
    document.querySelectorAll("pre[aria-label='Test summary'] > span").forEach((line) => {
      const label = line.querySelector(".label")?.textContent ?? "";
      const value = line.querySelector(":scope > span:not(.label):not(.rest)");
      const rest = line.querySelector(".rest");
      if (label.startsWith("Duration") && value && rest) {
        set(value, "116");
        set(rest, " ms (0h:0m:0s:116ms)");
      } else if (label.startsWith("Executed requests") && rest) set(rest, " (86.2/s)");
    });
    // Per file in the Test run table.
    document.querySelectorAll("td[data-volatile]").forEach((el, i) => {
      if (el.textContent?.trim()) set(el, fileMs[i % fileMs.length]);
    });
    // The stream's offsets: a heartbeat shares its event's time.
    document.querySelectorAll(".stream-row .t").forEach((el, i) => set(el, `+${(0.02 * Math.floor(i / 2 + 1)).toFixed(2)}`));
    // The copied command's note says where to run it: the project's folder,
    // a host path (the design has no note).
    document.querySelectorAll("p.muted.small").forEach((el) => {
      if (el.textContent?.startsWith("Run it in ")) el.remove();
    });
    // The agent's command names the project's folder: where a user keeps it.
    document.querySelectorAll("pre.snippet").forEach((el) => set(el, (el.textContent ?? "").replaceAll("/tmp/sonde-showcase/", "~/code/")));
    for (const el of document.querySelectorAll("[data-volatile]")) {
      if (el.matches("td")) continue;
      const t = el.textContent?.trim() ?? "";
      if (el.matches(".ms")) continue;
      if (/^(just now|\d+[smh] ago)$/.test(t)) set(el, ago);
      else if (/^· finished \d{1,2}:\d{2}/.test(t)) set(el, `· finished ${ago}`);
      else if (/^\d+ ms$/.test(t)) set(el, "116 ms");
      else if (/^[\d.]+ (ms|s) · /.test(t)) set(el, t.replace(/^[\d.]+ (ms|s)/, "28 ms"));
      else if (/^[\d.]+ s$/.test(t)) set(el, "0.12 s");
      else if (/^\d{2}:\d{2}:\d{2}$/.test(t)) set(el, "01:14:07");
    }
  });
}
