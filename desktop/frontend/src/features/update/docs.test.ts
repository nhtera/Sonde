// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

// The update texts Settings shows are the docs' words, so a change to one
// side fails until the other matches (docs/desktop.md › Updates).

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { privacyLine, updateCheckNote } from "./texts";

const docs = readFileSync(new URL("../../../../../docs/desktop.md", import.meta.url), "utf8");

describe("update texts", () => {
  it("are quoted in docs/desktop.md, word for word", () => {
    expect(docs).toContain(`> ${updateCheckNote}\n`);
    expect(docs).toContain(`> ${privacyLine(true)}\n`);
  });
});
