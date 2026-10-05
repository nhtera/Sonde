// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

/**
 * A captured value: `→ token` (captured by this request, teal) or
 * `← token` (used by it, outlined).
 */
export function CaptureChip({ name, direction }: { name: string; direction: "out" | "in" }) {
  return direction === "out" ? <span className="chip cap">→ {name}</span> : <span className="chip">← {name}</span>;
}
