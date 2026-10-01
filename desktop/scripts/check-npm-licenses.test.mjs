// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { fileURLToPath } from "node:url";
import { writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execSync } from "node:child_process";
import { test } from "node:test";
import assert from "node:assert";

const tmpBase = tmpdir();

function createTempDir() {
  return join(tmpBase, `npm-licenses-test-${Date.now()}-${Math.random().toString(36).slice(2)}`);
}

function runCheckLicenses(tempDir) {
  const script = fileURLToPath(new URL("./check-npm-licenses.mjs", import.meta.url));
  const cmd = `node ${script}`;
  try {
    const output = execSync(cmd, { cwd: tempDir, encoding: "utf8", stdio: "pipe" });
    return { code: 0, output };
  } catch (e) {
    return { code: e.status, output: e.stderr || e.stdout || "" };
  }
}

test("check-npm-licenses passes with allowed licenses", async (t) => {
  const tempDir = createTempDir();
  mkdirSync(tempDir, { recursive: true });
  mkdirSync(join(tempDir, "frontend"), { recursive: true });

  const lockfile = {
    packages: {
      "": { version: "1.0.0" },
      "node_modules/dep1": { license: "MIT", dev: false },
      "node_modules/dep2": { license: "Apache-2.0", dev: false },
      "node_modules/dep3": { license: "BSD-3-Clause", dev: false },
    },
  };

  writeFileSync(join(tempDir, "frontend", "package-lock.json"), JSON.stringify(lockfile));

  const result = runCheckLicenses(tempDir);
  assert.strictEqual(result.code, 0, `should pass with allowed licenses: ${result.output}`);
  assert(result.output.includes("npm licenses ok"), "output should confirm licenses ok");
  assert(result.output.includes("3 shipped packages"), "output should count shipped packages");
});

test("check-npm-licenses fails with disallowed license", async (t) => {
  const tempDir = createTempDir();
  mkdirSync(join(tempDir, "frontend"), { recursive: true });

  const lockfile = {
    packages: {
      "": { version: "1.0.0" },
      "node_modules/dep1": { license: "MIT", dev: false },
      "node_modules/bad-dep": { license: "AGPL-3.0", dev: false },
    },
  };

  writeFileSync(join(tempDir, "frontend", "package-lock.json"), JSON.stringify(lockfile));

  const result = runCheckLicenses(tempDir);
  assert.notStrictEqual(result.code, 0, "should fail with disallowed license");
  assert(result.output.includes("AGPL-3.0"), "error should mention disallowed license");
  assert(result.output.includes("not allowed"), "error should indicate license is not allowed");
});

test("check-npm-licenses ignores dev dependencies", async (t) => {
  const tempDir = createTempDir();
  mkdirSync(join(tempDir, "frontend"), { recursive: true });

  const lockfile = {
    packages: {
      "": { version: "1.0.0" },
      "node_modules/dep1": { license: "MIT", dev: false },
      "node_modules/dev-dep": { license: "AGPL-3.0", dev: true }, // dev dep with bad license
    },
  };

  writeFileSync(join(tempDir, "frontend", "package-lock.json"), JSON.stringify(lockfile));

  const result = runCheckLicenses(tempDir);
  assert.strictEqual(result.code, 0, `should ignore dev dependencies: ${result.output}`);
  assert(result.output.includes("npm licenses ok"), "should pass when only dev deps have bad licenses");
});

test("check-npm-licenses handles missing license field", async (t) => {
  const tempDir = createTempDir();
  mkdirSync(join(tempDir, "frontend"), { recursive: true });

  const lockfile = {
    packages: {
      "": { version: "1.0.0" },
      "node_modules/unlicensed": { dev: false }, // no license field
    },
  };

  writeFileSync(join(tempDir, "frontend", "package-lock.json"), JSON.stringify(lockfile));

  const result = runCheckLicenses(tempDir);
  assert.notStrictEqual(result.code, 0, "should fail with missing license");
  assert(result.output.includes("not stated"), "error should indicate license is not stated");
});

test("check-npm-licenses allows OFL-1.1 for fonts", async (t) => {
  const tempDir = createTempDir();
  mkdirSync(join(tempDir, "frontend"), { recursive: true });

  const lockfile = {
    packages: {
      "": { version: "1.0.0" },
      "node_modules/font-dep": { license: "OFL-1.1", dev: false },
    },
  };

  writeFileSync(join(tempDir, "frontend", "package-lock.json"), JSON.stringify(lockfile));

  const result = runCheckLicenses(tempDir);
  assert.strictEqual(result.code, 0, `OFL-1.1 should be allowed for fonts: ${result.output}`);
});

test("check-npm-licenses counts only shipped packages", async (t) => {
  const tempDir = createTempDir();
  mkdirSync(join(tempDir, "frontend"), { recursive: true });

  const lockfile = {
    packages: {
      "": { version: "1.0.0" }, // root package, skipped
      "node_modules/shipped1": { license: "MIT", dev: false },
      "node_modules/shipped2": { license: "MIT", dev: false },
      "node_modules/dev-only": { license: "MIT", dev: true }, // dev, skipped
    },
  };

  writeFileSync(join(tempDir, "frontend", "package-lock.json"), JSON.stringify(lockfile));

  const result = runCheckLicenses(tempDir);
  assert.strictEqual(result.code, 0, `should count only shipped packages: ${result.output}`);
  assert(result.output.includes("2 shipped packages"), "should count 2 shipped packages (ignoring root and dev)");
});

test("check-npm-licenses accepts all standard permissive licenses", async (t) => {
  const tempDir = createTempDir();
  mkdirSync(join(tempDir, "frontend"), { recursive: true });

  const licenses = ["MIT", "ISC", "0BSD", "BSD-2-Clause", "BSD-3-Clause", "Apache-2.0"];
  const packages = { "": { version: "1.0.0" } };

  licenses.forEach((license, i) => {
    packages[`node_modules/dep${i}`] = { license, dev: false };
  });

  const lockfile = { packages };
  writeFileSync(join(tempDir, "frontend", "package-lock.json"), JSON.stringify(lockfile));

  const result = runCheckLicenses(tempDir);
  assert.strictEqual(result.code, 0, `all permissive licenses should be allowed: ${result.output}`);
});
