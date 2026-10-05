// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Turns the screenshots of `make site-screens` (desktop/frontend/
// marketing-shots/<id>-<theme>.png, 1440x900 at 2x) into the site's images:
// site/public/screens/<id>-<theme>-<width>.<hash>.webp at three widths, and
// the manifest site/src/content/screens.json the landing page reads. The
// hash is the content's (the first 8 hex of its sha256), so a changed shot
// is a new file and the old ones go. The same input gives the same output.
// Needs SONDE_VERSION: the desktop version the shots show (the Makefile sets
// it). Also copies the share image to site/public/og.png.

import { createHash } from "node:crypto";
import { copyFile, mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import sharp from "sharp";

const site = join(dirname(fileURLToPath(import.meta.url)), "..");
const shots = join(site, "../desktop/frontend/marketing-shots");
const outDir = join(site, "public/screens");
const manifestFile = join(site, "src/content/screens.json");
const widths = [2880, 1440, 1080];
const themes = ["dark", "light"];
// The WebP quality by width: the large one is for retina screens, where a
// little loss does not show.
const quality = { 2880: 24, 1440: 40, 1080: 40 };
const budget = 2.5 * 1024 * 1024;

const version = process.env.SONDE_VERSION;
if (!version) throw new Error("SONDE_VERSION is not set (the desktop version the shots show; make site-screens sets it)");

const names = await readdir(shots).catch(() => []);
const ids = [...new Set(names.filter((n) => /-(dark|light)\.png$/.test(n)).map((n) => n.replace(/-(dark|light)\.png$/, "")))].sort();
if (ids.length === 0) throw new Error(`no screenshots in ${shots}: run make site-screens`);
const crops = JSON.parse(await readFile(join(shots, "crops.json"), "utf8"));

await mkdir(outDir, { recursive: true });
const keep = new Set();
let total = 0;
const manifest = { version, shots: {} };

for (const id of ids) {
  const entry = { width: 1440, height: 900 };
  for (const theme of themes) {
    const src = join(shots, `${id}-${theme}.png`);
    const meta = await sharp(src).metadata();
    if (meta.width !== 2880 || meta.height !== 1800) throw new Error(`${src}: ${meta.width}x${meta.height}, expected 2880x1800`);
    entry[theme] = [];
    for (const w of widths) {
      const data = await sharp(src).resize({ width: w }).webp({ quality: quality[w], effort: 6, smartSubsample: true }).toBuffer();
      const name = `${id}-${theme}-${w}.${createHash("sha256").update(data).digest("hex").slice(0, 8)}.webp`;
      await writeFile(join(outDir, name), data);
      keep.add(name);
      total += data.length;
      entry[theme].push({ w, src: `/screens/${name}` });
    }
  }
  if (crops[id]) entry.crop = crops[id];
  manifest.shots[id] = entry;
}

for (const name of await readdir(outDir)) if (!keep.has(name)) await rm(join(outDir, name), { recursive: true });
console.log(`${keep.size} images, ${(total / 1024).toFixed(0)} KiB, version ${version}`);
if (total > budget) throw new Error(`the images take ${total} bytes, over ${budget}: lower the quality`);
await writeFile(manifestFile, JSON.stringify(manifest, null, 2) + "\n");
await copyFile(join(shots, "og.png"), join(site, "public/og.png"));
