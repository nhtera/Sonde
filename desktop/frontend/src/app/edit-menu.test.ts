// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { EditorView } from "@codemirror/view";
import { history } from "@codemirror/commands";
import { afterEach, describe, expect, it, vi } from "vitest";
import { editCommand } from "./edit-menu";

afterEach(() => {
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

describe("editCommand", () => {
  it("undoes and redoes in the focused code editor", () => {
    const view = new EditorView({ doc: "a", extensions: [history()], parent: document.body });
    view.dispatch({ changes: { from: 1, insert: "b" } });
    view.contentDOM.focus();
    editCommand("undo");
    expect(view.state.doc.toString()).toBe("a");
    editCommand("redo");
    expect(view.state.doc.toString()).toBe("ab");
    view.destroy();
  });

  it("leaves a form field to the webview's own history", () => {
    const exec = vi.fn(() => true);
    Object.defineProperty(document, "execCommand", { value: exec, configurable: true });
    const input = document.body.appendChild(document.createElement("input"));
    input.focus();
    editCommand("undo");
    editCommand("bogus");
    expect(exec.mock.calls).toEqual([["undo"]]);
  });
});
