// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { EnvProject } from "../../lib/api";
import { toDefine } from "./result-tile";

describe("toDefine", () => {
  const used = [
    { name: "fieldId", files: 8 },
    { name: "nmk-cookie", files: 30 },
    { name: "tmp", files: 1 },
  ];
  const project = {
    config: "sonde.yaml",
    envs: [{ name: "collection", default: true, secretsFile: "", variables: [{ name: "fieldId", value: "f", type: "string", source: "sonde.yaml", secret: false }] }],
  } as EnvProject;

  it("is what neither the environment nor a session override sets", () => {
    const overrides = { count: 1, items: [{ name: "tmp", source: "session", flag: "--variable" }] };
    expect(toDefine(used, project, "collection", overrides as never).map((v) => v.name)).toEqual(["nmk-cookie"]);
  });

  it("is every variable with no environment", () => {
    expect(toDefine(used, null, "", { count: 0, items: [] }).map((v) => v.name)).toEqual(["fieldId", "nmk-cookie", "tmp"]);
  });
});
