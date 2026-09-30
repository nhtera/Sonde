// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { Entry } from "../../lib/view";

/** Asserts that passed and checks that failed (asserts and errors) of entries. */
export function counts(entries: Entry[]): { passed: number; failed: number } {
  let passed = 0;
  let failed = 0;
  for (const e of entries) {
    for (const a of e.asserts ?? []) {
      if (a.success) passed++;
      else failed++;
    }
    // An error that is not an assert (a transport error, a bad capture)
    // counts as one failed check.
    failed += (e.errors ?? []).filter((x) => !x.assert).length;
  }
  return { passed, failed };
}
