// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Settings › History & privacy, in the window app: the update check.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  Settings: { Get: vi.fn(), Set: vi.fn(async (v: unknown) => v) },
  Update: { Check: vi.fn(() => Promise.resolve()), ClearSkip: vi.fn(() => Promise.resolve()) },
}));
vi.mock("../../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));
vi.mock("../../../lib/mode", async (importOriginal) => ({ ...(await importOriginal<object>()), updatesOn: true }));

const { useSettings } = await import("../../../state/settings");
const { SettingsMain, goToSection } = await import("./settings-panel");
const { updateCheckNote } = await import("../../update/texts");
const { useUpdate } = await import("../../../state/update");
const { UpdateState } = await import("../../../lib/api");

const value = (updates: object) => ({
  version: 1,
  appearance: { theme: "system", dayTheme: "light", nightTheme: "dark", uiFontSize: 13, codeFontSize: 13, sideWidth: 248, resultsWidth: 440, resultsTop: 0, ligatures: false },
  shortcuts: {},
  network: { proxy: "", connectTimeout: "", retry: 0 },
  tls: { cacert: "", cert: "", key: "", skipVerify: false },
  cookies: { keep: false },
  history: { enabled: true, retention: "30d" },
  contract: { check: false },
  updates: { check: true, skipped: "", channel: "stable", lastCheck: "", ...updates },
});

beforeEach(() => {
  vi.clearAllMocks();
  goToSection("privacy");
});

describe("update settings", () => {
  it("says exactly what the check sends, and saves the switch alone", async () => {
    useSettings.setState({ value: value({ skipped: "0.2.1", lastCheck: "2026-10-05T01:00:00Z" }) as never });
    render(<SettingsMain />);
    expect(screen.getByText(updateCheckNote)).toBeInTheDocument();
    expect(updateCheckNote).toBe(
      "Once a day Sonde asks GitHub which Sonde Desktop versions exist (api.github.com). When one is newer, it downloads that version's signed description from github.com. These requests send nothing about you, your projects or your computer; GitHub sees your IP address, as with any download.",
    );
    expect(screen.getByText(/No account and no telemetry\. Sonde sends the requests you write, plus the update check when it is on\./)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("switch", { name: "Check for updates" }));
    expect(api.Settings.Set).toHaveBeenCalledWith(expect.objectContaining({ updates: { check: false, skipped: "0.2.1", channel: "stable", lastCheck: "2026-10-05T01:00:00Z" } }));
    await userEvent.click(screen.getByRole("button", { name: "Check now" }));
    expect(api.Update.Check).toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Show skipped updates again" }));
    expect(api.Update.ClearSkip).toHaveBeenCalled();
    expect(screen.getByText(/^Last checked /)).toBeInTheDocument();
  });

  it("shows the last result; with the check off, the same privacy line", () => {
    useSettings.setState({ value: value({ check: false }) as never });
    useUpdate.setState({ status: { seq: 1, state: UpdateState.StateUpToDate } as never });
    render(<SettingsMain />);
    expect(screen.getByText(/No account and no telemetry\. Sonde sends the requests you write, plus the update check when it is on\./)).toBeInTheDocument();
    expect(screen.getByText(/Not checked yet · up to date/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show skipped updates again" })).toBeNull();
  });
});
