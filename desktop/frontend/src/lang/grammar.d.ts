// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Grammar files build into parsers on import (@lezer/generator's plugin in
// vite.config.ts).

declare module "*.grammar" {
  import type { LRParser } from "@lezer/lr";
  export const parser: LRParser;
}

declare module "*/sonde.grammar.terms" {
  export const Method: number;
  export const UrlText: number;
  export const UrlTemplate: number;
  export const HttpVersion: number;
  export const SectionHeader: number;
  export const Key: number;
  export const JsonBody: number;
  export const XmlBody: number;
  export const MultilineString: number;
  export const LiteralBody: number;
  export const Regex: number;
  export const stringStart: number;
  export const stringContent: number;
  export const stringEnd: number;
  export const backtickStart: number;
  export const backtickContent: number;
  export const backtickEnd: number;
  export const Escape: number;
  export const templateOpen: number;
  export const eofEnd: number;
}
