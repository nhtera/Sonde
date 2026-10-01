// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { variableOptions } from "./variable-completion";

describe("variableOptions", () => {
  const vars = [
    { name: "token", kind: "capture" as const, value: "***", source: "capture · line 4" },
    { name: "base_url", kind: "project" as const, value: "http://localhost", source: "sonde.yaml" },
  ];

  it("keeps the list's order, functions last", () => {
    const opts = variableOptions(vars, true);
    expect(opts.map((o) => o.label)).toEqual(["token", "base_url", "newDate", "newUuid"]);
    expect(opts[0].boost! > opts[1].boost!).toBe(true);
    expect(opts.at(-1)?.type).toBe("function");
  });

  it("closes the template when it is open", () => {
    expect(variableOptions(vars, true)[0].apply).toBe("token");
    expect(variableOptions(vars, false)[0].apply).toBe("token}}");
  });
});
