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

function runCheckNoHarness(...files) {
  const script = fileURLToPath(new URL("./check-no-harness.mjs", import.meta.url));
  const cmd = `node ${script} ${files.join(" ")}`;
  try {
    const output = execSync(cmd, { encoding: "utf8", stdio: "pipe" });
    return { code: 0, output };
  } catch (e) {
    return { code: e.status, output: e.stderr || e.stdout || "" };
  }
}

function createBinaryFile(content) {
  const tempDir = join(tmpBase, `harness-test-${Date.now()}-${Math.random().toString(36).slice(2)}`);
  mkdirSync(tempDir, { recursive: true });
  const filePath = join(tempDir, "binary");
  writeFileSync(filePath, content);
  return filePath;
}

test("check-no-harness passes on clean binary", async (t) => {
  const filePath = createBinaryFile("This is a clean binary with no harness markers");
  const result = runCheckNoHarness(filePath);
  assert.strictEqual(result.code, 0, `should pass clean binary: ${result.output}`);
  assert(result.output.includes("no harness"), "output should confirm no harness found");
});

test("check-no-harness detects e2eharness marker", async (t) => {
  const filePath = createBinaryFile("Some binary content\x00e2eharness\x00more content");
  const result = runCheckNoHarness(filePath);
  assert.notStrictEqual(result.code, 0, "should fail when e2eharness marker found");
  assert(result.output.includes("e2eharness"), "error should mention e2eharness");
  assert(result.output.includes("harness build"), "error should explain it's a harness build");
});

test("check-no-harness detects harness_service.go marker", async (t) => {
  const filePath = createBinaryFile("Binary with harness_service.go in it");
  const result = runCheckNoHarness(filePath);
  assert.notStrictEqual(result.code, 0, "should fail when harness_service.go marker found");
  assert(result.output.includes("harness_service.go"), "error should mention harness_service.go");
});

test("check-no-harness detects main_harness.go marker", async (t) => {
  const filePath = createBinaryFile("Binary with main_harness.go in it");
  const result = runCheckNoHarness(filePath);
  assert.notStrictEqual(result.code, 0, "should fail when main_harness.go marker found");
  assert(result.output.includes("main_harness.go"), "error should mention main_harness.go");
});

test("check-no-harness fails when no files provided", async (t) => {
  const result = runCheckNoHarness();
  assert.notStrictEqual(result.code, 0, "should fail with no files");
  assert(result.output.includes("usage"), "error should show usage message");
});

test("check-no-harness checks multiple files", async (t) => {
  const file1 = createBinaryFile("Clean binary 1");
  const file2 = createBinaryFile("Clean binary 2");
  const result = runCheckNoHarness(file1, file2);
  assert.strictEqual(result.code, 0, `should pass multiple clean files: ${result.output}`);
  assert(result.output.includes(file1), "output should mention first file");
  assert(result.output.includes(file2), "output should mention second file");
});

test("check-no-harness fails if any file has marker", async (t) => {
  const clean = createBinaryFile("Clean binary");
  const contaminated = createBinaryFile("e2eharness contaminated binary");
  const result = runCheckNoHarness(clean, contaminated);
  assert.notStrictEqual(result.code, 0, "should fail if any file has harness marker");
  assert(result.output.includes(contaminated), "error should identify the bad file");
});
