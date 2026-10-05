// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const ws = vi.hoisted(() => ({ createProject: vi.fn(), openFolder: vi.fn(), project: null as unknown }));
vi.mock("../../lib/api", () => ({
  appError: (e: unknown) => ({ message: e instanceof Error ? e.message : String(e) }),
  WorkspaceDesktop: { ProjectsDir: vi.fn(() => Promise.resolve("/Users/me/Documents/Sonde")), PickProjectsDir: vi.fn(() => Promise.resolve("/work")) },
}));
vi.mock("../../state/workspace", () => ({ useWorkspace: { getState: () => ws } }));

import { NewProjectDialog, newProject } from "./new-project";

beforeEach(() => {
  vi.clearAllMocks();
  ws.project = null;
});

describe("NewProjectDialog", () => {
  it("makes the project in Documents/Sonde, or the location picked, without the system dialog first", async () => {
    render(<NewProjectDialog />);
    let created: Promise<boolean> = Promise.resolve(false);
    act(() => void (created = newProject()));
    const name = await screen.findByLabelText("Project name");
    expect(name).toHaveValue("Untitled project");
    expect(await screen.findByText("/Users/me/Documents/Sonde/Untitled project")).toBeInTheDocument();
    await userEvent.clear(name);
    await userEvent.type(name, "NMK");
    await userEvent.click(screen.getByRole("button", { name: "Change…" }));
    expect(await screen.findByText("/work/NMK")).toBeInTheDocument();
    ws.createProject.mockResolvedValueOnce(true);
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(ws.createProject).toHaveBeenCalledWith("/work", "NMK");
    await expect(created).resolves.toBe(true);
    expect(screen.queryByLabelText("Project name")).toBeNull();
  });

  it("says why a project is not made, and stays open", async () => {
    render(<NewProjectDialog />);
    act(() => void newProject());
    await screen.findByText(/Documents\/Sonde\/Untitled project/);
    ws.createProject.mockRejectedValueOnce(new Error("a folder named Untitled project is there already"));
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("is there already");
    expect(screen.getByLabelText("Project name")).toBeInTheDocument();
  });

  it("for an import, offers an existing folder instead", async () => {
    render(<NewProjectDialog />);
    let created: Promise<boolean> = Promise.resolve(false);
    act(() => void (created = newProject(true)));
    expect(await screen.findByText("Make a project to import into")).toBeInTheDocument();
    ws.openFolder.mockImplementationOnce(async () => {
      ws.project = { name: "api", dir: "/api" };
    });
    await userEvent.click(screen.getByRole("button", { name: "Use an existing folder…" }));
    expect(ws.openFolder).toHaveBeenCalledWith(true);
    await expect(created).resolves.toBe(true);
  });
});
