// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The form's edit path against a real tabs store, Go faked: what is sent,
// and when it is refused.

import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  EditSvc: { Apply: vi.fn(), Batch: vi.fn(), Model: vi.fn(), Checks: vi.fn() },
}));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

const { useTabs } = await import("../../state/tabs");
const { useForm } = await import("./model");
const { formEdit, setRow, flushForm } = await import("./edit");

const text = 'GET https://h\nHTTP 200\n[Asserts]\njsonpath "$.a" == 1\n';

beforeEach(() => {
  vi.clearAllMocks();
  useTabs.setState({ tabs: [{ path: "a.hurl", text, savedText: text, hash: "h", version: 3, conflict: false }], active: "a.hurl" });
  useForm.setState({ models: { "a.hurl": { version: 3, entries: [], checks: {} } } });
  api.EditSvc.Model.mockResolvedValue([]);
  api.EditSvc.Checks.mockResolvedValue([]);
});

describe("form edits", () => {
  it("sets an assert as text; never removes it for its empty key", async () => {
    const next = text.replace("== 1", "== 2");
    api.EditSvc.Apply.mockResolvedValue({ version: 3, edits: [], text: next });
    expect(await setRow("a.hurl", 1, "asserts", 0, "", 'jsonpath "$.a" == 2')).toBe(true);
    expect(api.EditSvc.Apply).toHaveBeenCalledWith(
      { file: "a.hurl", text, version: 3 },
      { kind: "setRow", entry: 1, section: "asserts", index: 0, key: "", value: 'jsonpath "$.a" == 2' },
    );
    expect(useTabs.getState().tabs[0].text).toBe(next);
    // A keyed row whose key is cleared is removed.
    api.EditSvc.Apply.mockClear();
    useForm.setState({ models: { "a.hurl": { version: 4, entries: [], checks: {} } } });
    await setRow("a.hurl", 1, "headers", 2, "", "x");
    expect(api.EditSvc.Apply.mock.calls[0][1]).toMatchObject({ kind: "removeRow", section: "headers", index: 2 });
  });

  it("refuses a row edit built from a model older than the text", async () => {
    useForm.setState({ models: { "a.hurl": { version: 2, entries: [], checks: {} } } });
    expect(await formEdit("a.hurl", { kind: "removeRow", entry: 1, section: "headers", index: 0 })).toBe(false);
    expect(api.EditSvc.Apply).not.toHaveBeenCalled();
    // An edit that names no row applies to the text as it is.
    api.EditSvc.Apply.mockResolvedValue({ version: 3, edits: [], text: text.replace("https://h", "https://i") });
    expect(await formEdit("a.hurl", { kind: "setURL", entry: 1, value: "https://i" })).toBe(true);
  });

  it("makes a batch of several ops, and runs wait for it", async () => {
    let done!: (v: unknown) => void;
    api.EditSvc.Batch.mockReturnValue(new Promise((r) => (done = r)));
    const p = formEdit("a.hurl", { kind: "removeSection", entry: 1, section: "form" }, { kind: "setBody", entry: 1, value: "{}" });
    const flushed = flushForm("a.hurl");
    expect(flushed).not.toBeNull();
    let over = false;
    void flushed!.then(() => (over = true));
    await Promise.resolve();
    expect(over).toBe(false);
    done({ version: 3, edits: [], text: text + "{}\n" });
    await p;
    await flushed;
    expect(over).toBe(true);
    expect(flushForm("a.hurl")).toBeNull();
  });
});
