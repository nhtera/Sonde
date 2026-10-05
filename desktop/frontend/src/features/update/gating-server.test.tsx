// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Server mode (and the harness unless asked) shows no update: no dialog,
// no palette command, no status-bar item, no version tooltip.

import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

await import("./index");
const { updatesOn } = await import("../../lib/mode");
const { registry } = await import("../../app/registry");
const { StatusBar } = await import("../shell/status-bar");
const { useUpdate } = await import("../../state/update");

describe("updates in server mode", () => {
  it("are absent", () => {
    expect(updatesOn).toBe(false);
    expect(registry.getCommand("app.checkUpdates")).toBeUndefined();
    expect(registry.getSlots("app.overlays").some((s) => s.id === "update.dialog")).toBe(false);
    useUpdate.setState({ info: { version: "0.2.0", engine: "v1.3.1+a7404ac", channel: "stable", testServer: true } });
    const { container } = render(<StatusBar />);
    expect(container.querySelector(".update-item, .update-tag")).toBeNull();
  });
});
