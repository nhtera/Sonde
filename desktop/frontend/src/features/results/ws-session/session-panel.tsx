// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// An interactive WebSocket session on a request of the file, in place of
// the results: dialed as a run would dial it, then driven by hand. It is
// not part of the test and writes nothing unless asked.

import { useEffect, useRef, useState } from "react";
import { appError, EditSvc, WSession } from "../../../lib/api";
import { on } from "../../../lib/events";
import type { StreamMessage } from "../../../lib/view";
import { applyEdit, type EditResult } from "../../../state/edits";
import { useEnv } from "../../../state/env";
import { useTabs } from "../../../state/tabs";
import { useUI } from "../../../state/ui";
import { useResults } from "../state";
import { MAX_MESSAGES, MessageTable } from "../tabs/stream-log";
import { Composer } from "./composer";

type Status = "opening" | "open" | "closed";

interface SessionEvent {
  type: "message" | "closed";
  message?: StreamMessage;
  error?: string;
}

const newId = () => `s${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;

/** "00:42" since start. */
function elapsed(start: number, now: number) {
  const s = Math.max(0, Math.floor((now - start) / 1000));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

export function SessionPanel({ file, entry }: { file: string; entry: number }) {
  const [status, setStatus] = useState<Status>("opening");
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");
  const [messages, setMessages] = useState<StreamMessage[]>([]);
  const [opened, setOpened] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const id = useRef("");

  useEffect(() => {
    // A fresh id per open (an effect that runs twice opens twice).
    const sid = newId();
    id.current = sid;
    let live = true;
    // Subscribe first: no message is missed.
    const off = on(`wsession:${sid}`, (data) => {
      const ev = data as SessionEvent;
      if (ev.type === "message" && ev.message) setMessages((m) => [...m.slice(m.length >= MAX_MESSAGES ? 1 : 0), ev.message!]);
      if (ev.type === "closed") {
        setStatus("closed");
        if (ev.error) setError(ev.error);
      }
    });
    const tab = useTabs.getState().tabs.find((t) => t.path === file);
    WSession.Open({ sessionId: sid, file, source: tab?.text ?? "", env: useEnv.getState().current, entry }).then(
      (o) => {
        // Closed while it opened (Go cancels a dial it sees closed; this
        // covers a Close that came before the dial began).
        if (!live) {
          void WSession.Close(sid);
          return;
        }
        setUrl(o?.url ?? "");
        setOpened(Date.now());
        setStatus((s) => (s === "opening" ? "open" : s));
      },
      (err) => {
        if (!live) return;
        setStatus("closed");
        setError(appError(err).message);
      },
    );
    return () => {
      live = false;
      off();
      void WSession.Close(sid);
    };
  }, [file, entry]);

  useEffect(() => {
    if (status !== "open") return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [status]);

  const close = () => useResults.getState().closeSession();
  // Esc closes the session from the panel (or with nothing focused), never
  // an Esc the editor or a dialog handled.
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented || document.querySelector('[role="dialog"][data-state="open"]')) return;
      const t = e.target as Node;
      if (t === document.body || panel.current?.contains(t)) close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const send = async (data: string, binary: boolean, write: boolean) => {
    const fail = (err: unknown) => {
      useUI.getState().toast({ kind: "error", text: appError(err).message });
      return false;
    };
    // The steps first: a message the file cannot hold is not sent either.
    let steps: EditResult | null = null;
    if (write) {
      const tab = useTabs.getState().tabs.find((t) => t.path === file);
      if (!tab) return fail(new Error("the file is not open"));
      try {
        steps = (await EditSvc.AppendMessages({ file, text: tab.text, version: tab.version }, entry, data, binary)) as EditResult | null;
      } catch (err) {
        return fail(err);
      }
    }
    try {
      await WSession.Send(id.current, data, binary);
    } catch (err) {
      return fail(err);
    }
    if (steps && !applyEdit(file, steps)) useUI.getState().toast({ kind: "warn", text: "The file changed meanwhile: the steps were not written" });
    return true;
  };

  return (
    <div className="session-panel" aria-label="Interactive session" ref={panel}>
      <header className="session-head">
        <div className="session-title">
          <h2>Session</h2>
          <span className={`session-state ${status}`}>
            <i aria-hidden />
            {status === "open" ? "Open" : status === "closed" ? "Closed" : "Opening"}
          </span>
          <button className="session-close" onClick={close} aria-keyshortcuts="Escape">
            Close
          </button>
          <kbd>Esc</kbd>
        </div>
        <div className="session-url mono">
          {url || "…"}
          {opened > 0 && status === "open" && ` · opened ${elapsed(opened, now)}`}
        </div>
        <p className="session-warn">Interactive session, not part of the test. Nothing here is written unless you ask.</p>
        {error && <p className="run-error">{error}</p>}
      </header>
      <MessageTable messages={messages} protocol="websocket" filters={false} />
      <Composer disabled={status !== "open"} onSend={send} />
    </div>
  );
}
