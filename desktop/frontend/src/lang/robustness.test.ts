// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

// Stress testing: fuzzed inputs, random documents, incremental reparse
// consistency. The parser must never crash, hang, or take >5s on any input.

import { TreeFragment, type Tree } from "@lezer/common";
import { describe, expect, it } from "vitest";
import { sondeFileLanguage } from "./language";

const parser = sondeFileLanguage.parser;

/** Seeded pseudo-random number generator (xorshift32) for determinism. */
class Rng {
  private x: number;
  constructor(seed: number) {
    this.x = seed || 1;
  }
  next(): number {
    let x = this.x;
    x ^= x << 13;
    x ^= x >> 17;
    x ^= x << 5;
    this.x = x >>> 0; // Ensure 32-bit unsigned
    return (this.x >>> 0) / 0x100000000;
  }
  nextInt(max: number): number {
    return Math.floor(this.next() * max);
  }
}

/** Fragments of request-file syntax. */
const fragments = {
  methods: ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"],
  urls: [
    "http://localhost:8080/api",
    "https://example.com/users",
    "ws://stream.local:9000",
    "{{base}}/v1/data",
  ],
  headers: ["Content-Type: application/json", "Authorization: Bearer {{token}}", "X-Custom: {{value}}"],
  bodies: [
    '{"name": "{{name}}", "age": {{age}}}',
    '{"items": [1, {{count}}, 3]}',
    '<?xml version="1.0"?><root><item>{{val}}</item></root>',
    "```graphql\nquery { me { name } }\n```",
  ],
  sections: ["[Captures]", "[Asserts]", "[Options]", "[SondeMessages]", "[SondeGrpc]"],
  captures: [
    'token: jsonpath "$.token"',
    'status: jsonpath "$.status" redact',
    'id: xpath "/root/@id"',
  ],
  asserts: [
    'status == 200',
    'jsonpath "$.name" exists',
    'body contains "success"',
    'header "Content-Type" matches /json/',
  ],
};

/** Builds a random but valid request file. */
function randomDocument(rng: Rng, numEntries: number): string {
  const entries: string[] = [];
  for (let i = 0; i < numEntries; i++) {
    const method = fragments.methods[rng.nextInt(fragments.methods.length)];
    const url = fragments.urls[rng.nextInt(fragments.urls.length)];
    let entry = `${method} ${url}\n`;

    // Random headers.
    for (let h = 0; h < rng.nextInt(3); h++) {
      entry += fragments.headers[rng.nextInt(fragments.headers.length)] + "\n";
    }

    // Sometimes a body.
    if (rng.next() < 0.4) {
      entry += fragments.bodies[rng.nextInt(fragments.bodies.length)] + "\n";
    }

    // Response line.
    entry += `HTTP 200\n`;

    // Sometimes sections.
    if (rng.next() < 0.5) {
      entry += fragments.sections[rng.nextInt(fragments.sections.length)] + "\n";
      for (let c = 0; c < rng.nextInt(3); c++) {
        entry += fragments.captures[rng.nextInt(fragments.captures.length)] + "\n";
      }
    }

    entry += "\n";
    entries.push(entry);
  }
  return entries.join("");
}

/** Edge cases: unbalanced delimiters, CRLF, long lines, empty file, etc. */
const edgeCases = [
  "", // empty
  "\n\n\n", // just newlines
  "GET http://x\n", // minimal
  "GET http://x\n{{{", // unbalanced braces
  "GET http://x\n[[[", // unbalanced brackets
  "GET http://x\n\"\"\"", // unbalanced quotes
  "GET http://x\n```````", // unbalanced backticks
  'GET http://x\n{{\n}}\n', // CRLF-like with templates
  `GET http://x\n${"x".repeat(100000)}\n`, // very long line
  'POST http://x\n{"a":1}\nGET http://y\n{broken json', // mixed valid/invalid
  "[Captures]\n" + "x: y\n".repeat(1000), // many lines in section
  "{{" + "x".repeat(500) + "}}", // long template
  '```\n' + "a\nb\nc".repeat(100) + "\n```", // long multiline string
];

describe("robustness: fuzzed inputs", () => {
  it("parses 2000 random documents without crashing (bounded time)", () => {
    const rng = new Rng(0x12345678);
    const times: number[] = [];
    let maxTime = 0;

    for (let i = 0; i < 2000; i++) {
      const doc = randomDocument(rng, rng.nextInt(5) + 1);
      const start = performance.now();
      try {
        parser.parse(doc);
      } catch (err) {
        throw new Error(`Parse crashed on doc ${i}: ${err}`, { cause: err });
      }
      const time = performance.now() - start;
      times.push(time);
      maxTime = Math.max(maxTime, time);
      expect(time).toBeLessThan(1000); // 1 second per document
    }
    times.sort((a, b) => a - b);
    const p95 = times[Math.floor(times.length * 0.95)];
    console.log(`random docs: ${times.length}, max ${maxTime.toFixed(2)}ms, p95 ${p95.toFixed(2)}ms`);
  });

  it("handles edge cases without crashing", () => {
    for (const src of edgeCases) {
      const start = performance.now();
      try {
        parser.parse(src);
      } catch (err) {
        throw new Error(`Parse crashed on edge case: ${err}\nInput length: ${src.length}`, { cause: err });
      }
      const time = performance.now() - start;
      expect(time).toBeLessThan(1000);
    }
  });

  it("parses a 5 MB JSON body without hanging", () => {
    const large = '{"data": "' + "x".repeat(5 * 1024 * 1024) + '"}';
    const src = `POST http://x\n${large}\nHTTP 200\n`;
    const start = performance.now();
    parser.parse(src);
    const time = performance.now() - start;
    expect(time).toBeLessThan(5000); // 5 seconds
    console.log(`5 MB JSON body: ${time.toFixed(2)}ms`);
  });

  it("handles files ending without newline", () => {
    const src = "GET http://x\nX-Header: value"; // no final newline
    const result = parser.parse(src);
    let errors = 0;
    result.iterate({ enter: (n) => void (n.type.isError && errors++) });
    expect(errors).toBe(0);
  });
});

describe("robustness: incremental reparse", () => {
  it("incremental reparse handles 300 random edits without crashing", () => {
    const rng = new Rng(0xdeadbeef);
    let doc = randomDocument(rng, 50);
    let tree: Tree = parser.parse(doc);
    let fragments = TreeFragment.addTree(tree);

    for (let edit = 0; edit < 300; edit++) {
      // Random edit position.
      const pos = rng.nextInt(Math.max(1, doc.length - 1));
      const char = String.fromCharCode(32 + rng.nextInt(94)); // printable ASCII

      // Apply edit.
      doc = doc.slice(0, pos) + char + doc.slice(pos + 1);

      // Incremental parse with TreeFragment - should not crash.
      try {
        fragments = TreeFragment.applyChanges(fragments, [{ fromA: pos, toA: pos + 1, fromB: pos, toB: pos + 1 }]);
        tree = parser.parse(doc, fragments);
        fragments = TreeFragment.addTree(tree, fragments);
      } catch (err) {
        throw new Error(`Incremental reparse crashed at edit ${edit}: ${err}`, { cause: err });
      }
    }
    console.log("incremental reparse: 300 edits handled without crash");
  });

  it("incremental reparse on corpus file (variables.hurl) without crashing", () => {
    // This is line 23 of the corpus file, which has a template as a JSON value.
    const src = `GET http://x
[Captures]
id: jsonpath "$.id"
POST http://x
{"age": {{age}}}
HTTP 200
`;

    let tree = parser.parse(src);
    let fragments = TreeFragment.addTree(tree);
    const rng = new Rng(0xabcd1234);

    for (let i = 0; i < 50; i++) {
      // Random edit in the JSON body (line 5).
      const pos = src.indexOf('{"age"');
      const editPos = pos + rng.nextInt(20);
      const newChar = String.fromCharCode(32 + rng.nextInt(94));

      const newSrc = src.slice(0, editPos) + newChar + src.slice(editPos + 1);

      try {
        fragments = TreeFragment.applyChanges(fragments, [{ fromA: editPos, toA: editPos + 1, fromB: editPos, toB: editPos + 1 }]);
        tree = parser.parse(newSrc, fragments);
        fragments = TreeFragment.addTree(tree, fragments);
      } catch (err) {
        throw new Error(`Incremental reparse on variables.hurl crashed: ${err}`, { cause: err });
      }
    }
    console.log("corpus incremental reparse: variables.hurl handled without crash");
  });
});

describe("robustness: template handling", () => {
  it("templates in various contexts without errors", () => {
    const cases = [
      'GET {{url}}', // in URL
      'GET http://x\nX-Id: {{id}}', // in header value
      'GET http://x\nX-Key-{{var}}: value', // in header name
      'GET http://x\n{"key": {{val}}}', // in JSON value
      'GET http://x\n{"{{k}}": 1}', // in JSON key
      'GET http://x\n["a {{b}} c"]', // in JSON string
      'GET http://x\n[Captures]\nx: jsonpath "$.{{path}}"', // in assertion
      "GET http://x\nbody contains `text {{var}} text`", // in backtick
    ];

    for (const src of cases) {
      let errors = 0;
      parser.parse(src).iterate({ enter: (n) => void (n.type.isError && errors++) });
      expect(errors, src).toBe(0);
    }
  });
});
