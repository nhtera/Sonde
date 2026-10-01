// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// copyItem: the plain copy writes the text Go renders; the copy with
// secret values asks first, then lets Go write the clipboard.

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CopyItem } from "./model";

const api = vi.hoisted(() => ({
  CopyAs: { Curl: vi.fn(), Sonde: vi.fn() },
  CopyAsReveal: { Curl: vi.fn(), Sonde: vi.fn() },
}));
const ask = vi.hoisted(() => ({ confirm: vi.fn() }));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));
vi.mock("../../components/ask", () => ask);

const { copyItem } = await import("./copy-as-menu");
const { useUI } = await import("../../state/ui");

const item = (tool: "curl" | "sonde"): CopyItem => ({
  id: tool,
  title: tool,
  tool,
  req: { file: "a.hurl", source: "GET https://a", env: "local", entry: 1, kind: "", files: [], shell: "posix", clock: "", jobs: 0, continueOnError: false },
});

const writeText = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  writeText.mockResolvedValue(undefined);
});

describe("copyItem", () => {
  it("copies the text Go renders, and tells its note", async () => {
    api.CopyAs.Sonde.mockResolvedValue({ text: "sonde run a.hurl", note: "Run it in /p." });
    const toast = vi.spyOn(useUI.getState(), "toast");
    await copyItem(item("sonde"));
    expect(api.CopyAs.Sonde).toHaveBeenCalledWith(item("sonde").req);
    expect(writeText).toHaveBeenCalledWith("sonde run a.hurl");
    expect(toast).toHaveBeenCalledWith({ kind: "success", text: "Copied. Run it in /p." });
    expect(ask.confirm).not.toHaveBeenCalled();
  });

  it("with secret values: asks, then Go writes the clipboard (the page never)", async () => {
    ask.confirm.mockResolvedValue(true);
    api.CopyAsReveal.Curl.mockResolvedValue("");
    await copyItem(item("curl"), true);
    expect(ask.confirm).toHaveBeenCalledOnce();
    expect(api.CopyAsReveal.Curl).toHaveBeenCalledWith(item("curl").req);
    expect(api.CopyAs.Curl).not.toHaveBeenCalled();
    expect(writeText).not.toHaveBeenCalled();
  });

  it("declined: nothing is copied", async () => {
    ask.confirm.mockResolvedValue(false);
    await copyItem(item("curl"), true);
    expect(api.CopyAsReveal.Curl).not.toHaveBeenCalled();
    expect(writeText).not.toHaveBeenCalled();
  });
});
