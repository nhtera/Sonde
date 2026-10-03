// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { StringStream } from "@codemirror/language";
import { describe, expect, it } from "vitest";
import { yamlToken } from "./yaml";

/** The tokens of each line, as [text, style] pairs (spaces left out). */
function tokens(text: string): [string, string | null][] {
  const state = { value: false };
  const out: [string, string | null][] = [];
  for (const line of text.split("\n")) {
    const s = new StringStream(line, 2, 2);
    while (!s.eol()) {
      const style = yamlToken(s, state);
      const t = s.current();
      if (t.trim()) out.push([t, style]);
      s.start = s.pos;
    }
  }
  return out;
}

describe("yaml highlighting", () => {
  it("colors keys, scalars, dashes and comments", () => {
    expect(
      tokens(
        [
          "# shop-api",
          "version: 1",
          "environments:",
          "  local:",
          "    variables:",
          "      base_url: http://127.0.0.1:34120 # the fixture",
          '      name: "ada"',
          "      on: true",
          "      none: ~",
          "    secrets_files:",
          "      - secrets/local.secrets",
        ].join("\n"),
      ),
    ).toEqual([
      ["# shop-api", "lineComment"],
      ["version", "propertyName"],
      [":", null],
      ["1", "number"],
      ["environments", "propertyName"],
      [":", null],
      ["local", "propertyName"],
      [":", null],
      ["variables", "propertyName"],
      [":", null],
      ["base_url", "propertyName"],
      [":", null],
      ["http://127.0.0.1:34120", "string"],
      ["# the fixture", "lineComment"],
      ["name", "propertyName"],
      [":", null],
      ['"ada"', "string"],
      ["on", "propertyName"],
      [":", null],
      ["true", "bool"],
      ["none", "propertyName"],
      [":", null],
      ["~", "null"],
      ["secrets_files", "propertyName"],
      [":", null],
      ["-", "modifier"],
      ["secrets/local.secrets", "string"],
    ]);
  });
});
