// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// One request of a run: its meta line ("200 OK GET /orders · 28 ms ·
// 193 B"), then its detail tabs.

import { displayPath } from "../../components/run/model";
import type { Entry } from "../../lib/view";
import type { FileRun } from "../../state/run-model";
import { cardOf, formatBytes, formatMs, statusText, stopText, tabCounts } from "./model";
import { shownTab, tabsOf, useResults, type TabId } from "./state";
import { AssertsTab } from "./tabs/asserts";
import { BodyTab } from "./tabs/body/body-tab";
import { CapturesTab } from "./tabs/captures";
import { CookiesTab } from "./tabs/cookies";
import { HeadersTab } from "./tabs/headers";
import { RequestTab } from "./tabs/request";
import { MessageTable } from "./tabs/stream-log";
import { TimelineTab } from "./tabs/timeline";
import { ErrorCard } from "./error-cards";

const titles: Record<TabId, string> = {
  error: "Error", stream: "Stream", body: "Body", headers: "Headers", asserts: "Asserts",
  captures: "Captures", cookies: "Cookies", timeline: "Timeline", request: "Request",
};

export interface EntryDetailProps {
  file: string;
  run: FileRun;
  entry: number;
  /** The text that ran, by line. */
  lines: string[];
}

function Meta({ e, run, entry }: { e?: Entry; run: FileRun; entry: number }) {
  const call = e?.calls?.at(-1);
  const sending = run.sending[entry];
  if (call) {
    const status = call.response.status;
    const size = e!.bodies?.at(-1)?.size ?? 0;
    return (
      <div className="entry-meta">
        <b className={`mono ${status >= 400 ? "fail" : "pass"}`}>{statusText(status)}</b>
        <span className="mono path">
          {call.request.method} {displayPath(call.request.url)}
        </span>
        <span className="mono right" data-volatile>
          {formatMs(e!.time)} · {formatBytes(size)}
        </span>
      </div>
    );
  }
  if (e) {
    const card = cardOf(e.errors?.[0]);
    return (
      <div className="entry-meta">
        <b className={`mono ${card?.neutral ? "" : "fail"}`}>{card?.neutral ? "Canceled" : "No response"}</b>
        <span className="mono right">{card?.code ?? ""}</span>
      </div>
    );
  }
  if (sending) {
    return (
      <div className="entry-meta">
        <b className="mono">Sending…</b>
        <span className="mono path">
          {sending.method} {displayPath(sending.url)}
        </span>
      </div>
    );
  }
  return null;
}

function StreamTab({ file, e, run, entry }: { file: string; e?: Entry; run: FileRun; entry: number }) {
  const stream = e?.sonde?.stream;
  const messages = stream?.messages ?? run.messages[entry] ?? [];
  const protocol = stream?.protocol ?? (messages.some((m) => m.direction === "sent") ? "websocket" : "sse");
  return (
    <div className="stream-tab">
      <MessageTable
        messages={messages}
        protocol={protocol}
        dropped={run.running ? run.dropped : 0}
        actions={
          protocol === "websocket" && (
            <button className="chip" onClick={() => useResults.getState().openSession(file, entry)}>
              Open interactive session
            </button>
          )
        }
      />
      {stream && (
        <p className="stream-stop">
          <b>Stopped: {stopText(stream)}.</b>{" "}
          {protocol === "sse"
            ? "Heartbeats and comments carry no data, so they are not dispatched. A stream also stops at sonde-stream-timeout, sonde-stream-max-bytes, or when the server closes it."
            : protocol === "websocket"
              ? "Transcript of the scripted run only."
              : ""}
        </p>
      )}
    </div>
  );
}

export function EntryDetail({ file, run, entry, lines }: EntryDetailProps) {
  const e = run.entries[entry];
  const picked = useResults((s) => s.tab);
  const live = !e && (run.messages[entry]?.length ?? 0) > 0;
  const tabs: TabId[] = e ? tabsOf(e) : live ? ["stream", "timeline", "request"] : tabsOf(undefined);
  const tab = e ? shownTab(e, run.runId, picked) : picked?.runId === run.runId && tabs.includes(picked.value) ? picked.value : tabs[0];
  const counts = e ? tabCounts(e) : null;
  const logs = run.logs.filter((l) => l.entry === entry);
  const skipped = run.skipped[entry];

  if (!e && !run.sending[entry] && !live) {
    return (
      <div className="entry-detail">
        <p className="body-note tab-pad">
          {skipped
            ? `Request ${entry} was skipped (${skipped === "repeat-zero" ? "repeat: 0" : "skip option"}).`
            : run.running
              ? `Request ${entry} has not run yet.`
              : `Request ${entry} did not run in this run.`}
        </p>
      </div>
    );
  }

  const label = (t: TabId) => {
    if (!counts) return titles[t];
    switch (t) {
      case "stream":
        return (
          <>
            Stream <span className="n">{counts.stream}</span>
          </>
        );
      case "headers":
        return (
          <>
            Headers <span className="n">{counts.headers}</span>
          </>
        );
      case "asserts":
        return counts.asserts.total ? (
          <>
            Asserts{" "}
            <span className={`n ${counts.asserts.failed ? "fail" : ""}`}>
              {counts.asserts.failed ? counts.asserts.failed : counts.asserts.total}/{counts.asserts.total}
            </span>
          </>
        ) : (
          titles[t]
        );
      case "cookies":
        return counts.cookies ? (
          <>
            Cookies <span className="n">{counts.cookies}</span>
          </>
        ) : (
          titles[t]
        );
      default:
        return titles[t];
    }
  };

  const body = e?.bodies?.at(-1);
  let content;
  switch (tab) {
    case "error":
      content = e?.errors?.[0] ? <ErrorCard file={file} entry={entry} err={e.errors[0]} /> : null;
      break;
    case "stream":
      content = <StreamTab file={file} e={e} run={run} entry={entry} />;
      break;
    case "body":
      content = body?.id ? (
        <BodyTab file={file} entry={entry} body={body} />
      ) : (
        <p className="body-note tab-pad">{body?.error ? `The body could not be decoded: ${body.error}` : "No body."}</p>
      );
      break;
    case "headers":
      content = e && <HeadersTab entry={e} />;
      break;
    case "asserts":
      content = e && <AssertsTab entry={e} lines={lines} onShowResponse={() => useResults.getState().setTab(run.runId, "body")} />;
      break;
    case "captures":
      content = e && <CapturesTab entry={e} />;
      break;
    case "cookies":
      content = e && <CookiesTab file={file} entry={e} entries={run.entries} />;
      break;
    case "timeline":
      content = <TimelineTab entry={e} logs={logs} sending={run.sending[entry]} />;
      break;
    case "request":
      content = <RequestTab entry={e} sending={run.sending[entry]} />;
      break;
  }

  return (
    <div className="entry-detail">
      <Meta e={e} run={run} entry={entry} />
      <div className="detail-tabs" role="tablist" aria-label="Request details">
        {tabs.map((t) => (
          <button key={t} role="tab" aria-selected={t === tab} onClick={() => useResults.getState().setTab(run.runId, t)}>
            {label(t)}
          </button>
        ))}
      </div>
      <div className={`detail-body tab-${tab}`} role="tabpanel" aria-label={titles[tab]}>
        {content}
      </div>
    </div>
  );
}
