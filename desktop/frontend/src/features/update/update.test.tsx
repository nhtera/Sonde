// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The update store, dialog, status-bar item and restart, with the
// bindings faked.

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UpdateStatus } from "../../lib/api";

const m = vi.hoisted(() => ({
  Update: {
    Install: vi.fn(() => Promise.resolve()),
    Skip: vi.fn(() => Promise.resolve()),
    Restart: vi.fn(() => Promise.resolve()),
    Check: vi.fn(() => Promise.resolve()),
    OpenReleasePage: vi.fn(() => Promise.resolve()),
    ClearSkip: vi.fn(() => Promise.resolve()),
    State: vi.fn(),
    Info: vi.fn(),
  },
  answer: true,
  confirm: vi.fn(() => Promise.resolve(m.answer)),
}));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), Update: m.Update }));
vi.mock("../../components/ask", () => ({ confirm: m.confirm }));

const { UpdateState, UpdateErrorKind } = await import("../../lib/api");
const { useUpdate } = await import("../../state/update");
const { useUI } = await import("../../state/ui");
const { useTabs } = await import("../../state/tabs");
const { UpdateDialog } = await import("./update-dialog");
const { UpdateItem } = await import("./update-item");

let seq = 0;
const status = (o: Partial<UpdateStatus>): UpdateStatus => ({
  seq: ++seq,
  state: UpdateState.StateIdle,
  version: "",
  notes: "",
  written: 0,
  total: 0,
  error: "",
  errorKind: UpdateErrorKind.$zero,
  manual: false,
  skipped: false,
  canInstall: true,
  reason: "",
  otherInstances: 0,
  ...o,
});
const offer = (o: Partial<UpdateStatus> = {}) => status({ state: UpdateState.StateAvailable, version: "0.2.1", notes: "Sonde Desktop 0.2.1", total: 1000, ...o });
const apply = (s: UpdateStatus) => act(() => useUpdate.getState().apply(s));

beforeEach(() => {
  vi.clearAllMocks();
  m.answer = true;
  useUpdate.setState({
    status: null,
    info: { version: "0.2.0", engine: "v1.3.1+a7404ac", channel: "stable", testServer: false },
    open: false,
    shownFind: false,
    lastOffered: "",
  });
  useUI.setState({ toasts: [] });
  useTabs.setState({ tabs: [], active: null, closed: [] });
});

describe("update store", () => {
  it("drops a status that is not newer", () => {
    const ready = status({ state: UpdateState.StateReady, version: "0.2.1" });
    const late = status({ state: UpdateState.StateDownloading, version: "0.2.1", written: 500, total: 1000 });
    late.seq = ready.seq - 1; // a progress event that arrives after ready
    apply(ready);
    apply(late);
    apply({ ...ready });
    expect(useUpdate.getState().status?.state).toBe(UpdateState.StateReady);
  });

  it("opens for the first background find, then toasts", () => {
    apply(offer());
    expect(useUpdate.getState().open).toBe(true);
    act(() => useUpdate.getState().hide());
    apply(offer({ version: "0.2.2" }));
    expect(useUpdate.getState().open).toBe(false);
    expect(useUI.getState().toasts.at(-1)).toMatchObject({ text: "Sonde Desktop 0.2.2 is available", action: { label: "View" } });
    // The same version found again (the next day, back online): no news.
    apply(status({ state: UpdateState.StateChecking }));
    apply(offer({ version: "0.2.2" }));
    expect(useUI.getState().toasts).toHaveLength(1);
  });

  it("answers a manual check: up to date as a toast, the rest in the dialog", () => {
    apply(status({ state: UpdateState.StateUpToDate, manual: true }));
    expect(useUI.getState().toasts.at(-1)?.text).toBe("Sonde Desktop is up to date (0.2.0).");
    expect(useUpdate.getState().open).toBe(false);
    apply(status({ state: UpdateState.StateError, errorKind: UpdateErrorKind.KindNetwork, manual: true }));
    expect(useUpdate.getState().open).toBe(true);
  });

  it("keeps a background error that is not a verification quiet", () => {
    apply(status({ state: UpdateState.StateError, errorKind: UpdateErrorKind.KindVerification }));
    expect(useUpdate.getState().open).toBe(false);
    expect(useUI.getState().toasts).toHaveLength(0);
  });
});

describe("update dialog", () => {
  const open = (s: UpdateStatus) => {
    apply(s);
    act(() => useUpdate.getState().show());
    return render(<UpdateDialog />);
  };

  it("offers the update: Install, Skip, Later", async () => {
    open(offer({ notes: "Fixes.\n<b>Not bold</b>" }));
    expect(screen.getByRole("dialog", { name: "Sonde Desktop 0.2.1 is available" })).toBeInTheDocument();
    expect(screen.getByText("You have 0.2.0.")).toBeInTheDocument();
    // The notes are text: the tag shows as typed, nothing is bold.
    expect(screen.getByText(/<b>Not bold<\/b>/)).toBeInTheDocument();
    expect(document.querySelector(".update-notes b")).toBeNull();
    expect(screen.getByRole("button", { name: "Install" })).toHaveFocus();
    await userEvent.click(screen.getByRole("button", { name: "Install" }));
    expect(m.Update.Install).toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Skip this version" }));
    expect(m.Update.Skip).toHaveBeenCalledWith("0.2.1");
  });

  it("closes on Esc: Later", async () => {
    open(offer());
    await userEvent.keyboard("{Escape}");
    expect(useUpdate.getState().open).toBe(false);
    expect(m.Update.Install).not.toHaveBeenCalled();
  });

  it("offers a skipped version from a manual check: Install anyway", async () => {
    open(offer({ skipped: true, manual: true }));
    expect(screen.getByRole("dialog", { name: "Sonde Desktop 0.2.1 (skipped) is available" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Skip this version" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Install anyway" }));
    expect(m.Update.Install).toHaveBeenCalled();
  });

  it("offers the release page where this copy cannot install", async () => {
    open(offer({ canInstall: false, reason: "Move Sonde to the Applications folder to install updates in place." }));
    expect(screen.getByText(/Move Sonde to the Applications folder/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Install" })).toBeNull();
    expect(screen.getByRole("button", { name: "Skip this version" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Download" }));
    expect(m.Update.OpenReleasePage).toHaveBeenCalled();
  });

  it("says a check is under way, and shows nothing between answers", () => {
    const view = open(status({ state: UpdateState.StateChecking }));
    expect(screen.getByRole("dialog", { name: "Checking for updates…" })).toBeInTheDocument();
    apply(status({ state: UpdateState.StateIdle }));
    view.rerender(<UpdateDialog />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("shows the progress", () => {
    open(status({ state: UpdateState.StateDownloading, version: "0.2.1", written: 430, total: 1000 }));
    expect(screen.getByText("Downloading… 43%")).toBeInTheDocument();
  });

  it("says each error in plain words, and offers the release page", async () => {
    const cases = [
      [UpdateErrorKind.KindNetwork, "Could not reach GitHub."],
      [UpdateErrorKind.KindVerification, "The update could not be verified and was not installed."],
      [UpdateErrorKind.KindRelease, "This release is not complete yet. Try again later."],
      [UpdateErrorKind.KindInstall, "The update did not finish. Sonde is still on 0.2.0."],
    ] as const;
    for (const [kind, words] of cases) {
      const view = open(status({ state: UpdateState.StateError, errorKind: kind, error: kind === UpdateErrorKind.KindInstall ? words : "detail" }));
      expect(screen.getByText(words)).toBeInTheDocument();
      await userEvent.click(screen.getByRole("button", { name: "Download" }));
      view.unmount();
    }
    expect(m.Update.OpenReleasePage).toHaveBeenCalledTimes(cases.length);
  });

  it("refuses to restart while other windows run", async () => {
    open(status({ state: UpdateState.StateReady, version: "0.2.1", otherInstances: 2 }));
    expect(screen.getByText("Quit the other Sonde windows first (2 open).")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Restart to update" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Check again" }));
    expect(m.Update.Check).toHaveBeenCalled();
  });
});

describe("restart to update", () => {
  const dirty = { path: "a.hurl", text: "x", savedText: "", hash: "h", version: 1, conflict: false };
  const ready = () => {
    apply(status({ state: UpdateState.StateReady, version: "0.2.1" }));
    act(() => useUpdate.getState().show());
    render(<UpdateDialog />);
  };

  it("restarts at once with nothing unsaved", async () => {
    ready();
    await userEvent.click(screen.getByRole("button", { name: "Restart to update" }));
    expect(m.confirm).not.toHaveBeenCalled();
    expect(m.Update.Restart).toHaveBeenCalled();
  });

  it("asks about unsaved tabs; Cancel keeps everything", async () => {
    useTabs.setState({ tabs: [dirty] });
    m.answer = false;
    ready();
    await userEvent.click(screen.getByRole("button", { name: "Restart to update" }));
    expect(m.confirm).toHaveBeenCalledWith({
      title: "Unsaved changes",
      message: "a.hurl has unsaved changes. Restart without saving?",
      submit: "Restart without saving",
    });
    expect(m.Update.Restart).not.toHaveBeenCalled();
    m.answer = true;
    await userEvent.click(screen.getByRole("button", { name: "Restart to update" }));
    expect(m.Update.Restart).toHaveBeenCalled();
  });
});

describe("status bar item", () => {
  it("shows each state", async () => {
    const view = render(<UpdateItem />);
    expect(view.container).toBeEmptyDOMElement();
    apply(offer());
    act(() => useUpdate.getState().hide());
    await userEvent.click(screen.getByRole("button", { name: "Update 0.2.1" }));
    expect(useUpdate.getState().open).toBe(true);
    apply(status({ state: UpdateState.StateDownloading, version: "0.2.1", written: 250, total: 1000 }));
    expect(screen.getByRole("button", { name: "Updating… 25%" })).toBeInTheDocument();
    apply(status({ state: UpdateState.StateReady, version: "0.2.1", otherInstances: 1 }));
    act(() => useUpdate.getState().hide());
    // Other windows open: the dialog says why, before any question.
    await userEvent.click(screen.getByRole("button", { name: "Restart to update" }));
    expect(useUpdate.getState().open).toBe(true);
    expect(m.confirm).not.toHaveBeenCalled();
    expect(m.Update.Restart).not.toHaveBeenCalled();
    apply(status({ state: UpdateState.StateError, errorKind: UpdateErrorKind.KindVerification }));
    expect(screen.getByRole("button", { name: "Update not verified" })).toHaveAttribute("data-warn");
    apply(status({ state: UpdateState.StateUpToDate }));
    expect(view.container).toBeEmptyDOMElement();
    act(() => useUpdate.setState({ info: { version: "0.2.0", engine: "x", channel: "stable", testServer: true } }));
    expect(screen.getByText("Test update server")).toBeInTheDocument();
  });
});
