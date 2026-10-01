// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The run events the Go side sends (desktop/internal/view), already
// redacted. Entries use the CLI's JSON report shape.

import type * as report from "@bindings/internal/report";

export type ReportEntry = report.Entry;
export type ReportRequest = report.Request;
export type StreamMessage = report.StreamMessage;

export interface Body {
  id: string;
  size: number;
  contentType: string;
  error?: string;
}

export interface EntryError {
  line: number;
  column: number;
  kind: string;
  assert: boolean;
  description: string;
  message: string;
  /** connect, resolve, timeout, tls, host-denied, canceled, other */
  transport?: string;
}

/** A call's phases, in microseconds since it began. */
export interface Timings {
  name_lookup: number;
  connect: number;
  app_connect: number;
  pre_transfer: number;
  start_transfer: number;
  total: number;
}

export interface Entry extends ReportEntry {
  bodies: Body[];
  /** One per call (older recordings have none: use the report's). */
  timings?: Timings[];
  errors: EntryError[];
  success: boolean;
  retried: boolean;
}

export type RunEvent =
  | { type: "log"; entry: number; level: string; text: string }
  | { type: "entryStarted"; entry: number; retry: number; last: number }
  | { type: "requestSent"; entry: number; call: number; request: ReportRequest }
  | { type: "entrySkipped"; entry: number; reason: string }
  | { type: "message"; entry: number; message: StreamMessage }
  | { type: "entryFinished"; entry: Entry }
  | { type: "unitStarted"; file: string; row?: number; label?: string };
