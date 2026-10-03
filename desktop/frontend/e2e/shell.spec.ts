// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The app shell on the shop-api project (its own harness, see
// playwright.config.ts), against the fixture API. Candidate screenshots
// for the design approval go to E2E_CANDIDATES (default e2e-candidates/).

import { readdirSync, readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";

const candidates = process.env.E2E_CANDIDATES ?? "e2e-candidates";

async function open(page: Page, file = "checkout.hurl") {
  await page.goto("/");
  await page.locator(".tree-row", { hasText: file }).first().click();
  await expect(page.getByRole("tab", { name: new RegExp(file) })).toHaveAttribute("aria-selected", "true");
}

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test("opens the project and a file", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator(".titlebar")).toContainText("shop-api");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("checkout.hurl");
  await open(page);
  await expect(page.getByRole("navigation", { name: "Path" })).toHaveText("checkout.hurl");
  await expect(page.locator(".cm-content")).toContainText("POST {{base_url}}/login");
  // Requests are listed under the open file.
  await expect(page.locator(".tree-row.req")).toHaveCount(5);
});

test("a request in the tree opens its file at the request", async ({ page }) => {
  await open(page);
  // POST /carts/{{cart_id}}/items, on line 22 of checkout.hurl.
  await page.locator(".tree-row.req").nth(3).click();
  await expect(page.locator(".statusbar")).toContainText("Ln 22, Col 1");
});

test("filters requests across files", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("checkout.hurl");
  await page.keyboard.press("ControlOrMeta+Shift+KeyF");
  const filter = page.getByLabel("Filter requests in all files");
  await expect(filter).toBeFocused();
  await filter.fill("carts");
  await expect(page.getByText(/^3 requests in 1 file ·/)).toBeVisible();
  await expect(page.locator(".tree-row.req .hl").first()).toHaveText("carts");
  await filter.press("Escape");
  await expect(filter).toHaveValue("");
});

test("runs a file and shows every result as it arrives", async ({ page }) => {
  await open(page);
  // The run starts from the keyboard; its events are subscribed to before
  // the call, so a run that ends at once still shows every request.
  await page.keyboard.press("ControlOrMeta+KeyR");
  const results = page.getByRole("region", { name: "Results" });
  await expect(results.locator(".outcome")).toHaveText("Failed");
  const rows = results.getByRole("list", { name: "Requests" }).getByRole("listitem");
  await expect(rows).toHaveCount(5);
  await expect(rows.nth(0)).toHaveAccessibleName("POST /login passed");
  await expect(rows.nth(4)).toHaveAccessibleName(/checkout failed$/);
  await expect(page.locator(".statusbar")).toContainText("✗ 1");
});

test.describe("themes", () => {
  type Prefs = { theme: string; dayTheme: string; nightTheme: string };
  const defaults: Prefs = { theme: "system", dayTheme: "light", nightTheme: "dark" };
  // The harness's hooks load after the page: waited for after a reload.
  const harness = (page: Page) => page.waitForFunction(() => !!window.sondeHarness);
  const setTheme = async (page: Page, prefs: Partial<Prefs> = {}) => {
    await harness(page);
    await page.evaluate((p) => window.sondeHarness!.setTheme(p), { ...defaults, ...prefs });
  };
  const prefs = async (page: Page) => {
    await harness(page);
    return page.evaluate(() => window.sondeHarness!.theme());
  };
  const html = (page: Page) => page.locator("html");

  // Each test starts from the default settings and a dark OS, and leaves
  // the defaults: the shell's tests share the harness's settings.
  test.beforeEach(async ({ page }) => {
    await page.emulateMedia({ colorScheme: "dark" });
    await page.goto("/");
    await setTheme(page);
    await expect(html(page)).toHaveAttribute("data-theme", "dark");
  });
  test.afterEach(async ({ page }) => {
    await setTheme(page);
  });

  test("switches the theme and keeps it", async ({ page }) => {
    await page.getByRole("button", { name: "Toggle theme" }).click();
    await expect(html(page)).toHaveAttribute("data-theme", "light");
    await expect.poll(() => prefs(page)).toEqual({ ...defaults, theme: "light" });
    await page.reload();
    await expect(html(page)).toHaveAttribute("data-theme", "light");
  });

  test("picks Day and Night themes", async ({ page }) => {
    await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name: "Settings", exact: true }).click();
    await page.getByRole("combobox", { name: "Night theme" }).selectOption("dracula");
    await expect(html(page)).toHaveAttribute("data-theme", "dracula");
    await page.emulateMedia({ colorScheme: "light" });
    await expect(html(page)).toHaveAttribute("data-theme", "light");
    await page.getByRole("combobox", { name: "Day theme" }).selectOption("solarized-light");
    await expect(html(page)).toHaveAttribute("data-theme", "solarized-light");
    await expect.poll(() => prefs(page)).toEqual({ theme: "system", dayTheme: "solarized-light", nightTheme: "dracula" });
    await page.reload();
    await expect(html(page)).toHaveAttribute("data-theme", "solarized-light");
    await page.emulateMedia({ colorScheme: "dark" });
    await expect(html(page)).toHaveAttribute("data-theme", "dracula");
    expect(await prefs(page)).toEqual({ theme: "system", dayTheme: "solarized-light", nightTheme: "dracula" });
  });

  test("Manual keeps one theme", async ({ page }) => {
    await page.getByRole("navigation", { name: "Panels" }).getByRole("button", { name: "Settings", exact: true }).click();
    await page.getByRole("radio", { name: "Manual" }).click();
    await page.getByRole("combobox", { name: "Theme", exact: true }).selectOption("monokai");
    await expect(html(page)).toHaveAttribute("data-theme", "monokai");
    await expect.poll(() => prefs(page)).toEqual({ ...defaults, theme: "monokai" });
    await page.emulateMedia({ colorScheme: "light" });
    await page.reload();
    await expect(html(page)).toHaveAttribute("data-theme", "monokai");
    await page.emulateMedia({ colorScheme: "dark" });
    await expect(html(page)).toHaveAttribute("data-theme", "monokai");
  });

  for (const mode of ["Sync", "Manual"] as const) {
    test(`Select theme previews, reverts and keeps the pick (${mode})`, async ({ page }) => {
      if (mode === "Manual") {
        await setTheme(page, { theme: "dark" });
        await page.reload();
      }
      const select = async () => {
        await page.keyboard.press("ControlOrMeta+KeyK");
        await page.keyboard.type(">Select theme");
        await page.keyboard.press("Enter");
        await expect(page.getByRole("dialog", { name: "Select theme" })).toBeVisible();
      };
      await select();
      await page.keyboard.press("ArrowDown");
      await expect(html(page)).toHaveAttribute("data-theme", "hc-dark");
      await page.keyboard.press("Escape");
      await expect(page.getByRole("dialog", { name: "Select theme" })).toHaveCount(0);
      await expect(html(page)).toHaveAttribute("data-theme", "dark");
      expect(await prefs(page)).toEqual(mode === "Sync" ? defaults : { ...defaults, theme: "dark" });

      await select();
      await page.keyboard.type("nord");
      await page.keyboard.press("Enter");
      await expect(html(page)).toHaveAttribute("data-theme", "nord");
      // With Sync on a dark OS, the night theme; with Manual, the theme.
      const want = mode === "Sync" ? { ...defaults, nightTheme: "nord" } : { ...defaults, theme: "nord" };
      await expect.poll(() => prefs(page)).toEqual(want);
      await page.reload();
      await expect(html(page)).toHaveAttribute("data-theme", "nord");
    });
  }

  test("paints in the theme from the start, within the CSP", async ({ page }) => {
    await setTheme(page, { nightTheme: "dracula" });
    // The page as served carries the settings.
    const served = await (await page.request.get("/")).text();
    expect(served).toContain('data-theme-pref="system" data-theme-day="light" data-theme-night="dracula"');
    // The boot script alone (the app's code blocked) sets the theme.
    await page.route("**/assets/*.js", (r) => r.abort());
    await page.reload();
    await expect(html(page)).toHaveAttribute("data-theme", "dracula");
    await page.unroute("**/assets/*.js");
    // No script refused.
    const refused: string[] = [];
    page.on("console", (m) => {
      if (/Refused to|Content Security Policy/i.test(m.text())) refused.push(m.text());
    });
    await page.addInitScript(() => {
      document.addEventListener("securitypolicyviolation", (e) => console.error(`Content Security Policy: ${e.violatedDirective} ${e.blockedURI}`));
    });
    await page.reload();
    await expect(page.locator(".titlebar")).toContainText("shop-api");
    await expect(html(page)).toHaveAttribute("data-theme", "dracula");
    expect(refused).toEqual([]);
  });

  test("every theme reaches the page", async ({ page }) => {
    // Each theme's --panel from its CSS, as the Go catalog has it (the
    // token tests check that).
    const dir = new URL("../src/app/theme/", import.meta.url);
    const files = [readFileSync(new URL("tokens.css", dir), "utf8"), ...readdirSync(new URL("themes/", dir)).map((f) => readFileSync(new URL(`themes/${f}`, dir), "utf8"))];
    const panels = new Map<string, string>();
    for (const css of files) for (const m of css.matchAll(/\[data-theme="([\w-]+)"\]\s*\{[^}]*?--panel:\s*(#\w+);/g)) panels.set(m[1], m[2]);
    expect(panels.size).toBe(24);
    const got = await page.evaluate((ids) => {
      const root = document.documentElement;
      return ids.map((id) => {
        root.dataset.theme = id;
        return getComputedStyle(root).getPropertyValue("--panel").trim();
      });
    }, [...panels.keys()]);
    expect(got).toEqual([...panels.values()]);
  });
});

test("the palette opens files, and > lists commands only", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("tree", { name: "Project files" })).toContainText("users.hurl");
  await page.keyboard.press("ControlOrMeta+KeyK");
  const palette = page.getByRole("dialog");
  await page.keyboard.type("users");
  // The file first, then its requests (GET /users/… · users.hurl:3).
  await expect(palette.getByRole("option", { name: /^users\.hurl/ })).toBeVisible();
  await expect(palette.getByRole("option", { name: /^GET \/users/ }).first()).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("tab", { name: /users\.hurl/ })).toHaveAttribute("aria-selected", "true");

  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.keyboard.type(">");
  await expect(palette.getByRole("option", { name: /\.hurl/ })).toHaveCount(0);
  await page.keyboard.type("keyboard");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
});

test("rebinds a shortcut and refuses a conflict", async ({ page }) => {
  await open(page, "users.hurl");
  await page.getByRole("button", { name: "? Shortcuts" }).click();
  const sheet = page.getByRole("dialog", { name: "Keyboard shortcuts" });
  const rebind = sheet.getByRole("button", { name: "Rebind Run file" });

  // Keys another command uses are refused and reported.
  await rebind.click();
  await page.keyboard.press("ControlOrMeta+KeyK");
  await expect(sheet.getByRole("alert")).toContainText("is used by Search files and commands");

  await page.keyboard.press("Alt+Shift+KeyR");
  await expect(rebind).toContainText(/R/);
  await sheet.getByRole("button", { name: "Done" }).click();

  await page.keyboard.press("Alt+Shift+KeyR");
  await expect(page.getByRole("region", { name: "Results" }).locator(".outcome")).toHaveText(/Passed|Failed/);

  // Back to the default.
  await page.getByRole("button", { name: "? Shortcuts" }).click();
  await sheet.locator("li", { hasText: "Run file" }).getByRole("button", { name: "Reset" }).click();
  await expect(rebind).not.toContainText("Alt");
});

test("candidate screenshots: dark, light and 1024px", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await open(page);
  await page.keyboard.press("ControlOrMeta+KeyR");
  await expect(page.getByRole("region", { name: "Results" }).locator(".outcome")).toHaveText("Failed");
  const theme = (t: string) => page.evaluate((t) => document.documentElement.setAttribute("data-theme", t), t);
  await theme("dark");
  await page.screenshot({ path: `${candidates}/shell-dark.png` });
  await theme("light");
  await page.screenshot({ path: `${candidates}/shell-light.png` });
  await theme("dark");
  await page.setViewportSize({ width: 1024, height: 768 });
  await expect(page.locator(".window")).toHaveAttribute("data-narrow", "yes");
  await page.screenshot({ path: `${candidates}/shell-1024.png` });
  await page.getByRole("button", { name: "Results" }).click();
  await expect(page.getByRole("region", { name: "Results" })).toBeHidden();
});
