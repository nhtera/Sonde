// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Test-only hooks of the e2e harness and the spikes. Loaded only by the
// harness build (vite --mode harness).

import { Call, Events } from "@wailsio/runtime";
import { Settings } from "../lib/api";
import { useSettings, type ThemePrefs } from "../state/settings";
import { runSpike } from "./spikes";

const svc = "github.com/nhtera/sonde/desktop/internal/host.HarnessService";

export const harness = {
  call: (method: string, ...args: unknown[]) => Call.ByName(`${svc}.${method}`, ...args),
  /** Resolves with the data of the next event called name. */
  nextEvent: (name: string) =>
    new Promise<unknown>((resolve) => {
      const off = Events.On(name, (ev) => {
        off();
        resolve(ev.data);
      });
    }),
  spike: runSpike,
  /** The theme settings as stored (the page shows a change first). */
  theme: async (): Promise<ThemePrefs> => {
    const { theme, dayTheme, nightTheme } = (await Settings.Get()).appearance;
    return { theme, dayTheme, nightTheme };
  },
  /** Sets the theme settings (a test's starting point). */
  setTheme: async (prefs: ThemePrefs) => {
    if (!useSettings.getState().value) await useSettings.getState().load();
    const v = useSettings.getState().value!;
    await useSettings.getState().save({ ...v, appearance: { ...v.appearance, ...prefs } });
  },
};

declare global {
  interface Window {
    sondeHarness?: typeof harness;
  }
}

export async function start(): Promise<void> {
  window.sondeHarness = harness;
  document.documentElement.dataset.harness = "ready";
  const spike = (await harness.call("Spike")) as string;
  if (spike) {
    let result: unknown;
    try {
      result = await runSpike(spike);
    } catch (err) {
      result = { error: String(err) };
    }
    await harness.call("Report", JSON.stringify(result));
  }
}
