// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { Entry } from "../../lib/view";
import { failureOf } from "./model";

describe("failureOf", () => {
  it("gives a status check the status expected and the one received", () => {
    const entry = { index: 1, errors: [{ line: 7, column: 1, kind: "assert-status", description: "Assert status code", message: "actual value is <422>" }] } as unknown as Entry;
    const f = failureOf(entry, ["", "", "", "", "", "", "HTTP 201"]);
    expect(f).toMatchObject({ title: "Status failed", line: 7, code: "HTTP 201", expected: "201", actual: "422" });
  });
});
