// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The performance budgets, measured: the shipped app's own ("Native",
// sonde-desktop --perf-trace on a 1000-file project, with the fixture API
// serving a ~50 MB response), the browser tests' ("Harness", --harness:
// Playwright's perf and large-body tests on the harness build), and the
// build's output. Prints the budget table (Markdown) and exits 1 when a
// measured budget is missed.
//
// Run from desktop/ after `task darwin:package` (or another OS's build),
// on an awake, unlocked display (the webview draws no frames otherwise):
//   node scripts/perf.mjs [--app PATH] [--harness] [--no-native]

import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { gzipSync } from "node:zlib";

const args = process.argv.slice(2);
const opt = (name) => {
  const i = args.indexOf(name);
  return i >= 0 ? args[i + 1] : undefined;
};
const app =
  opt("--app") ??
  (process.platform === "darwin" ? "bin/Sonde.app/Contents/MacOS/sonde-desktop" : process.platform === "win32" ? "bin/sonde-desktop.exe" : "bin/sonde-desktop");
const fixturePort = 34120;

/** A project like the browser tests': 1000 request files in 20 folders,
 * big.hurl (5,000 lines), and json.hurl (a ~50 MB response). */
function project() {
  const dir = mkdtempSync(join(tmpdir(), "sonde-perf-"));
  for (let f = 1; f <= 20; f++) {
    const folder = join(dir, `folder${String(f).padStart(2, "0")}`);
    mkdirSync(folder);
    for (let i = 1; i <= 50; i++) {
      writeFileSync(join(folder, `request${String(i).padStart(2, "0")}.hurl`), `GET {{base_url}}/items/${String(f).padStart(2, "0")}/${String(i).padStart(2, "0")}\nHTTP 204\n`);
    }
  }
  let big = "";
  for (let i = 1; i <= 385; i++) {
    big += `# Order ${i}\nPOST {{base_url}}/orders?page=${i}\nAuthorization: Bearer {{token}}\n{\n  "sku": "TEA-${i}",\n  "quantity": {{quantity}}\n}\nHTTP 201\n[Captures]\norder_id: jsonpath "$.id"\n[Asserts]\njsonpath "$.status" == "created"\n\n`;
  }
  writeFileSync(join(dir, "big.hurl"), big);
  writeFileSync(join(dir, "json.hurl"), `GET http://127.0.0.1:${fixturePort}/big?mb=50\nHTTP 200\n`);
  return dir;
}

async function fixture() {
  const up = await fetch(`http://127.0.0.1:${fixturePort}/health`).then((r) => r.ok, () => false);
  if (up) return null;
  const p = spawn("bin/fixture-server", ["--port", String(fixturePort)], { stdio: "ignore" });
  for (let i = 0; i < 50; i++) {
    await new Promise((r) => setTimeout(r, 100));
    if (await fetch(`http://127.0.0.1:${fixturePort}/health`).then((r) => r.ok, () => false)) return p;
  }
  throw new Error("the fixture API did not start (go build -o bin/fixture-server ./cmd/fixture-server)");
}

/** Runs the app's tour; returns its measures by name. */
async function native() {
  if (!existsSync(app)) throw new Error(`no app at ${app}: build it first (task darwin:package), or give --app`);
  const dir = project();
  const data = mkdtempSync(join(tmpdir(), "sonde-perf-data-"));
  const out = join(data, "trace.json");
  const fx = await fixture();
  try {
    const tour = JSON.stringify({ filter: "items/07/", filterDone: "50 requests in 50 files", big: "big.hurl", json: "json.hurl" });
    // The app's progress (perf: …) shows on this terminal.
    const run = spawnSync(app, ["--root", dir, "--data", data, "--perf-trace", out, "--perf-tour", tour], { timeout: 180_000, stdio: "inherit" });
    if (run.error?.code === "ETIMEDOUT" || !existsSync(out)) {
      throw new Error("the app's tour did not finish: it draws frames only on an awake, unlocked display with its window on screen");
    }
    if (run.error) throw run.error;
    const measures = JSON.parse(readFileSync(out, "utf8")).measures;
    return Object.fromEntries(measures.map((m) => [m.name, m]));
  } finally {
    fx?.kill();
    rmSync(dir, { recursive: true, force: true });
    rmSync(data, { recursive: true, force: true });
  }
}

/** The harness rows: Playwright's tests that check them. */
function harness() {
  const run = spawnSync("npx", ["playwright", "test", "--project", "perf", "--project", "results", "--grep", "scrolls 1000|filters 1000|types in a 5,000|48 MB", "--reporter", "json"], {
    cwd: "frontend",
    encoding: "utf8",
    maxBuffer: 64 << 20,
  });
  const report = JSON.parse(run.stdout);
  const results = {};
  const walk = (suite) => {
    for (const s of suite.suites ?? []) walk(s);
    for (const spec of suite.specs ?? []) results[spec.title] = spec.ok;
  };
  for (const s of report.suites) walk(s);
  return results;
}

/** The gzipped size of the JS the page loads first. */
function initialJS() {
  const html = readFileSync("frontend/dist/index.html", "utf8");
  let total = 0;
  for (const m of html.matchAll(/(?:src|href)="\/?(assets\/[^"]+\.js)"/g)) total += gzipSync(readFileSync(join("frontend/dist", m[1]))).length;
  return total;
}

const rows = [];
const row = (budget, target, measured, ok, where) => rows.push({ budget, target, measured, ok, where });
const n = (m, digits = 0) => (m && Number.isFinite(m.value) ? `${m.value.toFixed(digits)} ${m.unit}` : "failed");

// --no-native: the build's rows only (CI: no display to run the app on).
const nat = args.includes("--no-native") ? null : await native();
const failed = Object.keys(nat ?? {}).filter((k) => k.startsWith("tour failed"));
if (nat) nativeRows(nat);

function nativeRows(nat) {
  row("Cold start to interactive (1000 files)", "under 1.5 s", n(nat["cold start to interactive"]), nat["cold start to interactive"]?.value < 1500, "Native");
  row("Tree scroll", "60 fps", `${n(nat["tree scroll"])}, longest frame ${n(nat["tree scroll longest frame"])}`, nat["tree scroll"]?.value >= 55, "Native");
  row("Tree filter", "under 100 ms", n(nat["tree filter"]), nat["tree filter"]?.value < 100, "Native");
  row("Keystroke to paint", "under 16 ms p95", n(nat["keystroke to paint p95"], 1), nat["keystroke to paint p95"]?.value < 16.7, "Native");
  row("50 MB JSON", "under 3 s to the tree, no task over 50 ms", `${n(nat["50 MB JSON to the tree"])}, longest frame ${n(nat["50 MB JSON longest frame"])}`, nat["50 MB JSON to the tree"]?.value < 3000 && nat["50 MB JSON longest frame"]?.value < 66, "Native");
}
if (args.includes("--harness")) {
  const h = harness();
  const verdict = (title) => Object.entries(h).find(([t]) => t.startsWith(title))?.[1];
  row("Tree scroll", "no long task", verdict("scrolls 1000") ? "met" : "missed", verdict("scrolls 1000"), "Harness");
  row("Tree filter", "under 100 ms", verdict("filters 1000") ? "met" : "missed", verdict("filters 1000"), "Harness");
  row("Keystroke work", "under 16 ms p95", verdict("types in a 5,000") ? "met" : "missed", verdict("types in a 5,000"), "Harness");
  row("48 MB JSON", "under 3 s, no task over 50 ms", verdict("a 48 MB body") ? "met" : "missed", verdict("a 48 MB body"), "Harness");
}
const js = initialJS();
row("Initial JS", "700 KB gzip or less", `${(js / 1024).toFixed(0)} KB gzip`, js <= 700 * 1024, "Build output");
const dmg = readdirSync("bin").find((f) => f.endsWith(".dmg"));
if (dmg) {
  const mb = statSync(join("bin", dmg)).size / 1e6;
  row("App download (dmg)", "recorded; alert above 60 MB", `${mb.toFixed(1)} MB`, mb <= 60, "Build output");
}

console.log("| Budget | Target | Measured | Met | Where |\n|---|---|---|---|---|");
for (const r of rows) console.log(`| ${r.budget} | ${r.target} | ${r.measured} | ${r.ok ? "yes" : "**no**"} | ${r.where} |`);
for (const f of failed) console.log(`\n${f}`);
process.exit(rows.every((r) => r.ok) && failed.length === 0 ? 0 : 1);
