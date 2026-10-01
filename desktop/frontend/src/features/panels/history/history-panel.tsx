// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// History: the requests this project's runs and sends made, newest first,
// kept in the app's data (never the project), redacted. Opening one shows
// its run read-only in the Results panel, on that request.

import { useEffect, useState } from "react";
import { confirm } from "../../../components/ask";
import { LockIcon } from "../../../components/icons";
import { displayPath } from "../../../components/run/model";
import { appError, History, type HistoryCall, type HistoryItem } from "../../../lib/api";
import { on } from "../../../lib/events";
import { historyRun, useHistoryView } from "../../../state/history-view";
import { useSettings } from "../../../state/settings";
import { useTabs } from "../../../state/tabs";
import { useUI } from "../../../state/ui";

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

/** "Today", "Yesterday" or the date of an ISO time. */
export function dayOf(iso: string, now = new Date()): string {
  const d = new Date(iso);
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((day(now) - day(d)) / 86_400_000);
  if (diff === 0) return "Today";
  if (diff === 1) return "Yesterday";
  return d.toLocaleDateString();
}

interface Row {
  item: HistoryItem;
  call?: HistoryCall;
}

export function HistoryPanel() {
  const [items, setItems] = useState<HistoryItem[]>([]);
  const [filter, setFilter] = useState("");
  const enabled = useSettings((s) => s.value?.history.enabled ?? true);
  const shown = useHistoryView((s) => (s.view ? `${s.view.id}:${s.view.result ?? ""}:${s.view.entry ?? ""}` : ""));
  useEffect(() => {
    const load = () => void History.List().then((l) => setItems(l ?? []), fail);
    load();
    return on("history:changed", load);
  }, []);
  const open = async (item: HistoryItem, call?: HistoryCall) => {
    try {
      const rec = await History.Get(item.id);
      const file = call?.file ?? item.files?.[0];
      if (!rec || !file) return;
      const run = historyRun(rec, file, call?.result);
      if (!run) return;
      await useTabs.getState().open(file);
      useHistoryView.getState().show({ id: item.id, file, at: item.at, run, entry: call?.entry, result: call?.result });
    } catch (err) {
      fail(err);
    }
  };
  // A row per request (a run that sent none: one for the run).
  const rows: Row[] = items.flatMap((item) => (item.calls?.length ? item.calls.map((call) => ({ item, call })) : [{ item }]));
  const q = filter.trim().toLowerCase();
  const list = rows.filter(
    ({ item, call }) =>
      !q ||
      (call ? [call.method, displayPath(call.url), String(call.status), call.file] : [item.kind, item.outcome, ...(item.files ?? [])]).some((s) => s.toLowerCase().includes(q)),
  );
  // A day heading before each day's first row.
  const days = list.map((r) => dayOf(r.item.at));
  return (
    <div className="panel-body">
      <div className="panel-head">
        <h2>History</h2>
        <button
          className="btn-ghost"
          disabled={items.length === 0}
          onClick={async () => {
            if (await confirm({ title: "Clear the history?", message: "Every run and send kept for this project is deleted.", submit: "Clear" })) {
              await History.Clear().then(() => setItems([]), fail);
            }
          }}
        >
          Clear
        </button>
      </div>
      <input className="panel-filter" aria-label="Filter the history" placeholder="Filter by path, status or file" value={filter} onChange={(e) => setFilter(e.target.value)} />
      {!enabled && <p className="form-note">History is off (Settings › History &amp; privacy).</p>}
      <ul className="history-list" aria-label="History">
        {list.map(({ item, call }, n) => {
          const day = days[n];
          const head = n === 0 || days[n - 1] !== day;
          const time = new Date(item.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
          return (
            <li key={`${item.id}:${call?.result ?? ""}:${call?.entry ?? ""}`}>
              {head && <div className="day muted small">{day}</div>}
              {call ? (
                <button aria-pressed={shown === `${item.id}:${call.result}:${call.entry}`} onClick={() => void open(item, call)}>
                  <span className={`method m-${call.method.toLowerCase()}`}>{call.method}</span>
                  <span className="mono path" title={call.url}>
                    {displayPath(call.url)}
                  </span>
                  <span className={`status ${call.status >= 400 || call.status === 0 ? "fail" : "pass"}`}>{call.status || "ERR"}</span>
                  <span className="muted small mono">
                    {call.file} · <span data-volatile>{time} · {call.durationMs} ms</span>
                  </span>
                </button>
              ) : (
                <button aria-pressed={shown === `${item.id}::`} onClick={() => void open(item)}>
                  <span className="method">{item.kind}</span>
                  <span className="mono path">{(item.files ?? []).join(", ")}</span>
                  <span className={`status ${item.outcome === "passed" ? "pass" : "fail"}`}>{item.outcome}</span>
                  <span className="muted small mono">
                    <span data-volatile>{time}</span> · no request sent
                  </span>
                </button>
              )}
            </li>
          );
        })}
        {list.length === 0 && <li className="muted small">{items.length ? "Nothing matches." : "No runs yet."}</li>}
      </ul>
      <p className="side-note small history-note">
        <LockIcon />
        <span>Stored in the app&apos;s data, not in the project. Authorization, Cookie and Set-Cookie values, captured tokens and declared secrets are saved as ***.</span>
      </p>
    </div>
  );
}
