// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What Copy as offers for a file: curl for the request at the cursor or
// the whole file, and the sonde commands that run it as the app does.

import type { FileRun } from "../../state/run-model";

/** A copy request, as copyas takes it. */
export interface CopyRequest {
  file: string;
  source: string;
  env: string;
  entry: number;
  kind: string;
  files: string[];
  shell: string;
  clock: string;
  /** A test run's options (unused for a file's command). */
  jobs: number;
  continueOnError: boolean;
}

export interface CopyItem {
  id: string;
  title: string;
  /** "curl" or "sonde": the service called. */
  tool: "curl" | "sonde";
  req: CopyRequest;
  /** Under the title, before the preview comes. */
  sub?: string;
}

/** The shell the commands are quoted for. */
export function shellOf(platform: string): "posix" | "powershell" {
  return /Win/.test(platform) ? "powershell" : "posix";
}

/** HH:MM of an RFC 3339 time, in the page's zone. */
export function clockOf(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/** The items for file (its text source), entry the request at the cursor
 * (0: none), run the file's last run. */
export function copyItems(o: { file: string; source: string; env: string; entry: number; run?: FileRun; shell: string }): CopyItem[] {
  const base: CopyRequest = { file: o.file, source: o.source, env: o.env, entry: 0, kind: "", files: [], shell: o.shell, clock: "", jobs: 0, continueOnError: false };
  const items: CopyItem[] = [];
  if (o.entry > 0) items.push({ id: "curl.request", title: "curl · this request", tool: "curl", req: { ...base, entry: o.entry } });
  items.push({ id: "curl.file", title: "curl · whole file", tool: "curl", req: base, sub: "every request, captures filled from the last run" });
  items.push({ id: "sonde.file", title: "sonde · run this file", tool: "sonde", req: { ...base, kind: "run" } });
  if (o.entry > 0) items.push({ id: "sonde.to", title: "sonde · run to this request", tool: "sonde", req: { ...base, kind: "send", entry: o.entry } });
  // After a Send: the command that runs up to the request sent.
  const sent = o.run?.kind === "send" ? o.run.sent : undefined;
  if (sent) {
    const at = o.run?.summary?.baseRunAt;
    items.push({ id: "sonde.send", title: `sonde · the Send of request ${sent}`, tool: "sonde", req: { ...base, kind: "send", entry: sent, clock: at ? clockOf(at) : "" } });
  }
  items.push({ id: "sonde.test", title: "sonde · test run", tool: "sonde", req: { ...base, kind: "test", files: [o.file] } });
  return items;
}
