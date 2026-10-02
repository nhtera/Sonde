// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The request's headers, and greyed below them the ones Sonde adds: as the
// last run sent them, else as it would (Host, User-Agent, Content-Type
// from the body, Accept-Encoding when compressed).

import { useRuns } from "../../../state/run";
import { KvGrid } from "../kv-grid";
import { authOf, bodyKindOf, rowsOf, useForm, type EntryModel } from "../model";
import { listSuggester } from "../suggest-input";
import { headerNames } from "./header-names";

const contentTypes: Partial<Record<string, string>> = {
  json: "application/json",
  xml: "application/xml",
  urlencoded: "application/x-www-form-urlencoded",
  "form-data": "multipart/form-data; boundary=…",
  graphql: "application/json",
  text: "text/plain",
};

/** The headers Sonde adds to entry's request: those the last run sent
 * that the file does not write, else predicted. */
export function automaticHeaders(entry: EntryModel, sent?: { name: string; value: string }[]): { name: string; value: string }[] {
  const own = new Set(rowsOf(entry, "headers").filter((r) => !r.Disabled).map((r) => r.Key.toLowerCase()));
  if (sent) return sent.filter((h) => !own.has(h.name.toLowerCase()));
  const out: { name: string; value: string }[] = [];
  const host = /^[a-z]+:\/\/([^/?#]+)/i.exec(entry.URL)?.[1];
  out.push({ name: "Host", value: host ?? "from the URL" });
  out.push({ name: "Accept", value: "*/*" });
  out.push({ name: "User-Agent", value: "sonde/<version>" });
  const ct = contentTypes[bodyKindOf(entry)];
  if (ct) out.push({ name: "Content-Type", value: ct });
  if (bodyKindOf(entry) !== "none") out.push({ name: "Content-Length", value: "the body's size" });
  if (rowsOf(entry, "options").some((r) => !r.Disabled && r.Key === "compressed" && r.Value === "true")) {
    out.push({ name: "Accept-Encoding", value: "gzip, deflate, br" });
  }
  return out.filter((h) => !own.has(h.name.toLowerCase()));
}

export function HeadersTab({ file, entry }: { file: string; entry: EntryModel }) {
  const sent = useRuns((s) => s.runs[file]?.entries[entry.Index]?.calls?.[0]?.request.headers ?? undefined);
  const auto = automaticHeaders(entry, sent);
  // Names offered: those the request does not set yet.
  const own = new Set(rowsOf(entry, "headers").map((r) => r.Key.toLowerCase()));
  const names = listSuggester(headerNames.filter((h) => !own.has(h.name.toLowerCase())));
  const auth = authOf(entry);
  return (
    <div className="form-tab">
      <KvGrid
        file={file}
        entry={entry}
        sec="headers"
        keyLabel="Header"
        keySuggest={names}
        managed={(_, i) => (auth.sec === "headers" && auth.index === i ? { title: "Set in the Auth tab", open: () => useForm.getState().setTab("auth") } : undefined)}
      />
      <section className="auto-headers" aria-label="Headers Sonde adds">
        <div className="section-note">
          <span>Added by Sonde{sent ? " (as the last run sent them)" : ""}</span>
          <span>Add a header with the same name to override</span>
        </div>
        {auto.map((h) => (
          <div key={h.name} className="auto-row mono">
            <span className="auto-tag">auto</span>
            <span>{h.name}</span>
            <span className="muted">{h.value}</span>
          </div>
        ))}
      </section>
    </div>
  );
}
