// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from "react";

export type PillTone = "pass" | "fail" | "warn" | "neutral";

/** A result pill: Passed / Failed / Skipped / Running. */
export function Pill({ tone, children }: { tone: PillTone; children: ReactNode }) {
  return <span className={`pill ${tone}`}>{children}</span>;
}
