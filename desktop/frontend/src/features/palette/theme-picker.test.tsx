// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({ Settings: { Get: vi.fn(), Set: vi.fn(async (v: unknown) => v) } }));
vi.mock("../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

// The OS is dark (jsdom has no matchMedia).
const { useSettings } = await import("../../state/settings");
const { useUI } = await import("../../state/ui");
const { ThemePicker } = await import("./theme-picker");

const value = {
  version: 1,
  appearance: { theme: "system", dayTheme: "light", nightTheme: "dark", uiFontSize: 13, codeFontSize: 13, sideWidth: 248, resultsWidth: 440, resultsTop: 0, ligatures: false },
  shortcuts: {},
};

const html = document.documentElement;
const sent = () => api.Settings.Set.mock.calls.at(-1)?.[0] as typeof value | undefined;

function open(appearance: Partial<typeof value.appearance> = {}) {
  const v = { ...structuredClone(value), appearance: { ...value.appearance, ...appearance } };
  useSettings.setState({ value: v as never });
  html.dataset.theme = appearance.theme && appearance.theme !== "system" ? appearance.theme : "dark";
  render(<ThemePicker />);
  act(() => useUI.getState().setThemePickerOpen(true));
}

describe("ThemePicker", () => {
  beforeAll(() => {
    // cmdk measures and scrolls its list.
    Element.prototype.scrollIntoView = () => {};
    globalThis.ResizeObserver ??= class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as never;
  });

  beforeEach(() => {
    api.Settings.Set.mockClear();
    useUI.getState().setThemePickerOpen(false);
  });

  it("starts on the theme in effect and previews the highlighted one", async () => {
    open();
    expect(screen.getByRole("option", { name: /^Dark/ })).toHaveAttribute("aria-selected", "true");
    expect(html.dataset.theme).toBe("dark");
    await userEvent.keyboard("{ArrowDown}");
    expect(html.dataset.theme).toBe("hc-dark");
    expect(sent()).toBeUndefined();
  });

  it("puts the theme back on Esc", async () => {
    open();
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    expect(html.dataset.theme).toBe("ayu-dark");
    await userEvent.keyboard("{Escape}");
    expect(useUI.getState().themePickerOpen).toBe(false);
    expect(html.dataset.theme).toBe("dark");
    expect(sent()).toBeUndefined();
  });

  it("filters by name", async () => {
    open();
    await userEvent.keyboard("gru");
    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual(["Gruvbox Light", "Gruvbox Dark"]);
  });

  it("keeps the pick as the night theme with Sync on a dark OS", async () => {
    open();
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(sent()?.appearance).toMatchObject({ theme: "system", nightTheme: "hc-dark" });
    expect(html.dataset.theme).toBe("hc-dark");
    expect(useUI.getState().themePickerOpen).toBe(false);
  });

  it("keeps the pick as the theme with Manual", async () => {
    open({ theme: "monokai" });
    expect(screen.getByRole("option", { name: /Monokai/ })).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(sent()?.appearance).toMatchObject({ theme: "night-owl", nightTheme: "dark" });
    expect(html.dataset.theme).toBe("night-owl");
  });
});
