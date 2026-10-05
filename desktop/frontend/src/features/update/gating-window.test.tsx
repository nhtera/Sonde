// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The window app: the palette's check, the dialog, and the version's
// tooltip naming the engine.

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../lib/mode", async (importOriginal) => ({ ...(await importOriginal<object>()), updatesOn: true }));

await import("./index");
const { registry } = await import("../../app/registry");
const { StatusBar } = await import("../shell/status-bar");
const { useUpdate } = await import("../../state/update");

describe("updates in the window app", () => {
  it("are offered", () => {
    expect(registry.getCommand("app.checkUpdates")?.title).toBe("Check for updates…");
    expect(registry.getSlots("app.overlays").some((s) => s.id === "update.dialog")).toBe(true);
    useUpdate.setState({ info: { version: "0.2.0", engine: "v1.3.1+a7404ac", channel: "stable", testServer: true } });
    render(<StatusBar />);
    expect(screen.getByTitle("Sonde Desktop 0.2.0 · engine v1.3.1+a7404ac")).toBeInTheDocument();
    expect(screen.getByText("Test update server")).toBeInTheDocument();
  });
});
