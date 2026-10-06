// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { EntryModel } from "../model";
import { ParamsTab } from "./params";

vi.mock("../edit", () => ({ formEdit: vi.fn(), setRow: vi.fn() }));
vi.mock("../../../lib/api", () => ({ EditSvc: { Apply: vi.fn(), Batch: vi.fn() } }));

import { formEdit } from "../edit";

const entry = (over: Partial<EntryModel> = {}): EntryModel => ({
  Index: 1, Range: { Start: 0, End: 0 }, Method: "GET", MethodRange: { Start: 0, End: 0 },
  URL: "http://localhost:3001/api/v1/dashboard?from=2026-09-06T15%3A00%3A00.000Z&to=2026-10-06T15%3A00%3A00.000Z",
  URLRange: { Start: 0, End: 0 }, URLQuery: null, URLQueryKept: "", HasResponse: false, Status: "", StatusRange: { Start: 0, End: 0 },
  HasBody: false, Body: "", BodyRange: { Start: 0, End: 0 }, Rows: {}, ...over,
});

describe("ParamsTab", () => {
  beforeEach(() => vi.clearAllMocks());

  // A curl import keeps the URL as copied: its params show, and move to
  // [Query] in one edit.
  it("lists the URL's own params and moves them to [Query]", async () => {
    const e = entry({ URLQuery: [{ Key: "from", Value: "2026-09-06T15:00:00.000Z" }, { Key: "to", Value: "2026-10-06T15:00:00.000Z" }] });
    render(<ParamsTab file="t.hurl" entry={e} />);
    expect(screen.getByRole("table", { name: "URL query params" })).toBeInTheDocument();
    expect(screen.getByLabelText("URL param 1")).toHaveValue("from");
    expect(screen.getByLabelText("URL param 2 value")).toHaveValue("2026-10-06T15:00:00.000Z");
    expect(screen.getByLabelText("URL param 1")).toHaveAttribute("readonly");
    await userEvent.click(screen.getByRole("button", { name: "Move to [Query]" }));
    expect(formEdit).toHaveBeenCalledWith("t.hurl", { kind: "moveURLQuery", entry: 1 });
  });

  it("says why params stay in the URL", () => {
    render(<ParamsTab file="t.hurl" entry={entry({ URLQuery: [{ Key: "flag", Value: "" }], URLQueryKept: `"flag" is not name=value` })} />);
    expect(screen.queryByRole("button", { name: "Move to [Query]" })).toBeNull();
    expect(screen.getByText(/"flag" is not name=value/)).toBeInTheDocument();
  });

  it("shows only [Query] rows when the URL has no query string", () => {
    render(<ParamsTab file="t.hurl" entry={entry({ URL: "http://h/p" })} />);
    expect(screen.queryByRole("table", { name: "URL query params" })).toBeNull();
    expect(screen.getByRole("table", { name: "query rows" })).toBeInTheDocument();
  });
});
