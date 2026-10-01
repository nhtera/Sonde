// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { execSync } from "node:child_process";
import { test } from "node:test";
import assert from "node:assert";

const tmpBase = tmpdir();

// The files version.mjs reads and writes, and the script itself.
const files = [
  "scripts/version.mjs",
  "frontend/package.json",
  "frontend/package-lock.json",
  "build/darwin/Info.plist",
  "build/config.yml",
  "build/windows/info.json",
  "build/windows/nsis/wails_tools.nsh",
];

// Copies those files of projectDir to a temp directory (removed after
// the test t) and returns its path.
function copyProject(projectDir, t) {
  const tempDir = mkdtempSync(join(tmpBase, "sonde-version-test-"));
  t.after(() => rmSync(tempDir, { recursive: true, force: true }));
  for (const f of files) {
    mkdirSync(dirname(join(tempDir, f)), { recursive: true });
    copyFileSync(join(projectDir, f), join(tempDir, f));
  }
  return tempDir;
}

// Run version.mjs with cwd in tempDir
function runVersionScript(tempDir, ...args) {
  const script = join(tempDir, "scripts", "version.mjs");
  const cmd = `node ${script} ${args.join(" ")}`;
  try {
    const output = execSync(cmd, { cwd: tempDir, encoding: "utf8", stdio: "pipe" });
    return { code: 0, output };
  } catch (e) {
    return { code: e.status, output: e.stderr || e.stdout || "" };
  }
}

// Read version from a file using a regex
function readVersion(file, regex) {
  const content = readFileSync(file, "utf8");
  const match = content.match(regex);
  return match ? match[1] : undefined;
}

test("version.mjs --set updates all files", async (t) => {
  const desktopDir = dirname(dirname(fileURLToPath(import.meta.url)));
  const tempDir = copyProject(desktopDir, t);
  const version = "1.2.3-rc.1";

  // Set version
  const setResult = runVersionScript(tempDir, "--set", version);
  assert.strictEqual(setResult.code, 0, `--set failed: ${setResult.output}`);

  // Check all files have the new version
  const versionInPkg = JSON.parse(readFileSync(join(tempDir, "frontend", "package.json"), "utf8")).version;
  assert.strictEqual(versionInPkg, version, "frontend/package.json version not updated");

  const versionInLock = JSON.parse(readFileSync(join(tempDir, "frontend", "package-lock.json"), "utf8")).version;
  assert.strictEqual(versionInLock, version, "frontend/package-lock.json version not updated");

  const plistShort = readVersion(
    join(tempDir, "build", "darwin", "Info.plist"),
    /<key>CFBundleShortVersionString<\/key>\s*<string>([^<]*)<\/string>/
  );
  assert.strictEqual(plistShort, version, "Info.plist CFBundleShortVersionString not updated");

  const plistBuild = readVersion(
    join(tempDir, "build", "darwin", "Info.plist"),
    /<key>CFBundleVersion<\/key>\s*<string>([^<]*)<\/string>/
  );
  assert.strictEqual(plistBuild, version, "Info.plist CFBundleVersion not updated");

  const configYml = readVersion(
    join(tempDir, "build", "config.yml"),
    /^\s+version:\s*"([^"]*)"/m
  );
  assert.strictEqual(configYml, version, "build/config.yml version not updated");

  const infoJson = JSON.parse(readFileSync(join(tempDir, "build", "windows", "info.json"), "utf8"));
  // Windows' numeric versions take the version without its prerelease part.
  assert.strictEqual(infoJson.fixed.file_version, "1.2.3", "windows/info.json file_version not the numeric core");
  assert.strictEqual(infoJson.info["0000"].ProductVersion, version, "windows/info.json ProductVersion not updated");

  const nshFile = readVersion(
    join(tempDir, "build", "windows", "nsis", "wails_tools.nsh"),
    /!define INFO_PRODUCTVERSION "([^"]*)"/
  );
  assert.strictEqual(nshFile, "1.2.3", "wails_tools.nsh version not the numeric core");

  // Verify by running check (no args)
  const checkResult = runVersionScript(tempDir);
  assert.strictEqual(checkResult.code, 0, `verification check failed: ${checkResult.output}`);
});

test("version.mjs detects mismatched versions", async (t) => {
  const desktopDir = dirname(dirname(fileURLToPath(import.meta.url)));
  const tempDir = copyProject(desktopDir, t);

  // Corrupt one file: manually change package.json version
  const pkgPath = join(tempDir, "frontend", "package.json");
  const pkg = JSON.parse(readFileSync(pkgPath, "utf8"));
  pkg.version = "9.9.9";
  writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");

  // Check should fail
  const checkResult = runVersionScript(tempDir);
  assert.notStrictEqual(checkResult.code, 0, "version check should fail on mismatch");
  assert(checkResult.output.includes("9.9.9"), "error message should mention mismatched version");
});

test("version.mjs validates tag format", async (t) => {
  const desktopDir = dirname(dirname(fileURLToPath(import.meta.url)));
  const tempDir = copyProject(desktopDir, t);

  // First set a version
  const setResult = runVersionScript(tempDir, "--set", "1.2.3-rc.1");
  assert.strictEqual(setResult.code, 0, `--set should succeed: ${setResult.output}`);

  // Now check with a valid tag matching that version
  const checkResult = runVersionScript(tempDir, "desktop/v1.2.3-rc.1");
  assert.strictEqual(checkResult.code, 0, "valid tag should pass");

  // Try an invalid version format with --set (no dots)
  const badSetResult = runVersionScript(tempDir, "--set", "invalid");
  assert.notStrictEqual(badSetResult.code, 0, "--set should reject invalid version");
});

test("version.mjs --set requires a valid version", async (t) => {
  const desktopDir = dirname(dirname(fileURLToPath(import.meta.url)));
  const tempDir = copyProject(desktopDir, t);

  // Missing version
  const noVersionResult = runVersionScript(tempDir, "--set");
  assert.notStrictEqual(noVersionResult.code, 0, "--set requires a version argument");

  // Invalid version format
  const badVersionResult = runVersionScript(tempDir, "--set", "not-a-version");
  assert.notStrictEqual(badVersionResult.code, 0, "--set should reject invalid version format");
});

test("version.mjs --set then check succeeds with matching tag", async (t) => {
  const desktopDir = dirname(dirname(fileURLToPath(import.meta.url)));
  const tempDir = copyProject(desktopDir, t);
  const version = "2.0.0";

  // Set version
  const setResult = runVersionScript(tempDir, "--set", version);
  assert.strictEqual(setResult.code, 0, `--set failed: ${setResult.output}`);

  // Check with matching tag (version.mjs extracts version from tag)
  const checkResult = runVersionScript(tempDir, `desktop/v${version}`);
  assert.strictEqual(checkResult.code, 0, `check with tag should pass: ${checkResult.output}`);
});

test("version.mjs detects tag mismatch", async (t) => {
  const desktopDir = dirname(dirname(fileURLToPath(import.meta.url)));
  const tempDir = copyProject(desktopDir, t);
  const version = "3.0.0";

  // Set version
  const setResult = runVersionScript(tempDir, "--set", version);
  assert.strictEqual(setResult.code, 0, `--set failed: ${setResult.output}`);

  // Check with mismatched tag
  const checkResult = runVersionScript(tempDir, "desktop/v1.0.0");
  assert.notStrictEqual(checkResult.code, 0, "check should fail with mismatched tag");
  assert(checkResult.output.includes("1.0.0"), "error should mention tag version");
});
