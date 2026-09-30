// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { KvGrid } from "./kv-grid";
import { type EntryModel, type ModelRow } from "./model";

// Mock the edit.ts module to capture API calls
vi.mock("./edit", () => ({
  formEdit: vi.fn(),
  setRow: vi.fn(),
}));

// Mock the EditSvc API
vi.mock("../../lib/api", () => ({
  EditSvc: {
    Apply: vi.fn(),
    Batch: vi.fn(),
  },
}));

import { formEdit, setRow } from "./edit";

const createEntry = (rowsData: Record<string, ModelRow[]> = {}): EntryModel => ({
  Index: 1,
  Range: { Start: 0, End: 100 },
  Method: "POST",
  MethodRange: { Start: 0, End: 4 },
  URL: "https://api.test/x",
  URLRange: { Start: 5, End: 25 },
  HasResponse: true,
  Status: "200",
  StatusRange: { Start: 26, End: 29 },
  HasBody: false,
  Body: "",
  BodyRange: { Start: 30, End: 30 },
  Rows: {
    headers: rowsData.headers || [],
    query: rowsData.query || [],
    form: rowsData.form || [],
    multipart: rowsData.multipart || [],
    cookies: rowsData.cookies || [],
    "basic-auth": rowsData["basic-auth"] || [],
    options: rowsData.options || [],
    "response-headers": rowsData["response-headers"] || [],
    captures: rowsData.captures || [],
    asserts: rowsData.asserts || [],
    grpc: rowsData.grpc || [],
  },
});

describe("KvGrid", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders existing rows", () => {
    const entry = createEntry({
      headers: [
        { Key: "Content-Type", Value: "application/json", Disabled: false, Range: { Start: 0, End: 20 } },
        { Key: "Authorization", Value: "Bearer {{token}}", Disabled: false, Range: { Start: 20, End: 40 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    expect(screen.getByDisplayValue("Content-Type")).toBeInTheDocument();
    expect(screen.getByDisplayValue("application/json")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Authorization")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Bearer {{token}}")).toBeInTheDocument();
  });

  it("toggles row enabled/disabled via checkbox", async () => {
    const user = userEvent.setup();
    const entry = createEntry({
      headers: [
        { Key: "X-Test", Value: "value", Disabled: false, Range: { Start: 0, End: 10 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    const checkbox = screen.getByRole("checkbox", { name: /disable/i });
    await user.click(checkbox);

    expect(formEdit).toHaveBeenCalledWith("test.hurl", {
      kind: "toggleRow",
      entry: 1,
      section: "headers",
      index: 0,
    });
  });

  it("removes a row via remove button", async () => {
    const user = userEvent.setup();
    const entry = createEntry({
      headers: [
        { Key: "X-Test", Value: "value", Disabled: false, Range: { Start: 0, End: 10 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    const removeButton = screen.getByRole("button", { name: /remove x-test/i });
    await user.click(removeButton);

    expect(formEdit).toHaveBeenCalledWith("test.hurl", {
      kind: "removeRow",
      entry: 1,
      section: "headers",
      index: 0,
    });
  });

  it("allows editing an existing row's value", async () => {
    const user = userEvent.setup();
    const entry = createEntry({
      headers: [
        { Key: "Content-Type", Value: "application/json", Disabled: false, Range: { Start: 0, End: 20 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    const valueInput = screen.getByDisplayValue("application/json");
    await user.clear(valueInput);
    await user.type(valueInput, "application/xml");
    await user.keyboard("{Enter}");

    expect(setRow).toHaveBeenCalledWith("test.hurl", 1, "headers", 0, "Content-Type", "application/xml");
  });

  it("adds a new row once with both key and value when focus leaves", async () => {
    const user = userEvent.setup();
    const entry = createEntry({ headers: [] });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    // Click add row button
    const addButton = screen.getByRole("button", { name: /add row/i });
    await user.click(addButton);

    // Find the new row inputs
    const keyInputs = screen.getAllByPlaceholderText("Key");
    const valueInputs = screen.getAllByPlaceholderText("Value");

    const newKeyInput = keyInputs[keyInputs.length - 1];
    const newValueInput = valueInputs[valueInputs.length - 1];

    // Type into the new row
    await user.type(newKeyInput, "X-Custom");
    await user.type(newValueInput, "custom-value");

    // Blur to trigger the row add
    await user.click(document.body);

    // Should call setRow with index -1 to add new row
    expect(setRow).toHaveBeenCalledWith("test.hurl", 1, "headers", -1, "X-Custom", "custom-value");
    // Should only be called once
    expect(setRow).toHaveBeenCalledTimes(1);
  });

  it("does not add a new row if key is empty", async () => {
    const user = userEvent.setup();
    const entry = createEntry({ headers: [] });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    const addButton = screen.getByRole("button", { name: /add row/i });
    await user.click(addButton);

    const valueInputs = screen.getAllByPlaceholderText("Value");
    const newValueInput = valueInputs[valueInputs.length - 1];
    await user.type(newValueInput, "some-value");

    // Blur without typing key
    await user.click(document.body);

    expect(setRow).not.toHaveBeenCalled();
  });

  it("shows multiline values as read-only", () => {
    const entry = createEntry({
      headers: [
        { Key: "Description", Value: "line1\nline2\nline3", Disabled: false, Range: { Start: 0, End: 30 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    // Multiline value should show first line + ellipsis and be read-only
    expect(screen.getByText(/line1 …/)).toBeInTheDocument();
    expect(screen.getByTitle(/edit it in text/i)).toBeInTheDocument();
  });

  it("disables key editing for multiline values", () => {
    const entry = createEntry({
      headers: [
        { Key: "X-Multi", Value: "a\nb\nc", Disabled: false, Range: { Start: 0, End: 10 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    const keyInput = screen.getByDisplayValue("X-Multi") as HTMLInputElement;
    expect(keyInput.readOnly).toBe(true);
  });

  it("shows disabled rows with strikethrough styling", () => {
    const entry = createEntry({
      headers: [
        { Key: "X-Debug", Value: "true", Disabled: true, Range: { Start: 0, End: 10 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="headers" />);

    const row = screen.getByRole("row", { name: /x-debug/i });
    expect(row).toHaveClass("off");
  });

  it("shows table header when rows are present", () => {
    const entry = createEntry({
      query: [
        { Key: "q", Value: "search", Disabled: false, Range: { Start: 0, End: 10 } },
      ],
    });

    render(<KvGrid file="test.hurl" entry={entry} sec="query" keyLabel="Param" valueLabel="Value" />);

    expect(screen.getByText("Param")).toBeInTheDocument();
    expect(screen.getByText("Value")).toBeInTheDocument();
  });

  it("hides table header when no rows and not adding", () => {
    const entry = createEntry({ query: [] });

    render(<KvGrid file="test.hurl" entry={entry} sec="query" />);

    expect(screen.queryByRole("row")).not.toBeInTheDocument();
  });

  it("shows table header when adding first row", async () => {
    const user = userEvent.setup();
    const entry = createEntry({ query: [] });

    render(<KvGrid file="test.hurl" entry={entry} sec="query" />);

    const addButton = screen.getByRole("button", { name: /add row/i });
    await user.click(addButton);

    // Header should now be visible
    expect(screen.getByRole("columnheader", { name: "Key" })).toBeInTheDocument();
  });

  it("filters rows with only() predicate", () => {
    const entry = createEntry({
      options: [
        { Key: "retry", Value: "3", Disabled: false, Range: { Start: 0, End: 10 } },
        { Key: "cert", Value: "cert.pem", Disabled: false, Range: { Start: 10, End: 25 } },
        { Key: "key", Value: "key.pem", Disabled: false, Range: { Start: 25, End: 38 } },
      ],
    });

    render(
      <KvGrid
        file="test.hurl"
        entry={entry}
        sec="options"
        only={(r) => r.Key === "cert" || r.Key === "key"}
      />
    );

    expect(screen.getByDisplayValue("cert.pem")).toBeInTheDocument();
    expect(screen.getByDisplayValue("key.pem")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("3")).not.toBeInTheDocument();
  });

  it("respects custom key/value labels", () => {
    const entry = createEntry({
      form: [
        { Key: "email", Value: "user@example.com", Disabled: false, Range: { Start: 0, End: 20 } },
      ],
    });

    render(
      <KvGrid
        file="test.hurl"
        entry={entry}
        sec="form"
        keyLabel="Field Name"
        valueLabel="Field Value"
      />
    );

    expect(screen.getByText("Field Name")).toBeInTheDocument();
    expect(screen.getByText("Field Value")).toBeInTheDocument();
  });

  it("uses custom add label", async () => {
    const entry = createEntry({ form: [] });

    render(
      <KvGrid
        file="test.hurl"
        entry={entry}
        sec="form"
        addLabel="+ Add field"
      />
    );

    expect(screen.getByRole("button", { name: /add field/i })).toBeInTheDocument();
  });

  it("handles custom value renderer", () => {
    const entry = createEntry({
      multipart: [
        { Key: "file", Value: "file,data.bin;", Disabled: false, Range: { Start: 0, End: 15 } },
      ],
    });

    const customRender = (r: ModelRow) => <span className="custom">Custom: {r.Value}</span>;

    render(
      <KvGrid
        file="test.hurl"
        entry={entry}
        sec="multipart"
        renderValue={customRender}
      />
    );

    expect(screen.getByText(/Custom: file,data.bin;/)).toBeInTheDocument();
  });
});
