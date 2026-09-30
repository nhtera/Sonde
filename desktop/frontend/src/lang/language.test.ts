// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment node

import { foldNodeProp } from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { IterMode, type SyntaxNode, type Tree } from "@lezer/common";
import { describe, expect, it } from "vitest";
import { jsonBodyLanguage, sondeFileLanguage } from "./language";

const parse = (src: string) => sondeFileLanguage.parser.parse(src);

/** Each node named name, as its text. outer: the file grammar's nodes
 * only (a body's node is otherwise seen as its own parse's root). */
function texts(tree: Tree, src: string, name: string, outer = false): string[] {
  const out: string[] = [];
  tree.iterate({ mode: outer ? IterMode.IgnoreMounts : undefined, enter: (n) => void (n.name === name && out.push(src.slice(n.from, n.to))) });
  return out;
}

function errors(tree: Tree): number {
  let n = 0;
  tree.iterate({ enter: (x) => void (x.type.isError && n++) });
  return n;
}

describe("file grammar", () => {
  const src = [
    "# Log in",
    "POST {{base_url}}/login?next=/home # the form",
    "Content-Type: application/json",
    "{{name}}-Id: 7",
    '{"user": "{{user}}", "n": [1, {{n}}]}',
    "HTTP/1.1 200",
    "[Captures]",
    'token: jsonpath "$.token" redact',
    "[Asserts]",
    'jsonpath "$.name" matches /^a\\/b$/',
    'header "Location" == "{{base_url}}/x"',
    "body contains `a {{b}}`",
    "",
    "GET {{base_url}}/users/{{id}}",
    "[Options]",
    "retry: 3",
    "file,data.bin;",
  ].join("\n");
  const tree = parse(src);

  it("parses without errors", () => expect(errors(tree)).toBe(0));

  it("splits entries, request and response, and sections", () => {
    expect(texts(tree, src, "Method")).toEqual(["POST", "GET"]);
    expect(texts(tree, src, "HttpVersion")).toEqual(["HTTP/1.1"]);
    expect(texts(tree, src, "SectionHeader")).toEqual(["[Captures]", "[Asserts]", "[Options]"]);
    expect(texts(tree, src, "Entry")).toHaveLength(2);
    expect(texts(tree, src, "Url")).toEqual(["{{base_url}}/login?next=/home", "{{base_url}}/users/{{id}}"]);
    expect(texts(tree, src, "UrlTemplate")).toEqual(["{{base_url}}", "{{base_url}}", "{{id}}"]);
  });

  it("finds keys, keywords, strings, templates and regexes", () => {
    expect(texts(tree, src, "Key")).toEqual(["Content-Type", "{{name}}-Id", "token", "retry"]);
    expect(texts(tree, src, "QueryName")).toEqual(["jsonpath", "jsonpath", "header", "body"]);
    expect(texts(tree, src, "Predicate")).toEqual(["matches", "contains"]);
    expect(texts(tree, src, "Modifier")).toEqual(["redact"]);
    expect(texts(tree, src, "Regex")).toEqual(["/^a\\/b$/"]);
    // {{user}} is inside the JSON body's string.
    expect(texts(tree, src, "StringTemplate")).toEqual(["{{user}}", "{{base_url}}"]);
    expect(texts(tree, src, "BacktickTemplate")).toEqual(["{{b}}"]);
    expect(texts(tree, src, "Comment")).toEqual(["# Log in", "# the form"]);
    expect(texts(tree, src, "LiteralBody")).toEqual(["file,data.bin;"]);
  });

  it("parses a JSON body with templates as values and in strings", () => {
    expect(texts(tree, src, "JsonText")).toHaveLength(1);
    expect(texts(tree, src, "Template")).toContain("{{n}}");
    expect(texts(tree, src, "PropertyName")).toEqual(['"user"', '"n"']);
  });

  it("keeps a URL with an address and a port whole", () => {
    const s = "GET http://localhost:8080/a?b=c#frag\nHTTP 200";
    expect(texts(parse(s), s, "UrlText")).toEqual(["http://localhost:8080/a?b=c#frag"]);
  });

  it("reads a line without a newline at the end", () => {
    const s = "GET http://x\nHTTP 200";
    const t = parse(s);
    expect(errors(t)).toBe(0);
    expect(texts(t, s, "ResponseLine")).toEqual(["HTTP 200"]);
  });

  it("names a variable called like a keyword a variable", () => {
    const s = 'GET http://x\n[Asserts]\nurl == "{{url}}"\nstatus == {{status}}\n';
    const t = parse(s);
    expect(texts(t, s, "VariableName")).toEqual(["url", "status"]);
    expect(texts(t, s, "FunctionName")).toEqual([]);
    const f = "GET http://x/{{newUuid}}\nX-Id: {{ newUuid }}\n";
    expect(texts(parse(f), f, "FunctionName")).toEqual(["newUuid"]);
  });

  it("keeps an unclosed quote on its line", () => {
    const s = 'GET http://x\nX-Size: 5" tall\nHTTP 200\n';
    const t = parse(s);
    expect(errors(t)).toBe(0);
    expect(texts(t, s, "HttpVersion")).toEqual(["HTTP"]);
  });

  it("never errs on an unclosed template while typing", () => {
    for (const s of ["GET http://x\nX-A: {{foo\n", "GET http://x\n{{foo: 1\n", 'GET http://x\n[Asserts]\njsonpath "$.a" == {{\n', 'GET http://x\nX-A: "a {{b"\n', "GET http://x\nX-A: `a {{b`\n"]) {
      expect(errors(parse(s)), s).toBe(0);
    }
    const s = 'GET http://x\nX-A: "a {{b"\n';
    expect(texts(parse(s), s, "StringTemplate")).toEqual([]);
  });

  it("keeps a URL with spaced templates whole", () => {
    const s = "GET {{ base_url }}/users/{{ id }}?a=b # c\nHTTP 200";
    const t = parse(s);
    expect(texts(t, s, "Url")).toEqual(["{{ base_url }}/users/{{ id }}?a=b"]);
    expect(texts(t, s, "Comment")).toEqual(["# c"]);
  });

  it("reads CRLF files", () => {
    const s = "GET http://x\r\nX-A: b\r\n[Asserts]\r\nstatus == 200\r\n";
    const t = parse(s);
    expect(errors(t)).toBe(0);
    expect(texts(t, s, "Key")).toEqual(["X-A"]);
    expect(texts(t, s, "SectionHeader")).toEqual(["[Asserts]"]);
  });

  it("never joins two lines at a backslash", () => {
    const s = "GET http://x\nabc\\\nHost: y\n";
    expect(texts(parse(s), s, "Key")).toEqual(["Host"]);
  });

  it("reads [true] and [null] as JSON bodies, [Asserts] as a section", () => {
    const s = "POST http://x\n[true]\nHTTP 200\n[Asserts]\n";
    const t = parse(s);
    expect(texts(t, s, "JsonBody", true)).toEqual(["[true]"]);
    expect(texts(t, s, "SectionHeader")).toEqual(["[Asserts]"]);
  });

  it("does not let a broken body swallow the next request", () => {
    const s = 'POST http://x\n{\n  "a": 1\nHTTP 200\n\nGET http://y\n<a>\nHTTP 200\n';
    const t = parse(s);
    expect(texts(t, s, "Method")).toEqual(["POST", "GET"]);
    expect(texts(t, s, "HttpVersion")).toEqual(["HTTP", "HTTP"]);
  });

  it("reads bodies that span lines: JSON, XML, fenced strings", () => {
    const s = [
      "POST http://x",
      "{",
      '  "a": {{a}},',
      '  "b": "}{"',
      "}",
      "HTTP 200",
      "",
      "POST http://x",
      '<?xml version="1.0"?>',
      '<a x="1>2"><b/><!-- c --><![CDATA[<d>]]></a>',
      "HTTP 200",
      "",
      "POST http://x",
      "```graphql",
      "query { me { name } }",
      "```",
      "HTTP 200",
    ].join("\n");
    const t = parse(s);
    expect(errors(t)).toBe(0);
    expect(texts(t, s, "JsonBody", true)).toEqual(['{\n  "a": {{a}},\n  "b": "}{"\n}']);
    expect(texts(t, s, "XmlBody", true)[0]).toMatch(/^<\?xml[\s\S]*<\/a>$/);
    expect(texts(t, s, "MultilineString", true)).toEqual(["```graphql\nquery { me { name } }\n```"]);
    expect(texts(t, s, "HttpVersion")).toHaveLength(3);
  });

  it("reads a .sonde message step's JSON across lines", () => {
    const s = 'GET ws://x\n[SondeMessages]\nsend: {\n  "op": "sub"\n}\nreceive: 1\n';
    const t = parse(s);
    expect(errors(t)).toBe(0);
    expect(texts(t, s, "JsonBody", true)).toEqual(['{\n  "op": "sub"\n}']);
    expect(texts(t, s, "Key")).toEqual(["send", "receive"]);
  });
});

describe("JSON body grammar", () => {
  const json = (s: string) => jsonBodyLanguage.parser.parse(s);

  it("takes templates as values, keys' strings and inside strings", () => {
    for (const s of ['{"age": {{age}}}', '{"{{k}}": 1}', '["a {{b}} c", {{d}}, [{{e}}]]', "{{whole}}", '"x{{y}}"']) {
      expect(errors(json(s)), s).toBe(0);
    }
  });

  it("keeps a big integer one number token", () => {
    const s = '{"id": 123456789012345678901234567890}';
    expect(texts(json(s), s, "Number")).toEqual(["123456789012345678901234567890"]);
  });

  it("marks broken JSON", () => {
    expect(errors(json('{"a": }'))).toBeGreaterThan(0);
  });
});

describe("folding", () => {
  const src = "GET http://x\nA: b\n[Asserts]\nstatus == 200\nbody exists\n\nPOST http://y\n{\n  \"a\": 1\n}\n";
  const state = EditorState.create({ doc: src, extensions: sondeFileLanguage });

  /** The fold ranges of the nodes named name. */
  function folds(name: string) {
    const out: { from: number; to: number }[] = [];
    parse(src).iterate({
      enter: (n) => {
        if (n.name !== name) return;
        const fold = n.type.prop(foldNodeProp);
        const r = fold?.(n.node as SyntaxNode, state);
        if (r) out.push(r);
      },
    });
    return out.map((r) => src.slice(r.from, r.to));
  }

  it("folds an entry after its request line", () => {
    expect(folds("Entry")).toEqual(["\nA: b\n[Asserts]\nstatus == 200\nbody exists", '\n{\n  "a": 1\n}']);
  });

  it("leaves the next request's comment out of a fold", () => {
    const s = "GET a\nHTTP 200\n[Asserts]\nstatus == 200\n\n# Create user\nPOST b\n";
    const st = EditorState.create({ doc: s, extensions: sondeFileLanguage });
    const got: string[] = [];
    parse(s).iterate({
      enter: (n) => {
        const r = (n.name === "Entry" || n.name === "Section") && n.type.prop(foldNodeProp)?.(n.node as SyntaxNode, st);
        if (r) got.push(s.slice(r.from, r.to));
      },
    });
    expect(got).toEqual(["\nHTTP 200\n[Asserts]\nstatus == 200", "\nstatus == 200"]);
  });

  it("folds a section after its header", () => {
    expect(folds("Section")).toEqual(["\nstatus == 200\nbody exists"]);
  });

  it("folds a JSON body inside its braces (through its own parse)", () => {
    expect(folds("Object")).toEqual(['\n  "a": 1\n']);
  });
});
