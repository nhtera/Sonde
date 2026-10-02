// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What the Results panel shows for an entry, computed from its report
// (pure: the panel and its tests share it).

import type * as report from "@bindings/internal/report";
import type { Entry, EntryError, Timings } from "../../lib/view";
import { transportTitles } from "../../components/run/model";

/** Bodies above this are not formatted unless asked. */
export const FORMAT_LIMIT = 20 << 20;
/** Bodies above this are only saved or opened in another app. */
export const VIEW_LIMIT = 50 << 20;
/** How much of a large body the Raw view shows. */
export const RAW_PREVIEW = 1 << 20;

const reasons: Record<number, string> = {
  100: "Continue", 101: "Switching Protocols", 200: "OK", 201: "Created", 202: "Accepted", 204: "No Content",
  206: "Partial Content", 301: "Moved Permanently", 302: "Found", 303: "See Other", 304: "Not Modified",
  307: "Temporary Redirect", 308: "Permanent Redirect", 400: "Bad Request", 401: "Unauthorized", 403: "Forbidden",
  404: "Not Found", 405: "Method Not Allowed", 409: "Conflict", 410: "Gone", 415: "Unsupported Media Type",
  422: "Unprocessable Content", 429: "Too Many Requests", 500: "Internal Server Error", 502: "Bad Gateway",
  503: "Service Unavailable", 504: "Gateway Timeout",
};

/** "200 OK". */
export const statusText = (status: number) => `${status} ${reasons[status] ?? ""}`.trim();

/** "193 B", "3.1 kB", "48.2 MB" (1024-based, as the size limits). */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} kB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

/** "28 ms", "1.84 s". */
export const formatMs = (ms: number) => (ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(2)} s`);

/** "just now", "12s ago", "3m ago", "2h ago". */
export function ago(then: Date, now = new Date()): string {
  const s = Math.max(0, Math.round((now.getTime() - then.getTime()) / 1000));
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  return `${Math.floor(s / 3600)}h ago`;
}

/** The media type of a Content-Type, lower case, without parameters. */
export const mediaType = (ct: string) => ct.split(";")[0].trim().toLowerCase();

export type BodyKind = "json" | "xml" | "html" | "image" | "pdf" | "text" | "binary";

/** How a body is shown, from its content type. */
export function bodyKind(ct: string): BodyKind {
  const mt = mediaType(ct);
  if (mt === "application/x-ndjson") return "text";
  if (mt === "application/json" || mt.endsWith("+json")) return "json";
  if (mt === "text/html" || mt === "application/xhtml+xml") return "html";
  if (mt === "image/svg+xml") return "image";
  if (mt === "application/xml" || mt === "text/xml" || mt.endsWith("+xml")) return "xml";
  if (mt.startsWith("image/")) return "image";
  if (mt === "application/pdf") return "pdf";
  if (mt.startsWith("text/") || mt === "application/javascript" || mt === "application/x-www-form-urlencoded" || mt === "") return "text";
  return "binary";
}

export interface Segment {
  name: string;
  /** Offset and length, in milliseconds. */
  start: number;
  ms: number;
  /** A phase that did not happen ("http": no TLS). */
  skipped?: string;
}

/** A call's timings in microseconds: the view's, else the report's
 * (whole milliseconds). */
export function timingsOf(e: Entry): Timings | null {
  const t = e.timings?.at(-1);
  if (t) return t;
  const r = e.calls?.at(-1)?.timings;
  if (!r) return null;
  const us = (ms: number) => ms * 1000;
  return {
    name_lookup: us(r.name_lookup), connect: us(r.connect), app_connect: us(r.app_connect),
    pre_transfer: us(r.pre_transfer), start_transfer: us(r.start_transfer), total: us(r.total),
  };
}

/** The timing waterfall of a call (timings in microseconds). */
export function waterfall(t: Timings, secure: boolean): { segments: Segment[]; total: number } {
  const ms = (us: number) => Math.max(0, us) / 1000;
  const dns = ms(t.name_lookup);
  const connect = ms(t.connect);
  const tls = ms(t.app_connect);
  const pre = ms(t.pre_transfer) || Math.max(connect, tls);
  const first = ms(t.start_transfer);
  const total = ms(t.total);
  const segments: Segment[] = [
    { name: "DNS", start: 0, ms: dns },
    { name: "Connect", start: dns, ms: Math.max(0, connect - dns) },
    secure && tls > 0 ? { name: "TLS", start: connect, ms: Math.max(0, tls - connect) } : { name: "TLS", start: connect, ms: 0, skipped: "http" },
    { name: "Waiting", start: pre, ms: Math.max(0, first - pre) },
    { name: "Download", start: first, ms: Math.max(0, total - first) },
  ];
  return { segments, total };
}

/** Why a stream stopped, as the header and the Stream tab say it. */
export function stopText(stream: report.Stream): string {
  const n = stream.protocol === "websocket" ? stream.received : (stream.messages?.length ?? 0);
  switch (stream.stop_reason) {
    case "count":
      return `count ${n} reached`;
    case "timeout":
      return "timeout reached";
    case "max-bytes":
      return "max bytes reached";
    case "closed":
      return "the server closed the stream";
    case "script":
      return "last step done";
    default:
      return "the stream failed";
  }
}

export interface AssertRow {
  line: number;
  success: boolean;
  /** The first line of a failure's message. */
  message: string;
}

/** The asserts of an entry by line, once each: the report lists the
 * status line once per check (version, status), and a contract violation
 * both as an assert and as a violation (shown as the violation). */
export function assertRows(e: Entry): AssertRow[] {
  const contract = (e.sonde?.contract?.violations?.length ?? 0) > 0;
  const seen = new Set<string>();
  const out: AssertRow[] = [];
  for (const a of [...(e.asserts ?? [])].sort((x, y) => x.line - y.line)) {
    const message = a.success ? "" : (a.message ?? "").split("\n")[0];
    if (!a.success && contract && /contract/i.test(message)) continue;
    const key = `${a.line}:${a.success}:${message}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({ line: a.line, success: a.success, message });
  }
  return out;
}

/** Counts shown on the detail tabs. */
export function tabCounts(e: Entry) {
  const call = e.calls?.at(-1);
  const asserts = assertRows(e);
  const violations = e.sonde?.contract?.violations ?? [];
  const failed = asserts.filter((a) => !a.success).length + violations.filter((v) => !v.warning).length;
  return {
    headers: call?.response.headers?.length ?? 0,
    asserts: { total: asserts.length + violations.length, failed },
    captures: e.captures?.length ?? 0,
    cookies: (call?.request.cookies?.length ?? 0) + (call?.response.cookies?.length ?? 0),
    stream: e.sonde?.stream?.messages?.length ?? 0,
  };
}

export interface CardInfo {
  title: string;
  /** The short code chip ("ECONNREFUSED", "DNS"…). */
  code: string;
  hint: string;
  /** A canceled run is not an error. */
  neutral?: boolean;
}

/** The card a transport failure shows (null: not a transport failure). */
export function cardOf(err: EntryError | undefined): CardInfo | null {
  switch (err?.transport) {
    case "connect":
      return { title: transportTitles.connect, code: "ECONNREFUSED", hint: "Nothing is listening there. Start your API, or serve the project's OpenAPI spec with the mock server." };
    case "resolve":
      return { title: transportTitles.resolve, code: "DNS", hint: "The hostname did not resolve. Check the VPN, or base_url in this environment." };
    case "tls":
      return { title: transportTitles.tls, code: "TLS", hint: "Add the CA once for this project instead of turning verification off." };
    case "timeout":
      return { title: transportTitles.timeout, code: "TIMEOUT", hint: "The server accepted the connection but never answered. Raise the timeout or add a retry." };
    case "host-denied":
      return { title: transportTitles["host-denied"], code: "HOST", hint: "This host is outside the hosts allowed for this run." };
    case "canceled":
      return { title: transportTitles.canceled, code: "CANCELED", hint: "The request was stopped before it finished.", neutral: true };
    case "other":
      return { title: "Request failed", code: "ERROR", hint: "The request could not be completed." };
  }
  return null;
}

export interface CookieOrigin {
  name: string;
  value: string;
  /** The earlier request of the run that set it (1-based), or 0. */
  setBy: number;
}

/** The cookies a request sent, with the earlier request that set each. */
export function sentCookies(entries: Record<number, Entry>, index: number): CookieOrigin[] {
  const sent = entries[index]?.calls?.at(-1)?.request.cookies ?? [];
  return sent.map((c) => {
    let setBy = 0;
    for (let i = index - 1; i >= 1 && !setBy; i--) {
      for (const call of entries[i]?.calls ?? []) {
        if (call.response.cookies?.some((r) => r.name === c.name)) setBy = i;
      }
    }
    return { name: c.name, value: c.value, setBy };
  });
}

/** The line prefix of an engine log level, as curl -v writes it. */
export function logPrefix(level: string): string {
  if (level === "request" || level === "requestLine") return ">";
  if (level === "response" || level === "responseLine") return "<";
  return "*";
}

/**
 * The `output:` option of the request at line (1-based) in text: where
 * the run wrote its response body. Read from the request's [Options],
 * up to its response (HTTP …) or the next request; undefined when none.
 */
export function outputOption(text: string, line: number): string | undefined {
  const lines = text.split(/\r?\n/);
  let section = "";
  for (let i = line; i < lines.length; i++) {
    const l = lines[i].trim();
    if (/^HTTP(\/[\d.]+)?\s/.test(l) || /^[A-Z]+\s+\S/.test(l)) return undefined;
    const head = /^\[(\w+)\]$/.exec(l);
    if (head) section = head[1];
    else if (section === "Options") {
      const m = /^output\s*:\s*(.+)$/.exec(l);
      // "-" is the standard output, and a {{variable}} is not known here.
      if (m) return m[1].trim() === "-" || m[1].includes("{{") ? undefined : m[1].trim();
    }
  }
  return undefined;
}
