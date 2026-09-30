// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

// Every file the Go parser accepts parses without an error node, bodies
// included (JSON with templates, XML, GraphQL): the .hurl
// and .sonde files under testdata/, internal/conformance and docs/, and
// the ```hurl / ```sonde examples in docs/*.md.

import { execFileSync } from "node:child_process";
import { mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { afterAll, describe, expect, it } from "vitest";
import { sondeFileLanguage } from "./language";

const root = fileURLToPath(new URL("../../../../", import.meta.url));

function walk(dir: string, match = /\.(hurl|sonde)$/, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, match, out);
    else if (match.test(name)) out.push(p);
  }
  return out;
}

/** The docs' fenced examples, written out as files. */
function docExamples(dir: string): string[] {
  const out: string[] = [];
  for (const md of walk(join(root, "docs"), /\.md$/)) {
    const text = readFileSync(md, "utf8");
    for (const m of text.matchAll(/```(hurl|sonde)\n([\s\S]*?)```/g)) {
      const file = join(dir, `${relative(root, md).replace(/[/\\]/g, "_")}-${out.length}.${m[1]}`);
      writeFileSync(file, m[2]);
      out.push(file);
    }
  }
  return out;
}

/** The files the Go parser accepts (`sonde check`, built once). */
function accepted(files: string[], dir: string): string[] {
  const sonde = join(dir, process.platform === "win32" ? "sonde.exe" : "sonde");
  try {
    execFileSync("go", ["build", "-o", sonde, "./cmd/sonde"], { cwd: root, stdio: "pipe" });
  } catch (err) {
    throw new Error(`building the sonde CLI: ${(err as { stderr?: Buffer }).stderr?.toString() ?? err}`, { cause: err });
  }
  let report = "";
  try {
    execFileSync(sonde, ["check", ...files], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
  } catch (err) {
    const e = err as { status?: number; stdout?: string; stderr?: string };
    if (e.status !== 2) throw err;
    report = `${e.stdout ?? ""}${e.stderr ?? ""}`;
  }
  const invalid = new Set([...report.matchAll(/^\s*--> (.+):\d+:\d+$/gm)].map((m) => m[1]));
  return files.filter((f) => !invalid.has(f));
}

/** Error nodes in the file and in its bodies' own parses (JSON, XML…). */
function errorNodes(src: string): number {
  let n = 0;
  sondeFileLanguage.parser.parse(src).iterate({ enter: (node) => void (node.type.isError && n++) });
  return n;
}

describe("corpus", () => {
  const tmp = mkdtempSync(join(tmpdir(), "sonde-corpus-"));
  afterAll(() => rmSync(tmp, { recursive: true, force: true }));
  const files = [
    ...walk(join(root, "testdata")),
    ...walk(join(root, "internal", "conformance")),
    ...walk(join(root, "docs")),
    ...docExamples(tmp),
  ];
  const ok = accepted(files, tmp);
  console.log(`corpus: ${ok.length} of ${files.length} files accepted`);

  it("has the accepted files", () => {
    expect(ok.length).toBeGreaterThan(250);
    // A template as a JSON value (`"age": {{age}}`, line 23).
    expect(ok).toContain(join(root, "testdata/conformance/hurl/tests_ok/variables/variables.hurl"));
  });

  it("parses every accepted file without an error node", () => {
    const bad = ok
      .map((f) => ({ f: relative(root, f), n: errorNodes(readFileSync(f, "utf8")) }))
      .filter((x) => x.n > 0);
    expect(bad, bad.map((x) => `${x.f}: ${x.n}`).join("\n")).toEqual([]);
  });
});
