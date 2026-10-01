// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The import dialog's store, the bindings faked: a new input picks its
// secrets afresh, Apply writes only the accepted files once, and a curl
// command inserted saves its secrets only after the edit lands.

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ImportSuggestion } from "../../lib/api";

const api = vi.hoisted(() => ({
  Imports: { Preview: vi.fn(), Accept: vi.fn(), CurlText: vi.fn(), SaveSecrets: vi.fn(), Write: vi.fn() },
  EditSvc: { Apply: vi.fn() },
}));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

const { useImport } = await import("./state");
const { useTabs } = await import("../../state/tabs");

const change = (index: number) => ({ index, label: `change ${index}`, line: 1, after: "" });
const sugg = (path: string) => ({ path, labels: [], before: "", after: "", changes: [change(0), change(1)] }) as unknown as ImportSuggestion;

beforeEach(() => {
  vi.clearAllMocks();
  api.Imports.Preview.mockResolvedValue({ files: [], warnings: [], skipped: [], candidates: [], counts: {}, project: "" });
  useImport.getState().show("curl");
});

describe("the import store", () => {
  it("a new paste or pick forgets the secrets picked for the last one", async () => {
    useImport.getState().update({ text: "curl -H 'Authorization: Bearer x' https://a" });
    useImport.getState().setLift([1]);
    expect(useImport.getState().req.lift).toEqual([1]);
    useImport.getState().update({ text: "curl https://b" });
    expect(useImport.getState().req.lift).toBeNull();
    useImport.getState().setLift([0]);
    useImport.getState().update({ input: "id" });
    expect(useImport.getState().req.lift).toBeNull();
    // Other options keep the pick.
    useImport.getState().setLift([0]);
    useImport.getState().update({ folder: "x" });
    expect(useImport.getState().req.lift).toEqual([0]);
  });

  it("Apply writes the accepted changes only, and not twice after a failure", async () => {
    useImport.setState({ suggestions: [sugg("a.hurl"), sugg("b.hurl"), sugg("c.hurl")] });
    useImport.getState().decideFile("a.hurl", "accepted");
    useImport.getState().decideFile("b.hurl", "rejected");
    useImport.getState().decide("c.hurl#1", "accepted");
    api.Imports.Accept.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error("disk full"));
    await useImport.getState().apply();
    expect(api.Imports.Accept.mock.calls.map((c) => [c[1], c[2]])).toEqual([
      ["a.hurl", [0, 1]],
      ["c.hurl", [1]],
    ]);
    expect(useImport.getState().applied).toEqual(["a.hurl"]);
    api.Imports.Accept.mockResolvedValue(undefined);
    await useImport.getState().apply();
    // The retry: c only (a is written already).
    expect(api.Imports.Accept.mock.calls.map((c) => c[1])).toEqual(["a.hurl", "c.hurl", "c.hurl"]);
  });

  it("Import waits while a preview is on its way: what is written is what was shown", async () => {
    useImport.setState({ preview: { files: [] } as never, pending: true });
    await useImport.getState().write();
    expect(api.Imports.Write).not.toHaveBeenCalled();
    // Typing marks it at once, before the preview is asked for.
    useImport.setState({ pending: false });
    useImport.getState().update({ text: "curl https://c" });
    expect(useImport.getState().pending).toBe(true);
  });

  it("Insert saves the secrets after the edit lands, never when it is refused", async () => {
    useTabs.setState({ tabs: [{ path: "a.hurl", text: "GET https://a\n", savedText: "", hash: "", version: 4, conflict: false }], active: "a.hurl" });
    api.Imports.CurlText.mockResolvedValue("GET https://b\n");
    api.EditSvc.Apply.mockResolvedValue({ version: 3, edits: [], text: "x" }); // an older version: refused
    await useImport.getState().insert("a.hurl");
    expect(api.Imports.SaveSecrets).not.toHaveBeenCalled();
    expect(api.Imports.CurlText).toHaveBeenCalledWith(expect.objectContaining({ target: "a.hurl", targetText: "GET https://a\n" }));
  });
});
