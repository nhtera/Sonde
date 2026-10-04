// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { registry } from "../registry";
import { bindKeys, conflicts, effectiveKeys, isBindable, label, normalize } from "./keymap-manager";

describe("keymap", () => {
  registry.command({ id: "file.run", title: "Run file", run: () => {} });
  registry.command({ id: "file.save", title: "Save", run: () => {} });

  it("uses defaults, then the user's keys", () => {
    expect(effectiveKeys(undefined)["file.run"]).toBe("$mod+KeyR");
    expect(effectiveKeys({ "file.run": "$mod+Shift+KeyR" })["file.run"]).toBe("$mod+Shift+KeyR");
  });

  it("names punctuation keys by their character", () => {
    expect(label("$mod+Period")).toMatch(/\.$/);
    expect(label("$mod+Comma")).toMatch(/,$/);
  });

  it("reports conflicts whatever the modifier order", () => {
    const keys = effectiveKeys({ "file.run": "Shift+$mod+KeyS", "file.save": "$mod+Shift+KeyS" });
    expect(Object.values(conflicts(keys))).toEqual([["file.run", "file.save"]]);
    expect(normalize("Shift+$mod+KeyS")).toBe(normalize("$mod+Shift+KeyS"));
  });

  it("binds only combinations that leave the app's keys alone", () => {
    expect(isBindable("$mod+KeyR")).toBe(true);
    expect(isBindable("Alt+Shift+KeyR")).toBe(true);
    expect(isBindable("F5")).toBe(true);
    expect(isBindable("Shift+Slash")).toBe(true);
    for (const k of ["Tab", "Enter", "Space", "ArrowDown", "KeyR", "Shift+KeyR", "F13"]) expect(isBindable(k), k).toBe(false);
  });

  it("keeps a bound key from the browser even when its command cannot run", () => {
    const run = vi.fn();
    registry.command({ id: "test.never", title: "Never", when: () => false, run });
    const off = bindKeys(window, { "test.never": "Alt+KeyQ" });
    const e = new KeyboardEvent("keydown", { key: "q", code: "KeyQ", altKey: true, cancelable: true });
    window.dispatchEvent(e);
    expect(e.defaultPrevented).toBe(true);
    expect(run).not.toHaveBeenCalled();
    off();
  });

  it("runs only the palette key while a dialog is open", () => {
    const run = vi.fn();
    const palette = vi.fn();
    registry.command({ id: "test.run", title: "Run", run });
    registry.command({ id: "palette.open", title: "Palette", run: palette });
    const off = bindKeys(window, { "test.run": "Alt+KeyW", "palette.open": "Alt+KeyK" });
    const dialog = document.body.appendChild(document.createElement("div"));
    dialog.setAttribute("role", "dialog");
    dialog.dataset.state = "open";
    const press = (code: string) =>
      window.dispatchEvent(new KeyboardEvent("keydown", { key: code.slice(3).toLowerCase(), code, altKey: true, cancelable: true }));
    press("KeyW");
    press("KeyK");
    expect(run).not.toHaveBeenCalled();
    expect(palette).toHaveBeenCalledOnce();
    dialog.remove();
    press("KeyW");
    expect(run).toHaveBeenCalledOnce();
    off();
  });

  it("runs shortcuts with a modifier from inside the editor", () => {
    const run = vi.fn();
    registry.command({ id: "test.editor", title: "Editor", run });
    const off = bindKeys(window, { "test.editor": "Alt+KeyE" });
    const editor = document.body.appendChild(document.createElement("div"));
    editor.contentEditable = "true";
    editor.dispatchEvent(new KeyboardEvent("keydown", { key: "e", code: "KeyE", altKey: true, bubbles: true, cancelable: true }));
    expect(run).toHaveBeenCalledOnce();
    editor.remove();
    off();
  });
});
