// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

/** Request header names offered as completions, with what they are for. */
export const headerNames = [
  { name: "Accept", hint: "Media types the client can handle" },
  { name: "Accept-Charset", hint: "Preferred character sets" },
  { name: "Accept-Encoding", hint: "gzip, br · or use compressed in Options" },
  { name: "Accept-Language", hint: "Preferred languages" },
  { name: "Authorization", hint: "Credentials · or use the Auth tab" },
  { name: "Cache-Control", hint: "Caching directives" },
  { name: "Content-Type", hint: "The body's media type" },
  { name: "Cookie", hint: "Cookies · or use [Cookies]" },
  { name: "If-Match", hint: "Only if the ETag matches" },
  { name: "If-Modified-Since", hint: "Only if changed since a date" },
  { name: "If-None-Match", hint: "Only if the ETag differs" },
  { name: "Idempotency-Key", hint: "Makes a retried request safe" },
  { name: "Origin", hint: "Where the request comes from" },
  { name: "Prefer", hint: "Server preferences (return=minimal…)" },
  { name: "Range", hint: "Part of the resource" },
  { name: "Referer", hint: "The page that linked here" },
  { name: "User-Agent", hint: "The client · Sonde sends sonde/<version>" },
  { name: "X-API-Key", hint: "An API key · or use the Auth tab" },
  { name: "X-Correlation-ID", hint: "Traces a request across services" },
  { name: "X-Request-ID", hint: "Identifies the request" },
];
