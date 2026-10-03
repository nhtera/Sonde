// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({ Settings: { Get: vi.fn(), Set: vi.fn(async (v: unknown) => v) } }));
vi.mock("../../../lib/api", async (importOriginal) => ({ ...(await importOriginal<object>()), ...api }));

// The OS look, switchable.
const os = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  const os = { light: false, listeners, set: (light: boolean) => ((os.light = light), listeners.forEach((l) => l())) };
  window.matchMedia = ((query: string) => ({
    media: query,
    get matches() {
      return os.light;
    },
    addEventListener: (_: string, l: () => void) => listeners.add(l),
    removeEventListener: (_: string, l: () => void) => listeners.delete(l),
  })) as unknown as typeof window.matchMedia;
  return os;
});

const { useSettings } = await import("../../../state/settings");
const { ThemeSettings } = await import("./theme-settings");

const value = {
  version: 1,
  appearance: { theme: "system", dayTheme: "light", nightTheme: "dark", uiFontSize: 13, codeFontSize: 13, sideWidth: 248, resultsWidth: 440, resultsTop: 0, ligatures: false },
  shortcuts: {},
};

function Host() {
  const a = useSettings((s) => s.value!.appearance);
  return <ThemeSettings prefs={a} />;
}

const sent = () => api.Settings.Set.mock.calls.at(-1)![0] as typeof value;

describe("ThemeSettings", () => {
  beforeEach(() => {
    act(() => os.set(false));
    api.Settings.Set.mockClear();
    useSettings.setState({ value: structuredClone(value) as never });
  });

  it("shows the Day and Night themes with Sync, the Active badge following the OS", () => {
    render(<Host />);
    expect(screen.getByRole("radio", { name: "Sync with system" })).toBeChecked();
    const day = screen.getByRole("combobox", { name: "Day theme" });
    const night = screen.getByRole("combobox", { name: "Night theme" });
    expect(day).toHaveValue("light");
    expect(night).toHaveValue("dark");
    // The card's own kind first.
    expect(day.querySelector("optgroup")).toHaveAttribute("label", "Light");
    expect(night.querySelector("optgroup")).toHaveAttribute("label", "Dark");
    expect(screen.getByText("Active").closest(".theme-card")).toContainElement(night);
    act(() => os.set(true));
    expect(screen.getByText("Active").closest(".theme-card")).toContainElement(day);
  });

  it("sets a slot, and keeps a quick earlier change", async () => {
    render(<Host />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Night theme" }), "dracula");
    expect(sent().appearance).toMatchObject({ theme: "system", nightTheme: "dracula" });
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Day theme" }), "github-light");
    expect(sent().appearance).toMatchObject({ dayTheme: "github-light", nightTheme: "dracula" });
  });

  it("switches to Manual with the theme in effect, and back to Sync", async () => {
    useSettings.setState({ value: { ...structuredClone(value), appearance: { ...value.appearance, nightTheme: "nord" } } as never });
    render(<Host />);
    await userEvent.click(screen.getByRole("radio", { name: "Manual" }));
    expect(sent().appearance.theme).toBe("nord");
    const theme = screen.getByRole("combobox", { name: "Theme" });
    expect(theme).toHaveValue("nord");
    await userEvent.selectOptions(theme, "solarized-light");
    expect(sent().appearance.theme).toBe("solarized-light");
    await userEvent.click(screen.getByRole("radio", { name: "Sync with system" }));
    expect(sent().appearance.theme).toBe("system");
  });
});
