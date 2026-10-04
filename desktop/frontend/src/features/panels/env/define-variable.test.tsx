// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../lib/api", () => ({
  appError: (e: unknown) => ({ message: String(e) }),
  Envs: { SetSecret: vi.fn(), SetVariable: vi.fn(), SetOverride: vi.fn() },
}));

import { Envs } from "../../../lib/api";
import { useEnv } from "../../../state/env";
import { defaultWhere, defineVariable, DefineVariableDialog, nameError } from "./define-variable";

describe("defaultWhere", () => {
  it("makes a credential a secret, anything else a variable", () => {
    expect(defaultWhere("nmk-cookie", true)).toBe("secret");
    expect(defaultWhere("api_key", true)).toBe("secret");
    expect(defaultWhere("authToken", true)).toBe("secret");
    expect(defaultWhere("sessionId", true)).toBe("variable");
    expect(defaultWhere("base_url", true)).toBe("variable");
  });

  it("is this session only in a project with no environment", () => {
    expect(defaultWhere("nmk-cookie", false)).toBe("session");
  });
});

describe("DefineVariableDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useEnv.setState({
      current: "collection",
      project: { config: "sonde.yaml", envs: [{ name: "collection", default: true, secretsFile: "", variables: [] }] },
    });
  });

  it("writes a cookie to the environment's secrets file", async () => {
    render(<DefineVariableDialog />);
    act(() => defineVariable("nmk-cookie"));
    expect(screen.getByRole("radio", { name: /Secret of collection/ })).toBeChecked();
    expect(screen.getByText(/secrets\/collection\.secrets/)).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Value"), "sid=abc");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(Envs.SetSecret).toHaveBeenCalledWith("collection", "nmk-cookie", "sid=abc");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("writes a variable to sonde.yaml, or keeps it for the session", async () => {
    render(<DefineVariableDialog />);
    act(() => defineVariable("base_url"));
    await userEvent.type(screen.getByLabelText("Value"), "http://h");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(Envs.SetVariable).toHaveBeenCalledWith("collection", "base_url", "http://h");
    act(() => defineVariable("tmp"));
    await userEvent.click(screen.getByRole("radio", { name: /This session only/ }));
    await userEvent.type(screen.getByLabelText("Value"), "1");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(Envs.SetOverride).toHaveBeenCalledWith("collection", "tmp", "1");
  });

  it("offers only the session in a project with no environment", () => {
    useEnv.setState({ current: "", project: { config: "", envs: [] } });
    render(<DefineVariableDialog />);
    act(() => defineVariable("nmk-cookie"));
    expect(screen.getByRole("radio", { name: /This session only/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /^Secret/ })).toBeDisabled();
  });

  it("asks for the name from the palette, the default following it", async () => {
    render(<DefineVariableDialog />);
    act(() => defineVariable());
    expect(screen.getByRole("heading", { name: "Define a variable" })).toBeInTheDocument();
    const save = screen.getByRole("button", { name: "Save" });
    expect(save).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Name"), "a b");
    expect(screen.getByText("Letters, digits, _ and - only")).toBeInTheDocument();
    expect(save).toBeDisabled();
    await userEvent.clear(screen.getByLabelText("Name"));
    await userEvent.type(screen.getByLabelText("Name"), "authToken");
    expect(screen.getByRole("radio", { name: /Secret of collection/ })).toBeChecked();
    await userEvent.type(screen.getByLabelText("Value"), "t");
    await userEvent.click(save);
    expect(Envs.SetSecret).toHaveBeenCalledWith("collection", "authToken", "t");
  });
});

describe("nameError", () => {
  it("accepts a variable name, not a function's or one with spaces", () => {
    expect(nameError("nmk-cookie")).toBe("");
    expect(nameError("a.b")).not.toBe("");
    expect(nameError("newUuid")).toBe("newUuid is a function");
  });
});
