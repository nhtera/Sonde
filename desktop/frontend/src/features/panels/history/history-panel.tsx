// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// History: the runs and sends of this project, newest first, kept in the
// app's data (never the project), redacted. Opening one shows it
// read-only in the Results panel.

import { useEffect, useState } from "react";
import { confirm } from "../../../components/ask";
import { appError, History, type HistoryItem } from "../../../lib/api";
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

export function HistoryPanel() {
  const [items, setItems] = useState<HistoryItem[]>([]);
  const [filter, setFilter] = useState("");
  const enabled = useSettings((s) => s.value?.history.enabled ?? true);
  const shown = useHistoryView((s) => s.view?.id);
  useEffect(() => {
    const load = () => void History.List().then((l) => setItems(l ?? []), fail);
    load();
    return on("history:changed", load);
  }, []);
  const open = async (item: HistoryItem) => {
    try {
      const rec = await History.Get(item.id);
      const file = item.files?.[0];
      if (!rec || !file) return;
      const run = historyRun(rec, file);
      if (!run) return;
      await useTabs.getState().open(file);
      useHistoryView.getState().show({ id: item.id, file, at: item.at, run });
    } catch (err) {
      fail(err);
    }
  };
  const q = filter.trim().toLowerCase();
  const list = items.filter((i) => !q || [i.kind, i.outcome, i.env, ...(i.files ?? [])].some((s) => s.toLowerCase().includes(q)));
  // A day heading before each day's first item.
  const days = list.map((i) => dayOf(i.at));
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
      <input className="panel-filter" aria-label="Filter the history" placeholder="Filter by file, env or outcome" value={filter} onChange={(e) => setFilter(e.target.value)} />
      {!enabled && <p className="form-note">History is off (Settings › History &amp; privacy).</p>}
      <ul className="history-list" aria-label="History">
        {list.map((i, n) => {
          const day = days[n];
          const head = n === 0 || days[n - 1] !== day;
          return (
            <li key={i.id}>
              {head && <div className="day muted small">{day}</div>}
              <button aria-pressed={shown === i.id} onClick={() => void open(i)}>
                <span className={`kind k-${i.kind}`}>{i.kind}</span>
                <span className="mono file">{(i.files ?? []).join(", ")}</span>
                <span className={`outcome outcome-${i.outcome}`}>{i.outcome}</span>
                <span className="muted small mono">
                  <span data-volatile>
                    {new Date(i.at).toLocaleTimeString()} · {i.durationMs} ms
                  </span>{" "}
                  · {i.requests} req
                </span>
              </button>
            </li>
          );
        })}
        {list.length === 0 && <li className="muted small">{items.length ? "Nothing matches." : "No runs yet."}</li>}
      </ul>
      <p className="side-note small">
        Stored in the app&apos;s data, not in the project. Authorization, Cookie and Set-Cookie values, captured tokens and declared secrets are saved as ***.
      </p>
    </div>
  );
}
