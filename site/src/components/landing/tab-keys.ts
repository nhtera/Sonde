// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { KeyboardEvent } from "react";

/**
 * The tab a key moves to in a tablist (roving tabindex): arrows step and
 * wrap, Home and End jump to the ends. Undefined for any other key.
 */
export function nextTab(e: KeyboardEvent, current: number, count: number): number | undefined {
  switch (e.key) {
    case "ArrowRight":
    case "ArrowDown":
      return (current + 1) % count;
    case "ArrowLeft":
    case "ArrowUp":
      return (current - 1 + count) % count;
    case "Home":
      return 0;
    case "End":
      return count - 1;
    default:
      return undefined;
  }
}
