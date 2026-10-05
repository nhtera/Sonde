// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect } from "react";

// Fade-and-rise on scroll. Content is visible without JavaScript: elements
// are hidden only under html.js (set in the document head) and revealed by
// an IntersectionObserver; reduced motion shows everything at once.

/**
 * Self-contained (it is also serialized into an inline script). Returns a
 * cleanup that stops observing, for the effect.
 */
function revealOnScroll() {
  const els = document.querySelectorAll(".reveal:not(.in)");
  if (!("IntersectionObserver" in window) || matchMedia("(prefers-reduced-motion: reduce)").matches) {
    els.forEach((e) => e.classList.add("in"));
    return () => {};
  }
  const io = new IntersectionObserver(
    (entries) =>
      entries.forEach((e) => {
        if (e.isIntersecting) {
          e.target.classList.add("in");
          io.unobserve(e.target);
        }
      }),
    { rootMargin: "0px 0px -8% 0px" },
  );
  els.forEach((e) => io.observe(e));
  return () => io.disconnect();
}

/** Marks the document as scripted before first paint (for the root head). */
export const JS_CLASS_SCRIPT = "document.documentElement.classList.add('js')";

/**
 * Runs the reveal twice over: as an inline script on the prerendered page
 * (before hydration, no bundle needed), and on mount, for client-side
 * navigation, where React does not run inline scripts.
 */
export function Reveal() {
  useEffect(revealOnScroll, []);
  return <script dangerouslySetInnerHTML={{ __html: `(${revealOnScroll.toString()})();` }} />;
}
