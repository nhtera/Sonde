// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Test-only hooks of the e2e harness and the spikes. Loaded only by the
// harness build (vite --mode harness).

import { Call, Events } from "@wailsio/runtime";
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
