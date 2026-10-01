// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A data-driven run in the Test run: the file runs through RunData with
// the text on disk, and the table shows a pill per row.

import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Summary } from "../../../lib/api";

const api = vi.hoisted(() => ({
  Runs: { RunTest: vi.fn(), RunData: vi.fn(), Cancel: vi.fn() },
  Workspace: { Read: vi.fn() },
}));
vi.mock("../../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

const { useTestRun } = await import("./state");
const { TestRunMain } = await import("./test-run-main");

const summary = (over: Partial<Summary> = {}): Summary => ({
  runId: "r1", kind: "data", env: "local", outcome: "failed", startedAt: "", files: 1, succeeded: 0, requests: 4, durationMs: 30,
  units: [
    { file: "login.hurl", row: 1, success: true, interrupted: false, canceled: false, requests: 2, durationMs: 10 },
    { file: "login.hurl", row: 2, success: false, interrupted: false, canceled: false, requests: 2, durationMs: 20 },
  ],
  ...over,
});

beforeEach(() => {
  vi.clearAllMocks();
  useTestRun.setState({ running: false, summary: null, files: {}, runId: null });
});

describe("a data-driven run", () => {
  it("runs the one file with its text on disk and the picked data file", async () => {
    api.Workspace.Read.mockResolvedValue({ path: "login.hurl", text: "GET {{base_url}}/login\n", hash: "h" });
    api.Runs.RunData.mockResolvedValue(summary());
    await useTestRun.getState().start(["login.hurl"], "data-handle");
    expect(api.Runs.RunTest).not.toHaveBeenCalled();
    expect(api.Runs.RunData).toHaveBeenCalledWith(
      expect.objectContaining({ file: "login.hurl", source: "GET {{base_url}}/login\n", dataHandle: "data-handle", rows: [] }),
    );
    expect(useTestRun.getState().summary?.units).toHaveLength(2);
  });

  it("needs exactly one file", async () => {
    await useTestRun.getState().start(["a.hurl", "b.hurl"], "data-handle");
    expect(api.Runs.RunData).not.toHaveBeenCalled();
  });

  it("shows a pill per row, and the file's requests and time summed", () => {
    useTestRun.setState({ summary: summary(), runId: "r1" });
    render(<TestRunMain />);
    const row = screen.getByRole("table", { name: "Files" }).querySelector("tbody tr") as HTMLElement;
    const pills = within(row).getAllByRole("tab");
    expect(pills.map((p) => p.textContent)).toEqual(["Row 1", "Row 2"]);
    expect(pills.map((p) => p.className)).toEqual(["row-pill state-passed", "row-pill state-failed"]);
    expect(row).toHaveTextContent("30 ms");
    expect(within(row).getAllByRole("cell")[2]).toHaveTextContent("4");
  });

  it("shows no pills for a test run", () => {
    useTestRun.setState({ summary: summary({ kind: "test", units: [{ file: "a.hurl", success: true, interrupted: false, canceled: false, requests: 1, durationMs: 5 }] }), runId: "r1" });
    render(<TestRunMain />);
    expect(screen.queryByRole("tablist", { name: "Data rows" })).toBeNull();
  });
});

describe("a file's outcome", () => {
  it("is failed for a unit stopped before any entry ended", () => {
    useTestRun.setState({
      summary: summary({ kind: "test", units: [{ file: "a.hurl", success: false, interrupted: true, canceled: true, requests: 0, durationMs: 1 }] }),
      files: { "a.hurl": { runId: "r1", kind: "run", running: false, source: "", entries: {}, sending: {}, skipped: {}, logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null } },
      runId: "r1",
    });
    render(<TestRunMain />);
    const row = screen.getByRole("table", { name: "Files" }).querySelector("tbody tr") as HTMLElement;
    expect(row).toHaveTextContent("Failed");
  });
});
