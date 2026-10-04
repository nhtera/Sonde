// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The run components fed by events recorded from shop-api runs
// (desktop/record_test.go).

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Request, Summary } from "../../lib/api";
import type { RunItem } from "../../lib/run-bridge";
import type { Entry } from "../../lib/view";
import { apply, type FileRun } from "../../state/run-model";
import checkout from "./testdata/checkout.json";
import dataLogin from "./testdata/data-login.json";
import { counts } from "./counts";
import { DataRowPills } from "./data-row-pills";
import { FailureBox } from "./failure-box";
import { failureOf, requestRows } from "./model";
import { RequestList } from "./request-list";
import { ResultsHeader } from "./results-header";

interface Recorded {
  topic: string;
  data: { items?: RunItem[]; dropped?: number; summary?: Summary };
}

/** Replays a recorded run (one unit of it) and returns its state. */
function replay(events: Recorded[], unit = 0): FileRun {
  let r: FileRun = {
    runId: "", kind: "run", running: true, source: "", entries: {}, sending: {}, skipped: {},
    logs: [], messages: {}, current: 0, last: 0, dropped: 0, summary: null, error: null,
  };
  for (const e of events) {
    if (e.data.items) r = apply(r, e.data.items.filter((i) => i.unit === unit), e.data.dropped ?? 0);
    if (e.data.summary) r = { ...r, running: false, summary: e.data.summary };
  }
  return r;
}

/** The file's requests as the index lists them, from its entries. */
function requestsOf(r: FileRun): Request[] {
  return Object.values(r.entries).map((e) => ({
    file: "checkout.hurl",
    entry: e.index,
    method: e.calls![0].request.method,
    url: e.calls![0].request.url,
    line: e.line,
  }));
}

function CheckoutResults() {
  const run = replay(checkout as Recorded[]);
  const entries = Object.values(run.entries);
  const { passed, failed } = counts(entries);
  const rows = requestRows(requestsOf(run), run);
  const failing = entries.find((e) => !e.success)!;
  return (
    <>
      <ResultsHeader title="checkout.hurl" outcome="failed" passed={passed} failed={failed} durationMs={run.summary!.durationMs} env="local" />
      <RequestList rows={rows} selected={failing.index} />
      <FailureBox failure={failureOf(failing)!} onGoto={() => {}} />
    </>
  );
}

describe("run components on a recorded checkout run", () => {
  it("lists every request with its state", () => {
    render(<CheckoutResults />);
    const list = screen.getByRole("list", { name: "Requests" });
    const items = within(list).getAllByRole("listitem");
    expect(items).toHaveLength(5);
    expect(items[0]).toHaveAccessibleName("POST /login passed");
    expect(within(items[0]).getByText("→ token")).toBeInTheDocument();
    expect(items[4]).toHaveAccessibleName(/^POST \/carts\/.+\/checkout failed$/);
    expect(items[4]).toHaveAttribute("aria-selected", "true");
  });

  it("explains the failed assert", () => {
    render(<CheckoutResults />);
    const box = screen.getByRole("alert");
    expect(within(box).getByText("Assert failed")).toBeInTheDocument();
    expect(within(box).getByText('"paid"')).toBeInTheDocument();
    expect(within(box).getByText('"pending"')).toBeInTheDocument();
  });

  it("counts passed and failed checks in the header", () => {
    const { container } = render(<CheckoutResults />);
    expect(screen.getByText("Failed")).toHaveClass("outcome-failed");
    expect(container.querySelector(".run-counts")).toHaveTextContent(/^\d+ passed1 failed\d+ ms$/);
  });
});

describe("data row pills on a recorded data run", () => {
  const summary = (dataLogin as Recorded[]).find((e) => e.data.summary)!.data.summary!;
  const rows = summary.units!.map((u) => ({ row: u.row!, state: u.success ? ("passed" as const) : ("failed" as const) }));

  it("marks the failed row and selects on click", async () => {
    const onSelect = vi.fn();
    render(<DataRowPills rows={rows} selected={1} onSelect={onSelect} />);
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["Row 1", "Row 2", "Row 3"]);
    expect(tabs[1]).toHaveClass("state-failed");
    await userEvent.click(tabs[1]);
    expect(onSelect).toHaveBeenCalledWith(2);
  });

  it("explains a failed capture without expected and actual values", () => {
    const run = replay(dataLogin as Recorded[], 1);
    const f = failureOf(Object.values(run.entries)[0])!;
    expect(f.title).toBe("No query result");
    expect(f.expected).toBeUndefined();
    render(<FailureBox failure={f} />);
    expect(screen.getByText("query didn't return any result")).toBeInTheDocument();
  });
});

describe("an undefined variable", () => {
  it("offers to define it", async () => {
    const entry = { errors: [{ line: 4, kind: "", assert: false, description: "Undefined variable", message: "you must set the variable nmk-cookie" }] } as unknown as Entry;
    const f = failureOf(entry, ["", "", "", "Cookie: {{nmk-cookie}}"])!;
    expect(f.variable).toBe("nmk-cookie");
    const onDefine = vi.fn();
    render(<FailureBox failure={f} onDefine={onDefine} />);
    await userEvent.click(screen.getByRole("button", { name: "Define nmk-cookie…" }));
    expect(onDefine).toHaveBeenCalledWith("nmk-cookie");
  });
});
