// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The form's readers (auth, body kind, GraphQL parts) and the edits each
// control asks Go for.

import { describe, expect, it } from "vitest";
import { splitGrpcURL } from "./grpc/grpc-form";
import { authOf, bodyKindOf, countOf, graphqlBody, graphqlParts, lineOf, multilineText, type EntryModel, type ModelRow } from "./model";
import { gotText, joinCheck } from "./tabs/asserts";
import { bodyKindOps } from "./tabs/body/body-tab";
import { automaticHeaders } from "./tabs/headers";
import { optionOps } from "./tabs/options";

const row = (Key: string, Value: string, Disabled = false): ModelRow => ({ Key, Value, Disabled, Range: { Start: 0, End: 0 } });

function entry(over: Partial<EntryModel> & { rows?: Record<string, ModelRow[]> } = {}): EntryModel {
  const { rows, ...rest } = over;
  return {
    Index: 2, Range: { Start: 0, End: 0 }, Method: "GET", MethodRange: { Start: 0, End: 0 }, URL: "https://api.test/x",
    URLRange: { Start: 0, End: 0 }, HasResponse: true, Status: "200", StatusRange: { Start: 0, End: 0 },
    HasBody: false, Body: "", BodyRange: { Start: 0, End: 0 }, Rows: (rows ?? {}) as EntryModel["Rows"], ...rest,
  };
}

describe("form readers", () => {
  it("reads the auth an entry writes", () => {
    expect(authOf(entry({ rows: { headers: [row("Accept", "*/*"), row("Authorization", "Bearer {{token}}")] } }))).toEqual({ kind: "bearer", sec: "headers", index: 1 });
    // A disabled bearer header is no auth.
    expect(authOf(entry({ rows: { headers: [row("Authorization", "Bearer x", true)] } })).kind).toBe("none");
    expect(authOf(entry({ rows: { "basic-auth": [row("{{user}}", "{{password}}")] } })).kind).toBe("basic");
    expect(authOf(entry({ rows: { query: [row("api_key", "{{k}}")] } }))).toEqual({ kind: "apikey", sec: "query", index: 0 });
    expect(authOf(entry({ rows: { options: [row("retry", "1"), row("cert", "a.pem"), row("key", "a.key")] } }))).toEqual({ kind: "cert", sec: "options", rows: [1, 2] });
  });

  it("reads the body kind", () => {
    expect(bodyKindOf(entry())).toBe("none");
    expect(bodyKindOf(entry({ rows: { multipart: [row("f", "file,a.png;")] } }))).toBe("form-data");
    expect(bodyKindOf(entry({ rows: { form: [row("a", "1")] } }))).toBe("urlencoded");
    const body = (Body: string) => bodyKindOf(entry({ HasBody: true, Body }));
    expect(body('{"a": 1}')).toBe("json");
    expect(body("[1]")).toBe("json");
    expect(body("<a/>")).toBe("xml");
    expect(body("file,data.bin;")).toBe("binary");
    expect(body("hex,00ff;")).toBe("binary");
    expect(body("```graphql\nquery { a }\n```")).toBe("graphql");
    expect(body("```\nhello\n```")).toBe("text");
    expect(multilineText("```\nhello\nworld\n```")).toBe("hello\nworld");
    expect(multilineText("`one`")).toBe("one");
  });

  it("splits and joins a GraphQL body", () => {
    const b = "```graphql\nquery Products($first: Int) {\n  products(first: $first) { id }\n}\nvariables {\n  \"first\": 5\n}\n```";
    const { query, variables } = graphqlParts(b);
    expect(query).toBe("query Products($first: Int) {\n  products(first: $first) { id }\n}");
    expect(variables).toBe('{\n  "first": 5\n}');
    expect(graphqlBody(query, variables)).toBe(b);
    expect(graphqlParts("```graphql\n{ a }\n```")).toEqual({ query: "{ a }", variables: "" });
  });

  it("counts enabled rows and finds lines", () => {
    expect(countOf(entry({ rows: { options: [row("a", "1"), row("b", "2", true)] } }), "options")).toBe(1);
    expect(lineOf("a\nb🚀\nc", 6)).toBe(3); // the rocket is two UTF-16 units
  });

  it("splits a gRPC URL", () => {
    expect(splitGrpcURL("http://h:50051/inventory.v1.Inventory/GetStock")).toEqual({ base: "http://h:50051", service: "inventory.v1.Inventory", method: "GetStock" });
    expect(splitGrpcURL("http://h:50051/")).toEqual({ base: "http://h:50051", service: "", method: "" });
  });

  it("predicts the headers Sonde adds, or shows those the run sent", () => {
    const e = entry({ HasBody: true, Body: "{}", rows: { headers: [row("Accept", "application/json")], options: [row("compressed", "true")] } });
    const names = automaticHeaders(e).map((h) => h.name);
    expect(names).toEqual(["Host", "User-Agent", "Content-Type", "Content-Length", "Accept-Encoding"]);
    expect(automaticHeaders(e, [{ name: "accept", value: "x" }, { name: "Host", value: "api.test" }])).toEqual([{ name: "Host", value: "api.test" }]);
  });

  it("explains a failed assert", () => {
    expect(gotText("assert failure\n   |   actual:   string <pending>\n   |   expected: string <paid>")).toBe('got "pending"');
    expect(gotText("actual:   integer <3>")).toBe("got 3");
    expect(gotText("no value\nmore")).toBe("no value");
    expect(joinCheck({ query: ' jsonpath "$.a" ', predicate: "exists", value: "" })).toBe('jsonpath "$.a" exists');
  });
});

describe("the edits controls ask for", () => {
  it("sets, adds and removes an option", () => {
    const e = entry({ rows: { options: [row("retry", "1"), row("max-time", "10s")] } });
    expect(optionOps(e, "max-time", "30s")).toEqual([{ kind: "setRow", entry: 2, section: "options", index: 1, key: "max-time", value: "30s" }]);
    expect(optionOps(e, "delay", "1s")).toEqual([{ kind: "addRow", entry: 2, section: "options", key: "delay", value: "1s" }]);
    expect(optionOps(e, "retry", "")).toEqual([{ kind: "removeRow", entry: 2, section: "options", index: 0 }]);
    expect(optionOps(e, "proxy", "")).toEqual([]);
  });

  it("changes the body kind in one batch", () => {
    const json = entry({ HasBody: true, Body: '{"a": 1}' });
    expect(bodyKindOps(json, "urlencoded")).toEqual([
      { kind: "setBody", entry: 2, value: "" },
      { kind: "addRow", entry: 2, section: "form", key: "field", value: "value" },
    ]);
    const form = entry({ rows: { form: [row("a", "1")] } });
    expect(bodyKindOps(form, "json")).toEqual([
      { kind: "removeSection", entry: 2, section: "form" },
      { kind: "setBody", entry: 2, value: "{\n}" },
    ]);
    expect(bodyKindOps(form, "none")).toEqual([{ kind: "removeSection", entry: 2, section: "form" }]);
    expect(bodyKindOps(json, "json")).toEqual([]);
  });
});

describe("model.ts body readers (tricky cases)", () => {
  it("parses binary bodies correctly", () => {
    expect(bodyKindOf(entry({ HasBody: true, Body: "file,/path/to/data.bin;" }))).toBe("binary");
    expect(bodyKindOf(entry({ HasBody: true, Body: "hex,deadbeef;" }))).toBe("binary");
    expect(bodyKindOf(entry({ HasBody: true, Body: "base64,aGVsbG8=" }))).toBe("binary");
  });

  it("recognizes GraphQL bodies with variables", () => {
    const graphqlWithVars = '```graphql\nquery GetUser($id: ID!) {\n  user(id: $id) { name }\n}\nvariables {\n  "id": "123"\n}\n```';
    expect(bodyKindOf(entry({ HasBody: true, Body: graphqlWithVars }))).toBe("graphql");
  });

  it("recognizes GraphQL without variables", () => {
    const graphqlPlain = '```graphql\n{ hello }\n```';
    expect(bodyKindOf(entry({ HasBody: true, Body: graphqlPlain }))).toBe("graphql");
  });

  it("extracts GraphQL query and variables correctly", () => {
    const body = '```graphql\nquery Q { a }\nvariables {\n  "x": 1\n}\n```';
    const { query, variables } = graphqlParts(body);
    expect(query).toBe("query Q { a }");
    expect(variables).toBe('{\n  "x": 1\n}');
  });

  it("handles GraphQL with no variables block", () => {
    const body = '```graphql\nquery { id name }\n```';
    const { query, variables } = graphqlParts(body);
    expect(query).toBe("query { id name }");
    expect(variables).toBe("");
  });

  it("rebuilds GraphQL body from parts", () => {
    const query = "query GetItems { items { id } }";
    const variables = '{"limit": 10}';
    const rebuilt = graphqlBody(query, variables);
    const parsed = graphqlParts(rebuilt);
    expect(parsed.query).toBe(query);
    expect(parsed.variables).toBe(variables);
  });

  it("recognizes XML bodies", () => {
    expect(bodyKindOf(entry({ HasBody: true, Body: "<?xml version=\"1.0\"?>\n<root></root>" }))).toBe("xml");
    expect(bodyKindOf(entry({ HasBody: true, Body: "<element>content</element>" }))).toBe("xml");
  });

  it("recognizes plain text bodies", () => {
    expect(bodyKindOf(entry({ HasBody: true, Body: "plain text content" }))).toBe("text");
    expect(bodyKindOf(entry({ HasBody: true, Body: "```\nsome text\n```" }))).toBe("text");
  });

  it("handles multiline text extraction", () => {
    const multiline = "```\nline 1\nline 2\nline 3\n```";
    expect(multilineText(multiline)).toBe("line 1\nline 2\nline 3");
  });

  it("handles backtick-quoted single-line text", () => {
    expect(multilineText("`hello world`")).toBe("hello world");
  });

  it("handles text without markers", () => {
    expect(multilineText("raw text")).toBe("raw text");
  });

  it("recognizes form-data and urlencoded from sections", () => {
    const withMultipart = entry({
      rows: { multipart: [row("file", "file,data.bin;")] },
    });
    expect(bodyKindOf(withMultipart)).toBe("form-data");

    const withForm = entry({
      rows: { form: [row("email", "user@example.com")] },
    });
    expect(bodyKindOf(withForm)).toBe("urlencoded");
  });

  it("prioritizes section rows over body content", () => {
    // Even with JSON body, form section takes priority
    const withFormAndJson = entry({
      HasBody: true,
      Body: '{"data": "ignored"}',
      rows: { form: [row("field", "value")] },
    });
    expect(bodyKindOf(withFormAndJson)).toBe("urlencoded");
  });

  it("counts enabled rows correctly", () => {
    const withDisabled = entry({
      rows: {
        headers: [
          row("X-A", "1"),
          row("X-B", "2", true),
          row("X-C", "3"),
        ],
      },
    });
    expect(countOf(withDisabled, "headers")).toBe(2);
  });

  it("computes line numbers from UTF-16 offsets", () => {
    const text = "line 1\nline 2\nline 3";
    // text: "line 1" (0-5) "\n" (6) "line 2" (7-12) "\n" (13) "line 3" (14-19)
    expect(lineOf(text, 0)).toBe(1); // start of line 1
    expect(lineOf(text, 5)).toBe(1); // end of line 1 (before newline)
    expect(lineOf(text, 6)).toBe(1); // newline at position 6 (counted before offset)
    expect(lineOf(text, 7)).toBe(2); // start of line 2 (after first newline)
    expect(lineOf(text, 13)).toBe(2); // second newline at position 13
    expect(lineOf(text, 14)).toBe(3); // start of line 3 (after second newline)
  });

  it("handles emoji correctly in line counting", () => {
    // Emoji takes 2 UTF-16 units but is 1 char
    const text = "# 🚀 comment\nGET /api";
    expect(lineOf(text, text.indexOf("\n") + 1)).toBe(2);
  });
});

describe("review fixes", () => {
  it("sets an option on its live row, re-enables a disabled one", async () => {
    const { optionOps, versionOps } = await import("./tabs/options");
    const e = entry({ rows: { options: [row("max-time", "5s", true), row("retry", "1"), row("http1.1", "true"), row("http2", "true", true)] } });
    expect(optionOps(e, "max-time", "30s")).toEqual([
      { kind: "toggleRow", entry: 2, section: "options", index: 0 },
      { kind: "setRow", entry: 2, section: "options", index: 0, key: "max-time", value: "30s" },
    ]);
    // Clearing leaves a disabled row alone.
    expect(optionOps(e, "max-time", "")).toEqual([]);
    // HTTP 2: the live 1.1 row goes, then the disabled http2 row (index 3,
    // 2 once 1.1 is gone) is enabled and set.
    expect(versionOps(e, "http2")).toEqual([
      { kind: "removeRow", entry: 2, section: "options", index: 2 },
      { kind: "toggleRow", entry: 2, section: "options", index: 2 },
      { kind: "setRow", entry: 2, section: "options", index: 2, key: "http2", value: "true" },
    ]);
    expect(versionOps(e, "")).toEqual([{ kind: "removeRow", entry: 2, section: "options", index: 2 }]);
  });

  it("writes a predicate with or without its value", async () => {
    const { joinCheck, valueless } = await import("./tabs/asserts");
    expect(valueless("exists")).toBe(true);
    expect(valueless("not isString")).toBe(true);
    expect(valueless("==")).toBe(false);
    expect(joinCheck({ query: 'jsonpath "$.a"', predicate: "exists", value: "1" })).toBe('jsonpath "$.a" exists');
    expect(joinCheck({ query: 'jsonpath "$.a"', predicate: "==", value: "" })).toBe('jsonpath "$.a" == ""');
  });

  it("drops a Content-Type that described the old body kind", () => {
    const e = entry({ HasBody: true, Body: "{}", rows: { headers: [row("Accept", "*/*"), row("Content-Type", "application/json; charset=utf-8")] } });
    expect(bodyKindOps(e, "urlencoded")[0]).toEqual({ kind: "removeRow", entry: 2, section: "headers", index: 1 });
    const own = entry({ HasBody: true, Body: "{}", rows: { headers: [row("Content-Type", "application/vnd.custom")] } });
    expect(bodyKindOps(own, "xml").some((o) => o.section === "headers")).toBe(false);
  });

  it("reads escaped file names; a tagged block is not text", async () => {
    const { fileRef } = await import("./model");
    expect(fileRef("file,My\\ Photo\\;1.png; image/png")).toEqual({ path: "My Photo;1.png", type: "image/png" });
    expect(fileRef("file,a.txt;")).toEqual({ path: "a.txt", type: "" });
    expect(fileRef("plain")).toBeNull();
    expect(bodyKindOf(entry({ HasBody: true, Body: '```json\n{"a": 1}\n```' }))).toBe("other");
  });
});
