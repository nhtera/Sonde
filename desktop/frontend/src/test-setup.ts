// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach } from "vitest";

// Without vitest globals, Testing Library does not unmount on its own.
afterEach(cleanup);

// The Wails runtime starts a 50 ms timer on import. A file that ends
// sooner tears down the window first, and the timer then throws: the
// timers still set end with the file. (Files run in node have no window.)
if (typeof window !== "undefined") {
  const timers = new Set<number>();
  const setTimer = window.setInterval.bind(window);
  const clearTimer = window.clearInterval.bind(window);
  window.setInterval = ((...args: Parameters<typeof window.setInterval>) => {
    const id = setTimer(...args);
    timers.add(id);
    return id;
  }) as typeof window.setInterval;
  window.clearInterval = ((id?: number) => {
    if (id !== undefined) timers.delete(id);
    clearTimer(id);
  }) as typeof window.clearInterval;
  afterAll(() => timers.forEach(clearTimer));
}
