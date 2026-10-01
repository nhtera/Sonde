// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Results panel on a run recorded from shop-api (checkout.hurl: the
// last assert fails), with the bindings faked.

import { act, render, screen, within } from "@testing-library/react";
import { readFileSync } from "node:fs";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RunItem } from "../../lib/run-bridge";
import type { Entry } from "../../lib/view";

const api = vi.hoisted(() => ({
  Runs: { Run: vi.fn(), Send: vi.fn(), Cancel: vi.fn() },
  Mocks: { Spec: vi.fn(), Status: vi.fn(), Start: vi.fn() },
  EditSvc: { Model: vi.fn(), Apply: vi.fn(), AssertValue: vi.fn() },
}));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));
// The JSON tree's worker: never answers here (the browser tests cover it).
vi.stubGlobal("Worker", class { postMessage() {} });

const { apply } = await import("../../state/run-model");
const { useRuns } = await import("../../state/run");
const { useTabs } = await import("../../state/tabs");
const { useUI } = await import("../../state/ui");
const { useWorkspace } = await import("../../state/workspace");
const { ResultsPanel } = await import("./results-panel");
const { useResults } = await import("./state");
const { setMaxTime } = await import("./actions");
const checkout = (await import("../../components/run/testdata/checkout.json")).default;
// Tests run from desktop/frontend.
const source = readFileSync("../testdata/shop-api/checkout.hurl", "utf8");

type FileRun = ReturnType<typeof useRuns.getState>["runs"][string];

/** The recorded checkout run, finished. */
function recorded(over: Partial<FileRun> = {}): FileRun {
  let r: FileRun = {
    runId: "r1", kind: "run", running: true, source, entries: {}, sending: {}, skipped: {},
    logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null,
  };
  for (const e of checkout as { data: { items?: RunItem[]; summary?: FileRun["summary"] } }[]) {
    if (e.data.items) r = apply(r, e.data.items, 0);
    if (e.data.summary) r = { ...r, running: false, summary: e.data.summary };
  }
  return { ...r, ...over };
}

function show(run: FileRun) {
  const requests = Object.values(run.entries).map((e) => ({
    file: "checkout.hurl", entry: e.index, method: e.calls?.[0]?.request.method ?? "GET", url: e.calls?.[0]?.request.url ?? "{{base_url}}/health", line: e.line,
  }));
  useWorkspace.setState({ index: requests });
  useTabs.setState({ tabs: [{ path: "checkout.hurl", text: source, savedText: source, hash: "h", version: 1, conflict: false }], active: "checkout.hurl" });
  useRuns.setState({ runs: { "checkout.hurl": run } });
  return render(<ResultsPanel file="checkout.hurl" />);
}

const row = (n: number) => screen.getAllByRole("listitem").find((li) => li.querySelector(".idx")?.textContent === String(n))!;
const tab = (name: RegExp) => screen.getByRole("tab", { name });

beforeEach(() => {
  vi.clearAllMocks();
  useResults.setState({ picked: {}, tab: null, session: null, activeBody: null, bodyMode: "pretty" });
  useUI.setState({ toasts: [] });
});

describe("Results panel", () => {
  it("opens on the first failure, explained", () => {
    show(recorded());
    expect(row(5)).toHaveAttribute("aria-selected", "true");
    expect(tab(/Asserts/)).toHaveAttribute("aria-selected", "true");
    expect(tab(/Asserts/)).toHaveTextContent("1/2");
    const box = screen.getByRole("alert");
    expect(box).toHaveTextContent("Assert failed");
    expect(box).toHaveTextContent(`jsonpath "$.status" == "paid"`);
    expect(within(box).getByText(`"pending"`)).toBeInTheDocument();
    expect(screen.getByText(/200 OK/)).toBeInTheDocument();
    expect(screen.getByText(/Ran requests 1–5 · full file/)).toBeInTheDocument();
  });

  it("shows another request's captures, cookies and timing", async () => {
    const user = userEvent.setup();
    show(recorded());
    await user.click(row(1));
    await user.click(tab(/Captures/));
    const caps = screen.getByRole("tabpanel");
    expect(caps).toHaveTextContent("→ token***");
    expect(caps).toHaveTextContent("→ user_id7");
    await user.click(tab(/Cookies/));
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Received · Set-Cookie 1");
    await user.click(row(2));
    expect(screen.getByRole("tabpanel")).toHaveTextContent("set by request 1");
    await user.click(tab(/Timeline/));
    expect(screen.getByLabelText("Timing")).toHaveTextContent("Total");
    expect(screen.getByLabelText("Log")).toHaveTextContent("> Authorization: Bearer ***");
  });

  it("dims the requests a Send reused", () => {
    show(recorded({ kind: "send", sent: 5 }));
    expect(row(1)).toHaveClass("dim");
    expect(row(5)).not.toHaveClass("dim");
    expect(row(5)).toHaveTextContent("just sent");
  });

  it("offers the mock for a refused connection, then runs again", async () => {
    const user = userEvent.setup();
    api.Mocks.Spec.mockResolvedValue({ file: "openapi.yaml", operations: [] });
    api.Mocks.Status.mockResolvedValue({ running: false, url: "" });
    api.Mocks.Start.mockResolvedValue({ running: true, url: "http://127.0.0.1:4010" });
    api.Runs.Run.mockResolvedValue(null);
    const refused = {
      index: 1, line: 2, time: 3, calls: [], asserts: [], captures: [], curl_cmd: "", bodies: [], success: false, retried: false,
      errors: [{ line: 2, column: 5, kind: "http", assert: false, description: "HTTP connection", message: "connection refused", transport: "connect" }],
    } as unknown as Entry;
    show(recorded({ entries: { 1: refused } }));
    expect(tab(/Error/)).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Connection refused")).toBeInTheDocument();
    expect(screen.getByText("No response")).toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Start mock on :4010" }));
    expect(api.Mocks.Start).toHaveBeenCalledWith(4010);
    await act(async () => {});
    expect(api.Runs.Run).toHaveBeenCalled();
  });
});

/** A data run's summary: a unit per row. */
const dataSummary = (rows: [number, boolean][]) => ({ ...recorded().summary!, kind: "data", units: rows.map(([row, success]) => ({ file: "checkout.hurl", row, success, interrupted: false, canceled: false, requests: 1, durationMs: 1 })) });

describe("Results panel: data-driven runs", () => {
  it("shows row pills with passed/failed states", () => {
    const run: FileRun = {
      ...recorded(),
      kind: "data",
      summary: dataSummary([[1, true], [2, false]]),
      data: {
        file: "data/logins.csv",
        rows: [
          { row: 1, label: "alice", run: recorded({ entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never } }) },
          { row: 2, label: "bob", run: recorded({ entries: { 1: { index: 1, line: 1, success: false, errors: [{ kind: "assert" } as never] } as never } }) },
        ],
        row: 2,
        picked: false,
      },
    };
    show(run);

    const pills = screen.getAllByRole("tab", { name: /^Row \d+/ });
    expect(pills).toHaveLength(2);
    expect(pills[0]).toHaveTextContent("alice");
    expect(pills[0]).toHaveClass("state-passed");
    expect(pills[1]).toHaveTextContent("bob");
    expect(pills[1]).toHaveClass("state-failed");
  });

  it("displays N rows lead text and file name", () => {
    const run: FileRun = {
      ...recorded(),
      kind: "data",
      summary: dataSummary([[1, true], [2, true]]),
      data: {
        file: "data/logins.csv",
        rows: [
          { row: 1, run: recorded({ entries: {} }) },
          { row: 2, run: recorded({ entries: {} }) },
        ],
        row: 1,
        picked: false,
      },
    };
    show(run);

    expect(screen.getByText("2 rows")).toBeInTheDocument();
    expect(screen.getByText(/Ran all 2 rows of logins.csv/)).toBeInTheDocument();
  });

  it("clicking a pill shows that row's requests", async () => {
    const user = userEvent.setup();
    const run: FileRun = {
      ...recorded(),
      kind: "data",
      summary: dataSummary([[1, true], [2, false]]),
      data: {
        file: "data/logins.csv",
        rows: [
          { row: 1, label: "alice", run: recorded({ entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never } }) },
          { row: 2, label: "bob", run: recorded({ entries: { 1: { index: 1, line: 1, success: false, errors: [{ kind: "assert" } as never] } as never } }) },
        ],
        row: 1,
        picked: false,
      },
    };
    show(run);

    const pills = screen.getAllByRole("tab", { name: /^Row \d+/ });
    await user.click(pills[1]);

    // Row 2 shows, picked: its failed request is the one listed.
    const shown = useRuns.getState().runs["checkout.hurl"];
    expect(shown.data?.row).toBe(2);
    expect(shown.data?.picked).toBe(true);
    expect(shown.entries[1]?.success).toBe(false);
    expect(screen.getAllByRole("tab", { name: /^Row \d+/ })[1]).toHaveAttribute("aria-selected", "true");
  });

  it("shows running state for incomplete rows", () => {
    const run: FileRun = {
      ...recorded(),
      kind: "data",
      running: true,
      data: {
        file: "data/logins.csv",
        rows: [
          { row: 1, run: recorded({ running: false, entries: { 1: { index: 1, line: 1, success: true, errors: [] } as never } }) },
          { row: 2, run: recorded({ running: true, entries: {} }) },
        ],
        row: 1,
        picked: false,
      },
    };
    show(run);

    const pills = screen.getAllByRole("tab", { name: /^Row \d+/ });
    expect(pills[1]).toHaveClass("state-running");
  });
});

describe("Results actions", () => {
  it("changes max-time when the request has it, adds it otherwise", async () => {
    useTabs.setState({ tabs: [{ path: "a.hurl", text: "GET x\n", savedText: "", hash: "", version: 4, conflict: false }] });
    const result = { version: 4, edits: [{ Range: { Start: 6, End: 6 }, NewText: "[Options]\nmax-time: 30s\n" }], text: "GET x\n[Options]\nmax-time: 30s\n" };
    api.EditSvc.Apply.mockResolvedValue(result);
    api.EditSvc.Model.mockResolvedValue([{ Index: 1, Rows: { options: [{ Key: "retry", Value: "1" }, { Key: "max-time", Value: "10s" }] } }]);
    await setMaxTime("a.hurl", 1, "30s");
    expect(api.EditSvc.Apply).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({ kind: "setRow", index: 1, value: "30s" }));
    expect(useTabs.getState().tabs[0].text).toBe(result.text);
    expect(useUI.getState().toasts.at(-1)?.text).toBe("max-time: 30s set at line 2");

    useTabs.setState({ tabs: [{ path: "a.hurl", text: "GET x\n", savedText: "", hash: "", version: 4, conflict: false }] });
    api.EditSvc.Model.mockResolvedValue([{ Index: 1, Rows: {} }]);
    await setMaxTime("a.hurl", 1, "30s");
    expect(api.EditSvc.Apply).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({ kind: "addRow", key: "max-time" }));
  });
});
