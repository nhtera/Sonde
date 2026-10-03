// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

const set = vi.fn();
vi.mock("../lib/api", () => ({ Settings: { Get: vi.fn(), Set: set } }));
vi.mock("../lib/events", () => ({ on: () => () => {} }));

const { resolvedTheme, toggled, useSettings } = await import("./settings");

const slots = { dayTheme: "light", nightTheme: "dark" };

describe("resolvedTheme", () => {
  it("follows the OS with Sync", () => {
    const a = { theme: "system", dayTheme: "solarized-light", nightTheme: "dracula" };
    expect(resolvedTheme(a, true).id).toBe("solarized-light");
    expect(resolvedTheme(a, false).id).toBe("dracula");
  });

  it("keeps the Manual theme", () => {
    expect(resolvedTheme({ theme: "monokai", ...slots }, true)).toMatchObject({ id: "monokai", kind: "dark" });
  });

  it("is Light or Dark, as the OS, for an unknown theme", () => {
    expect(resolvedTheme({ theme: "neon", ...slots }, true).id).toBe("light");
    expect(resolvedTheme({ theme: "system", dayTheme: "neon", nightTheme: "neon" }, false).id).toBe("dark");
  });
});

describe("toggled", () => {
  const custom = { dayTheme: "ayu-light", nightTheme: "nord" };

  it("switches Sync to Manual with the other slot's theme", () => {
    expect(toggled({ theme: "system", ...custom }, false)).toEqual({ theme: "ayu-light" });
    expect(toggled({ theme: "system", ...custom }, true)).toEqual({ theme: "nord" });
  });

  it("goes from a dark theme to the day theme, from a light one to the night theme", () => {
    expect(toggled({ theme: "dracula", ...custom }, true)).toEqual({ theme: "ayu-light" });
    expect(toggled({ theme: "solarized-light", ...custom }, false)).toEqual({ theme: "nord" });
  });

  it("flips Light and Dark with the default slots", () => {
    expect(toggled({ theme: "dark", ...slots }, false)).toEqual({ theme: "light" });
    expect(toggled({ theme: "light", ...slots }, true)).toEqual({ theme: "dark" });
    expect(toggled({ theme: "system", ...slots }, false)).toEqual({ theme: "light" });
    expect(toggled({ theme: "system", ...slots }, true)).toEqual({ theme: "dark" });
  });
});

describe("theme writes", () => {
  const value = {
    version: 1,
    appearance: { theme: "system", ...slots, uiFontSize: 13, codeFontSize: 13, sideWidth: 248, resultsWidth: 440, resultsTop: 0, ligatures: false },
    shortcuts: {},
  };

  beforeEach(() => {
    set.mockReset();
    useSettings.setState({ value: structuredClone(value) as never });
  });

  it("sends one write at a time, and an older reply never undoes a newer change", async () => {
    // Each Set answers only when the test says so.
    const answers: (() => void)[] = [];
    set.mockImplementation((next) => new Promise((done) => answers.push(() => done(structuredClone(next)))));
    const html = document.documentElement;
    const first = useSettings.getState().setSlot("night", "dracula");
    const second = useSettings.getState().setSlot("night", "nord");
    const third = useSettings.getState().setSlot("day", "github-light");
    expect(set).toHaveBeenCalledTimes(1);
    expect(html.dataset.theme).toBe("nord"); // shown at once (jsdom: no light OS)
    // The first reply (Dracula) comes after the later changes: ignored.
    answers[0]();
    await vi.waitFor(() => expect(set).toHaveBeenCalledTimes(2));
    expect(html.dataset.theme).toBe("nord");
    // The later changes go together.
    expect(set.mock.calls[1][0].appearance).toMatchObject({ dayTheme: "github-light", nightTheme: "nord" });
    answers[1]();
    await Promise.all([first, second, third]);
    expect(useSettings.getState().value?.appearance).toMatchObject({ dayTheme: "github-light", nightTheme: "nord" });
    expect(html.dataset.theme).toBe("nord");
  });

  it("keeps a picked theme in the slot in use with Sync, as the theme with Manual", async () => {
    set.mockImplementation(async (next) => next);
    await useSettings.getState().pickTheme("nord");
    expect(set.mock.calls[0][0].appearance).toMatchObject({ theme: "system", nightTheme: "nord" });
    await useSettings.getState().setTheme("monokai");
    await useSettings.getState().pickTheme("gruvbox-dark");
    expect(set.mock.calls[2][0].appearance).toMatchObject({ theme: "gruvbox-dark", nightTheme: "nord" });
  });

  it("reloads the settings when a change fails", async () => {
    set.mockRejectedValue(new Error("disk full"));
    const { Settings } = await import("../lib/api");
    vi.mocked(Settings.Get).mockResolvedValue(structuredClone(value) as never);
    await expect(useSettings.getState().setTheme("nord")).rejects.toThrow("disk full");
    expect(useSettings.getState().value?.appearance.theme).toBe("system");
  });
});
