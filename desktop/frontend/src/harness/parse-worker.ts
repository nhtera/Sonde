// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Decodes and parses a JSON body off the main thread (spike baseline; the
// app's parser keeps number text, see the results phase).

self.onmessage = (e: MessageEvent<ArrayBuffer>) => {
  const t0 = performance.now();
  const value: unknown = JSON.parse(new TextDecoder().decode(e.data));
  const items = Array.isArray(value) ? value.length : 1;
  self.postMessage({ items, ms: Math.round(performance.now() - t0) });
};
