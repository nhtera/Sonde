// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

import { readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { themes } from "./themes";

// Vitest turns CSS imports into empty modules; read the files themselves.
const read = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");
const css = read("./tokens.css");
const files = [css, ...readdirSync(new URL("./themes/", import.meta.url)).map((f) => read(`./themes/${f}`))];

/** Every theme's custom properties and color-scheme, by id. */
const sets = new Map<string, { vars: Record<string, string>; scheme: string }>();
for (const file of files) {
  for (const m of file.matchAll(/\[data-theme="([\w-]+)"\]\s*\{([^}]*)\}/g)) {
    const vars = Object.fromEntries([...m[2].matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((v) => [v[1], v[2].trim()]));
    sets.set(m[1], { vars, scheme: /color-scheme:\s*(\w+)/.exec(m[2])?.[1] ?? "" });
  }
}

/** The Go catalog (internal/settings/themes.go): id, kind, background. */
const goThemes = [...read("../../../../internal/settings/themes.go").matchAll(/\{"([\w-]+)", "(light|dark)", 0x([0-9a-f]{6})\}/g)].map((m) => ({
  id: m[1],
  kind: m[2],
  background: `#${m[3]}`,
}));

type RGB = [number, number, number];

/** A token's color: #rgb, #rrggbb, or rgba() laid over the background. */
function color(value: string, under?: RGB): RGB {
  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(value)?.[1];
  if (hex) {
    const full = hex.length === 3 ? [...hex].map((c) => c + c).join("") : hex;
    return [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16)) as RGB;
  }
  const rgba = /^rgba?\((\d+),\s*(\d+),\s*(\d+)(?:,\s*([\d.]+))?\)$/.exec(value);
  if (rgba && under) {
    const a = rgba[4] === undefined ? 1 : Number(rgba[4]);
    return [1, 2, 3].map((i, k) => Number(rgba[i]) * a + under[k] * (1 - a)) as RGB;
  }
  throw new Error(`unsupported color ${value}`);
}

/** WCAG 2.x contrast ratio. */
function contrast(a: RGB, b: RGB): number {
  const lum = (c: RGB) => {
    const [r, g, bl] = c.map((v) => {
      const s = v / 255;
      return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * bl;
  };
  const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe("design tokens", () => {
  const dark = sets.get("dark")!.vars;

  it("keeps the brief's dark palette, the default", () => {
    expect(css).toMatch(/:root,\s*\[data-theme="dark"\]\s*\{/);
    expect(dark).toMatchObject({
      "--backdrop": "#070b12",
      "--bg": "#0b111c",
      "--accent": "#5dc9b8",
    });
  });

  it("turns ligatures off", () => {
    expect(css).toMatch(/font-variant-ligatures:\s*none/);
  });

  it("has a token set for each theme of the catalogs, and no other", () => {
    const ids = themes.map((t) => t.id);
    expect([...sets.keys()].sort()).toEqual([...ids].sort());
    expect(goThemes.map((t) => t.id)).toEqual(ids);
    expect(goThemes.map((t) => t.kind)).toEqual(themes.map((t) => t.kind));
  });

  describe.each(themes.map((t) => [t.id, t] as const))("%s", (id, theme) => {
    const { vars, scheme } = sets.get(id) ?? { vars: {}, scheme: "" };
    const bg = color(vars["--bg"] ?? "#000");
    const on = (fg: string, back = "--bg") => contrast(color(vars[fg], bg), color(vars[back], bg));
    const body = id === "hc-dark" ? 7 : 4.5;

    it("defines every dark token, and its color scheme", () => {
      expect(Object.keys(vars).sort()).toEqual(Object.keys(dark).sort());
      expect(scheme).toBe(theme.kind);
    });

    it("shows the window in its panel color", () => {
      expect(goThemes.find((t) => t.id === id)?.background).toBe(vars["--panel"]?.toLowerCase());
    });

    it("is readable", () => {
      const fails: string[] = [];
      const check = (fg: string, back: string, min: number) => {
        const r = on(fg, back);
        if (r < min) fails.push(`${fg} on ${back}: ${r.toFixed(2)} < ${min}`);
      };
      for (const back of ["--bg", "--panel", "--raised"]) check("--text", back, body);
      for (const back of ["--bg", "--panel"]) check("--muted", back, body);
      check("--faint", "--bg", 3);
      check("--accent-ink", "--accent", 4.5);
      for (const fg of ["--pass", "--fail", "--warn", "--info"]) check(fg, "--bg", 3);
      if (id === "hc-dark") check("--line", "--bg", 3);
      expect(fails).toEqual([]);
    });

    it("matches the snapshot", () => {
      expect(vars).toMatchSnapshot();
    });
  });
});
