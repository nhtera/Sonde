// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

/** The dot color of an environment: red for production, amber for staging. */
export function envColor(name: string): string {
  return /prod|live/i.test(name) ? "var(--fail)" : /stag|pre|qa|test/i.test(name) ? "var(--warn)" : "var(--pass)";
}
