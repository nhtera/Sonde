// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { MoonIcon, SunIcon } from "@phosphor-icons/react/ssr";
import { useTheme } from "next-themes";
import { useSyncExternalStore } from "react";
import { strings } from "@/content/strings";

const noop = () => () => {};

/**
 * Dark ⇄ light. The label names the theme it switches to. Both icons are
 * rendered and CSS shows the right one from html[data-theme], so the
 * prerendered HTML is correct before hydration.
 */
export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const mounted = useSyncExternalStore(noop, () => true, () => false);
  // Before hydration the stored theme is unknown; dark is the default.
  const current = mounted ? resolvedTheme : "dark";
  const next = current === "light" ? "dark" : "light";
  return (
    <button
      type="button"
      className="icon-btn"
      aria-label={next === "light" ? strings.theme.toLight : strings.theme.toDark}
      onClick={() => setTheme(next)}
    >
      <MoonIcon className="i-moon" aria-hidden="true" />
      <SunIcon className="i-sun" aria-hidden="true" />
    </button>
  );
}
