// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The desktop app's version is frontend/package.json's. This checks every
// other place it is written (the lockfile, the macOS Info.plist, the
// Wails config, the Windows info and installer) and, given a tag
// (desktop/vX.Y.Z), that the tag names it; --set writes a new version in
// all of them. Run from desktop/:
//   node scripts/version.mjs [tag]
//   node scripts/version.mjs --set X.Y.Z

import { readFileSync, writeFileSync } from "node:fs";

if (process.argv[2] === "--set") {
  const v = process.argv[3];
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(v ?? "")) {
    console.error("usage: node scripts/version.mjs --set X.Y.Z[-pre]");
    process.exit(2);
  }
  const edit = (file, f) => writeFileSync(file, f(readFileSync(file, "utf8")));
  for (const file of ["frontend/package.json", "frontend/package-lock.json"]) {
    edit(file, (t) => {
      const j = JSON.parse(t);
      j.version = v;
      if (j.packages?.[""]) j.packages[""].version = v;
      return JSON.stringify(j, null, 2) + "\n";
    });
  }
  edit("build/darwin/Info.plist", (t) =>
    t.replace(/(<key>CFBundle(?:ShortVersionString|Version)<\/key>\s*<string>)[^<]*(<\/string>)/g, `$1${v}$2`),
  );
  edit("build/config.yml", (t) => t.replace(/^(\s+version:\s*")[^"]*(")/m, `$1${v}$2`));
  // Windows' version numbers are numeric (X.Y.Z.W): a prerelease part
  // stays in the text fields only.
  const core = v.replace(/-.*/, "");
  edit("build/windows/info.json", (t) => {
    const j = JSON.parse(t);
    j.fixed.file_version = core;
    j.info["0000"].ProductVersion = v;
    return JSON.stringify(j, null, "\t") + "\n";
  });
  edit("build/windows/nsis/wails_tools.nsh", (t) => t.replace(/(!define INFO_PRODUCTVERSION ")[^"]*(")/, `$1${core}$2`));
  process.argv.length = 2; // then check them all
}

const version = JSON.parse(readFileSync("frontend/package.json", "utf8")).version;
const found = {
  "frontend/package-lock.json": JSON.parse(readFileSync("frontend/package-lock.json", "utf8")).version,
  "build/darwin/Info.plist (CFBundleShortVersionString)": /<key>CFBundleShortVersionString<\/key>\s*<string>([^<]*)<\/string>/.exec(readFileSync("build/darwin/Info.plist", "utf8"))?.[1],
  "build/darwin/Info.plist (CFBundleVersion)": /<key>CFBundleVersion<\/key>\s*<string>([^<]*)<\/string>/.exec(readFileSync("build/darwin/Info.plist", "utf8"))?.[1],
  "build/config.yml (info.version)": /^\s+version:\s*"([^"]*)"/m.exec(readFileSync("build/config.yml", "utf8"))?.[1],
  "build/windows/info.json (ProductVersion)": JSON.parse(readFileSync("build/windows/info.json", "utf8")).info?.["0000"]?.ProductVersion,
  "build/windows/info.json (file_version)": JSON.parse(readFileSync("build/windows/info.json", "utf8")).fixed?.file_version,
  "build/windows/nsis/wails_tools.nsh (INFO_PRODUCTVERSION)": /!define INFO_PRODUCTVERSION "([^"]*)"/.exec(readFileSync("build/windows/nsis/wails_tools.nsh", "utf8"))?.[1],
};
const tag = process.argv[2];
if (tag !== undefined) found[`the tag ${tag}`] = tag.replace(/^desktop\/v/, "");

// The numeric fields hold the version without its prerelease part.
const numeric = new Set(["build/windows/info.json (file_version)", "build/windows/nsis/wails_tools.nsh (INFO_PRODUCTVERSION)"]);
let bad = false;
for (const [where, v] of Object.entries(found)) {
  if (v !== (numeric.has(where) ? version.replace(/-.*/, "") : version)) {
    console.error(`${where}: ${v ?? "missing"}, frontend/package.json: ${version}`);
    bad = true;
  }
}
if (bad) process.exit(1);
console.log(`desktop version ${version}`);
