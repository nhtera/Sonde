// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// Vitest turns CSS imports into empty modules; read the file itself.
const css = readFileSync(new URL("./tokens.css", import.meta.url), "utf8");

/** The custom properties of the rule whose selector starts at marker. */
function vars(marker: string): Record<string, string> {
  const start = css.indexOf(marker);
  const body = css.slice(css.indexOf("{", start) + 1, css.indexOf("}", start));
  return Object.fromEntries([...body.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
}

describe("design tokens", () => {
  const dark = vars(':root,\n[data-theme="dark"]');
  const light = vars('[data-theme="light"]');

  it("light defines every dark token", () => {
    expect(Object.keys(light).sort()).toEqual(Object.keys(dark).sort());
  });

  it("keeps the brief's dark palette", () => {
    expect(dark).toMatchObject({
      "--backdrop": "#070b12",
      "--bg": "#0b111c",
      "--accent": "#5dc9b8",
    });
  });

  it("matches the snapshot", () => {
    expect({ dark, light }).toMatchSnapshot();
  });

  it("turns ligatures off", () => {
    expect(css).toMatch(/font-variant-ligatures:\s*none/);
  });
});
