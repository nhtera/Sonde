// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Shell pieces against the stores, with the bindings faked.

import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RunItem } from "../../lib/run-bridge";

const api = vi.hoisted(() => ({
  Settings: { Get: vi.fn(), Set: vi.fn(async (v: unknown) => v) },
  Runs: { Run: vi.fn(), Cancel: vi.fn() },
}));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

const { registry } = await import("../../app/registry");
const { ShortcutsSheet } = await import("../../app/keymap/shortcuts-sheet");
const { AskHost } = await import("../../components/ask");
const { RunSummary } = await import("../../components/run/run-summary");
const { openable } = await import("../palette/palette");
const { useEnv } = await import("../../state/env");
const { apply } = await import("../../state/run-model");
const { useRuns } = await import("../../state/run");
const { useSettings } = await import("../../state/settings");
const { useTabs } = await import("../../state/tabs");
const { useUI } = await import("../../state/ui");
const { useWorkspace } = await import("../../state/workspace");
const { ResultsHost } = await import("./hosts");
const { closeTab } = await import("./tabs-bar");
const { OverridesChip } = await import("./title-bar");
const checkout = (await import("../../components/run/testdata/checkout.json")).default;

const settings = {
  version: 1,
  appearance: { theme: "dark", uiFontSize: 13, codeFontSize: 13, sideWidth: 248, resultsWidth: 440 },
  shortcuts: {},
  network: { proxy: "", connectTimeout: "", retry: 0 },
  tls: { cacert: "", cert: "", key: "" },
  cookies: { keep: false },
  history: { enabled: true, retention: "30d" },
  contract: { check: false },
};

registry.command({ id: "file.run", title: "Run file", run: () => {} });
registry.command({ id: "palette.open", title: "Search files and commands", hidden: true, run: () => {} });

beforeEach(() => {
  useSettings.setState({ value: structuredClone(settings) });
  api.Settings.Set.mockClear();
});

describe("shortcuts sheet", () => {
  const openSheet = () => {
    useUI.setState({ shortcutsOpen: true });
    render(<ShortcutsSheet />);
    return screen.getByRole("dialog", { name: "Keyboard shortcuts" });
  };

  it("refuses keys another command uses", async () => {
    const sheet = openSheet();
    await userEvent.click(within(sheet).getByRole("button", { name: "Rebind Run file" }));
    await userEvent.keyboard("{Control>}{Meta>}k{/Meta}{/Control}");
    expect(within(sheet).getByRole("alert")).toHaveTextContent("is used by Search files and commands");
    expect(api.Settings.Set).not.toHaveBeenCalled();
  });

  it("refuses a bare key, then saves a combination", async () => {
    const sheet = openSheet();
    await userEvent.click(within(sheet).getByRole("button", { name: "Rebind Run file" }));
    await userEvent.keyboard("{Tab}");
    expect(within(sheet).getByRole("alert")).toHaveTextContent("would be taken from the whole app");
    await userEvent.keyboard("{Alt>}{Shift>}r{/Shift}{/Alt}");
    expect(api.Settings.Set).toHaveBeenCalledWith(expect.objectContaining({ shortcuts: { "file.run": "Alt+Shift+KeyR" } }));
  });
});

describe("overrides chip", () => {
  it("is hidden at 0 and counts the overrides", () => {
    useEnv.setState({ overrides: { count: 0, items: [] } });
    const { container } = render(<OverridesChip />);
    expect(container).toBeEmptyDOMElement();
    act(() => useEnv.setState({ overrides: { count: 1, items: [{ name: "base_url", source: "session", flag: "--variable base_url=…" }] } }));
    expect(screen.getByRole("button", { name: "1 override" })).toBeInTheDocument();
  });
});

describe("results host", () => {
  it("renders a run as the shared summary does", () => {
    let run = {
      runId: "checkout", kind: "run" as const, running: false, source: "", entries: {}, sending: {}, skipped: {},
      logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null,
    } as Parameters<typeof apply>[0];
    for (const e of checkout as { data: { items?: RunItem[]; summary?: never } }[]) {
      if (e.data.items) run = apply(run, e.data.items, 0);
      if (e.data.summary) run = { ...run, summary: e.data.summary };
    }
    const requests = Object.values(run.entries).map((e) => ({
      file: "checkout.hurl", entry: e.index, method: e.calls![0].request.method, url: e.calls![0].request.url, line: e.line,
    }));
    useWorkspace.setState({ index: requests });
    useRuns.setState({ runs: { "checkout.hurl": run } });
    const host = render(<ResultsHost file="checkout.hurl" />).container.innerHTML;
    // The Test run panel shows each file with the same component.
    const slot = render(<div className="results-body"><RunSummary file="checkout.hurl" run={run} requests={requests} /></div>).container.innerHTML;
    expect(host).toBe(slot);
    expect(host).toContain("outcome-failed");
  });
});

describe("closing a tab", () => {
  it("asks in the app before dropping unsaved edits", async () => {
    render(<AskHost />);
    useTabs.setState({ tabs: [{ path: "a.hurl", text: "GET x", savedText: "", hash: "h", version: 2, conflict: false }], active: "a.hurl" });
    let closing = closeTab("a.hurl");
    await userEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    await closing;
    expect(useTabs.getState().tabs).toHaveLength(1);
    closing = closeTab("a.hurl");
    await userEvent.click(await screen.findByRole("button", { name: "Close without saving" }));
    await closing;
    expect(useTabs.getState().tabs).toHaveLength(0);
  });
});

describe("run store", () => {
  it("starts one run of a file at a time and keeps the last run when the file is busy elsewhere", async () => {
    const prev = { ...useRuns.getState().runs["checkout.hurl"] };
    useTabs.setState({ tabs: [{ path: "checkout.hurl", text: "GET x", savedText: "GET x", hash: "h", version: 1, conflict: false }] });
    api.Runs.Run.mockReturnValueOnce(Promise.reject(new Error(JSON.stringify({ code: "busy", message: "checkout.hurl is already running" }))));
    await useRuns.getState().run("checkout.hurl");
    expect(useRuns.getState().runs["checkout.hurl"]).toEqual(prev);
    expect(useUI.getState().toasts.at(-1)).toMatchObject({ kind: "warn", text: "checkout.hurl is already running" });

    api.Runs.Run.mockReturnValueOnce(new Promise(() => {}));
    void useRuns.getState().run("checkout.hurl");
    void useRuns.getState().run("checkout.hurl");
    expect(api.Runs.Run).toHaveBeenCalledTimes(2);
    useRuns.getState().reset();
    expect(useRuns.getState().runs).toEqual({});
  });
});

describe("palette files", () => {
  it("lists every file but secrets files", () => {
    const tree = {
      name: "", path: "", kind: "dir",
      children: [
        { name: "env", path: "env", kind: "dir", children: [{ name: "prod.env", path: "env/prod.env", kind: "secrets" }] },
        { name: "b.hurl", path: "b.hurl", kind: "request" },
        { name: "a.sonde", path: "a.sonde", kind: "request" },
        { name: "x.secrets", path: "x.secrets", kind: "secrets" },
      ],
    };
    expect(openable(tree)).toEqual(["a.sonde", "b.hurl"]);
  });
});
