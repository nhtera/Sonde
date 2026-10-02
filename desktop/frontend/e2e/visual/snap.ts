// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The screens of the design references (plans/…/design, 1a–12c), in
// WebKit (the macOS app's engine) on the harness: snap() saves each as it
// is, for the batch review (E2E_CANDIDATES/visual), and compares it with
// its approved baseline (e2e/__screenshots__), the time-dependent parts
// hidden (visual.css). Run with E2E_VISUAL=1; update the baselines only
// after the screens are approved (--update-snapshots).

import { expect, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";

export async function snap(page: Page, id: string) {
  // Fonts and the last frame settled.
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(150);
  await page.screenshot({ path: `${candidates}/visual/${id}.png` });
  if (process.env.E2E_DUMP) await dump(page, `${process.env.E2E_DUMP}/${id}.json`);
  await expect(page).toHaveScreenshot(`${id}.png`, { stylePath: "e2e/visual/visual.css", maxDiffPixelRatio: 0.001 });
}

/** Opens the app on its tree. */
export async function home(page: Page) {
  // The window's own controls show, as the design draws the window.
  await page.addInitScript(() => sessionStorage.setItem("sonde.windowLook", "1"));
  await page.goto("/");
  await expect(page.locator(".tree-row").first()).toBeVisible();
}

/** Opens file from the tree. */
export async function open(page: Page, file: string) {
  if (!(await page.getByRole("tree", { name: "Project files" }).isVisible())) {
    await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name: "Files", exact: true }).click();
  }
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.getByRole("tablist", { name: "Open files" }).getByRole("tab", { selected: true })).toContainText(file);
  await expect(page.locator(".cm-content")).toBeVisible();
}

/** Runs the open file to an outcome. */
export async function run(page: Page, outcome: RegExp = /Passed|Failed|Error/) {
  await page.getByRole("banner").getByRole("button", { name: /^Run file/ }).click();
  await expect(page.getByRole("region", { name: "Results" }).locator(".run-header .outcome")).toHaveText(outcome, { timeout: 20_000 });
}

export async function panel(page: Page, name: string) {
  await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name, exact: true }).click();
}

export async function light(page: Page) {
  const html = page.locator("html");
  if ((await html.getAttribute("data-theme")) !== "light") {
    await page.getByRole("button", { name: "Toggle theme" }).click();
    // Off the button: no hover left in the screen.
    await page.mouse.move(720, 450);
  }
  await expect(html).toHaveAttribute("data-theme", "light");
}

export async function dark(page: Page) {
  const html = page.locator("html");
  if ((await html.getAttribute("data-theme")) !== "dark") {
    await page.getByRole("button", { name: "Toggle theme" }).click();
    // Off the button: no hover left in the screen.
    await page.mouse.move(720, 450);
  }
  await expect(html).toHaveAttribute("data-theme", "dark");
}

/** Writes each element with text or a background, its box and computed
 * style, to compare the screen with its design (E2E_DUMP=dir). */
async function dump(page: Page, path: string) {
  const rows = await page.evaluate(() => {
    const keys = ["color", "backgroundColor", "backgroundImage", "boxShadow", "borderRadius", "border", "fontFamily", "fontSize", "fontWeight", "lineHeight", "padding", "height", "opacity", "gap", "display"] as const;
    const out: unknown[] = [];
    for (const el of document.body.querySelectorAll("*")) {
      const own = [...el.childNodes].filter((n) => n.nodeType === 3).map((n) => n.textContent?.trim() ?? "").join(" ").trim();
      const cs = getComputedStyle(el);
      if (!own && cs.backgroundColor === "rgba(0, 0, 0, 0)" && cs.boxShadow === "none" && el.tagName !== "svg" && el.tagName !== "INPUT") continue;
      const b = el.getBoundingClientRect();
      if (b.width === 0 || b.height === 0) continue;
      const st: Record<string, string> = {};
      for (const k of keys) st[k] = cs[k];
      const value = (el as HTMLInputElement).value;
      out.push({ tag: el.tagName.toLowerCase(), text: (own || (typeof value === "string" ? value : "")).slice(0, 60), x: Math.round(b.x), y: Math.round(b.y), w: Math.round(b.width), h: Math.round(b.height), st });
    }
    return out;
  });
  const { writeFile, mkdir } = await import("node:fs/promises");
  const { dirname } = await import("node:path");
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, JSON.stringify(rows, null, 1));
}

