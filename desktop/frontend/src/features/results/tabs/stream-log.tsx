// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The messages of a stream: SSE events (the dispatched ones only:
// heartbeats and comments carry no data), WebSocket messages sent and
// received, gRPC messages. Live while the run is on; the last 10,000
// are kept.

import { useVirtualizer } from "@tanstack/react-virtual";
import { useMemo, useRef, useState } from "react";
import type { StreamMessage } from "../../../lib/view";

/** Messages kept in the log. */
export const MAX_MESSAGES = 10_000;

export interface MessageTableProps {
  messages: StreamMessage[];
  protocol: string;
  /** Messages the event bridge dropped (the page lagged). */
  dropped?: number;
}

/** "+5.12" seconds. */
const at = (ms: number) => `+${(ms / 1000).toFixed(2)}`;

export function MessageTable({ messages, protocol, dropped = 0 }: MessageTableProps) {
  const sse = protocol === "sse";
  const [filter, setFilter] = useState("all");
  const events = useMemo(() => (sse ? [...new Set(messages.map((m) => m.event || "message"))].slice(0, 6) : []), [messages, sse]);
  const shown = useMemo(() => {
    const nth = new Map<StreamMessage, number>();
    let n = 0;
    for (const m of messages) if (m.direction === "received") nth.set(m, n++);
    const list = messages
      .filter((m) => filter === "all" || (sse ? (m.event || "message") === filter : m.direction === filter))
      .map((m) => ({ m, nth: nth.get(m) }));
    return list.slice(-MAX_MESSAGES);
  }, [messages, filter, sse]);
  const earlier = Math.max(0, messages.length - MAX_MESSAGES) + dropped;
  const scroller = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtual = useVirtualizer({ count: shown.length, getScrollElement: () => scroller.current, estimateSize: () => 27, overscan: 20 });
  const filters = sse ? ["all", ...events] : ["all", "sent", "received"];

  return (
    <div className="stream-log">
      <div className="stream-filters" role="group" aria-label="Filter messages">
        {filters.map((f) => (
          <button key={f} className="chip" aria-pressed={filter === f} onClick={() => setFilter(f)}>
            {f === "all" ? "All" : f === "sent" ? "Sent" : f === "received" ? "Received" : f}
          </button>
        ))}
      </div>
      <div className={`stream-head mono ${sse ? "sse" : "ws"}`}>
        <span>Time</span>
        {sse ? <span>nth</span> : <span />}
        <span>{sse ? "Event" : "Type"}</span>
        <span>Data</span>
      </div>
      {earlier > 0 && <p className="body-note">{earlier.toLocaleString()} earlier messages not shown</p>}
      <div className="stream-rows" ref={scroller} role="table" aria-label="Messages">
        <div style={{ height: virtual.getTotalSize(), position: "relative" }}>
          {virtual.getVirtualItems().map((v) => {
            const { m, nth } = shown[v.index];
            return (
              <div key={v.key} role="row" className={`stream-row mono ${sse ? "sse" : "ws"} dir-${m.direction}`} style={{ transform: `translateY(${v.start}px)` }}>
                <span className="t">{at(m.time)}</span>
                {sse ? <span className="nth">{nth ?? ""}</span> : <span className="dir">{m.direction === "sent" ? "↑" : "↓"}</span>}
                <span className="ev">{sse ? m.event || "message" : m.binary ? "binary" : "text"}</span>
                <span className="data" title={m.data}>
                  {m.binary && !sse ? atobHex(m.data) : m.data}
                </span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

/** Base64 data as hex digits. */
function atobHex(b64: string): string {
  try {
    return [...atob(b64)].map((c) => c.charCodeAt(0).toString(16).padStart(2, "0")).join(" ");
  } catch {
    return b64;
  }
}
