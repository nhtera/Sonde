// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// App events. The desktop window gets them as Wails events; server mode
// (and the harness) reads its guarded event stream, one JSON line per
// event, and reconnects when dropped (a page that lags is disconnected).

import { Events } from "@wailsio/runtime";
import { serverMode } from "./mode";

type Handler = (data: unknown) => void;

const handlers = new Map<string, Set<Handler>>();
const reconnectHandlers = new Set<() => void>();

function dispatch(topic: string, data: unknown) {
  handlers.get(topic)?.forEach((h) => h(data));
}

/** Subscribes to topic; returns the unsubscribe function. */
export function on(topic: string, handler: Handler): () => void {
  let set = handlers.get(topic);
  if (!set) {
    set = new Set();
    handlers.set(topic, set);
    if (!serverMode) {
      const off = Events.On(topic, (ev) => dispatch(topic, ev.data));
      wailsOff.set(topic, off);
    }
  }
  set.add(handler);
  if (serverMode) ensureStream();
  return () => {
    const s = handlers.get(topic);
    if (!s) return;
    s.delete(handler);
    if (s.size === 0) {
      handlers.delete(topic);
      wailsOff.get(topic)?.();
      wailsOff.delete(topic);
    }
  };
}

/** Runs handler after the event stream reconnected (events may be lost). */
export function onReconnect(handler: () => void): () => void {
  reconnectHandlers.add(handler);
  return () => reconnectHandlers.delete(handler);
}

const wailsOff = new Map<string, () => void>();

let streaming = false;

/** Reads the event stream, reconnecting with backoff. */
function ensureStream() {
  if (streaming) return;
  streaming = true;
  let attempt = 0;
  const connect = async () => {
    try {
      const res = await fetch("/_sonde/events", { cache: "no-store" });
      if (!res.ok || !res.body) throw new Error(`event stream: ${res.status}`);
      if (attempt > 0) reconnectHandlers.forEach((h) => h());
      attempt = 0;
      await readLines(res.body, (line) => {
        const ev = JSON.parse(line) as { topic: string; data: unknown };
        dispatch(ev.topic, ev.data);
      });
    } catch {
      // reconnect below
    }
    attempt++;
    setTimeout(connect, Math.min(5000, 200 * 2 ** Math.min(attempt, 5)));
  };
  void connect();
}

/** Calls onLine for each line of a byte stream. */
export async function readLines(body: ReadableStream<Uint8Array>, onLine: (line: string) => void) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let i: number;
    while ((i = buf.indexOf("\n")) >= 0) {
      const line = buf.slice(0, i);
      buf = buf.slice(i + 1);
      if (line.trim()) onLine(line);
    }
  }
}
