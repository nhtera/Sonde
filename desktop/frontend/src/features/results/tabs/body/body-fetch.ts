// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A response body's text, read from its URL (redacted bytes), at most
// limit bytes of it; the last few bodies read are kept.

export const bodyURL = (id: string) => `/_sonde/body/${id}`;

export interface BodyText {
  text: string;
  /** The body was longer than the limit. */
  cut: boolean;
  bytes: Uint8Array;
}

const cache = new Map<string, Promise<BodyText>>();
const KEEP = 6;

/** Reads up to limit bytes of body id. */
export function fetchBody(id: string, limit: number): Promise<BodyText> {
  const key = `${id}:${limit}`;
  const had = cache.get(key);
  if (had) return had;
  const p = read(id, limit);
  cache.set(key, p);
  p.catch(() => cache.delete(key));
  while (cache.size > KEEP) cache.delete(cache.keys().next().value!);
  return p;
}

async function read(id: string, limit: number): Promise<BodyText> {
  const ctl = new AbortController();
  const res = await fetch(bodyURL(id), { cache: "no-store", credentials: "same-origin", signal: ctl.signal });
  if (!res.ok || !res.body) throw new Error(`the response is no longer available (${res.status})`);
  const reader = res.body.getReader();
  const parts: Uint8Array[] = [];
  let size = 0;
  let cut = false;
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    parts.push(value);
    size += value.length;
    if (size > limit) {
      cut = true;
      ctl.abort();
      break;
    }
  }
  const bytes = new Uint8Array(Math.min(size, limit));
  let off = 0;
  for (const p of parts) {
    const n = Math.min(p.length, bytes.length - off);
    bytes.set(p.subarray(0, n), off);
    off += n;
    if (off >= bytes.length) break;
  }
  return { text: new TextDecoder().decode(bytes), cut, bytes };
}

/** A hex dump of bytes: offset, 16 bytes, their printable characters. */
export function hexDump(bytes: Uint8Array, max = 4096): string {
  const lines: string[] = [];
  for (let off = 0; off < Math.min(bytes.length, max); off += 16) {
    const row = bytes.subarray(off, off + 16);
    const hex = [...row].map((b) => b.toString(16).padStart(2, "0")).join(" ");
    const chars = [...row].map((b) => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : ".")).join("");
    lines.push(`${off.toString(16).padStart(8, "0")}  ${hex.padEnd(47)}  ${chars}`);
  }
  if (bytes.length > max) lines.push(`… ${bytes.length - max} more bytes`);
  return lines.join("\n");
}
