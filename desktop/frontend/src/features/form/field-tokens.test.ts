// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { tokens } from "./field-tokens";

const kinds = (t: string) => tokens(t).map((x) => `${x.kind}:${x.text}`);

describe("field tokens", () => {
  it("colors a query, a predicate and values", () => {
    expect(kinds('jsonpath "$.status"')).toEqual(["query:jsonpath", "plain: ", 'str:"$.status"']);
    expect(kinds("==")).toEqual(["op:=="]);
    expect(kinds("25.8")).toEqual(["num:25.8"]);
    expect(kinds("true")).toEqual(["num:true"]);
  });

  it("keeps {{variables}} apart, in strings too, and while typed", () => {
    expect(kinds("{{base_url}}/orders/{{id}}")).toEqual(["var:{{base_url}}", "plain:/orders/", "var:{{id}}"]);
    expect(kinds('"{{ca')).toEqual(['str:"', "var:{{ca"]);
    expect(kinds("Bearer {{token}}")).toEqual(["plain:Bearer ", "var:{{token}}"]);
  });

  it("gives back the text", () => {
    for (const t of ['jsonpath "$.a" == 1', "a {{b}} c", '"x {{y}} z"', "  ", "{{name}", "{{a}b}}", "x{{"]) expect(tokens(t).map((x) => x.text).join("")).toBe(t);
  });
});
