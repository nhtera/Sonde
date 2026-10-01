// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { EditorState } from "@codemirror/state";
import { describe, expect, it } from "vitest";
import { shorten, typingTemplate } from "./decorations";

describe("shorten", () => {
  it("keeps a short warning whole", () => {
    expect(shorten("Parsing template variable: expecting a variable")).toBe("Parsing template variable: expecting a variable");
  });

  it("keeps a long warning's headline", () => {
    expect(shorten('undefined variable "trace_id": not captured earlier and not defined in environment "local"')).toBe('undefined variable "trace_id"');
  });

  it("cuts a long warning with no headline", () => {
    const s = shorten("x".repeat(80));
    expect(s).toHaveLength(60);
    expect(s.endsWith("…")).toBe(true);
  });
});

describe("typingTemplate", () => {
  const at = (text: string) => {
    const pos = text.indexOf("|");
    return typingTemplate(EditorState.create({ doc: text.replace("|", ""), selection: { anchor: pos } }));
  };

  it("is the cursor's line while a {{ is open there", () => {
    expect(at("GET x\nX-User: {{|")).toBe(2);
    expect(at("X-User: {{|}}")).toBe(1);
    expect(at("X-User: {{ us|}}")).toBe(1);
  });

  it("is null once the template is closed or the cursor is elsewhere", () => {
    expect(at("X-User: {{user}}|")).toBeNull();
    expect(at("X-User: |{{")).toBeNull();
    expect(at("X-User: {{user}} {{a}}|")).toBeNull();
  });
});
