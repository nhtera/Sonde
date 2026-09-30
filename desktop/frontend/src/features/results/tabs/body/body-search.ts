// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Finding text in a body's bytes without blocking the page: the bytes are
// scanned a few megabytes per task, ASCII letters matched in any case.

/** Matches kept at most. */
export const MAX_MATCHES = 10_000;
const CHUNK = 4 << 20;

const lower = (b: number) => (b >= 0x41 && b <= 0x5a ? b + 32 : b);

/** The byte offsets where query occurs in bytes (case-insensitive for
 * ASCII), at most MAX_MATCHES; [] when aborted. */
export async function byteMatches(bytes: Uint8Array, query: string, signal?: AbortSignal): Promise<number[]> {
  const q = new TextEncoder().encode(query).map(lower);
  if (q.length === 0) return [];
  const out: number[] = [];
  const first = q[0];
  const last = bytes.length - q.length;
  let pause = CHUNK;
  outer: for (let i = 0; i <= last; i++) {
    if (i >= pause) {
      pause += CHUNK;
      await new Promise((r) => setTimeout(r, 0));
      if (signal?.aborted) return [];
    }
    if (lower(bytes[i]) !== first) continue;
    for (let j = 1; j < q.length; j++) if (lower(bytes[i + j]) !== q[j]) continue outer;
    out.push(i);
    if (out.length >= MAX_MATCHES) return out;
    i += q.length - 1;
  }
  return out;
}
