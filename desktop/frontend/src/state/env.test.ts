// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { EnvProject } from "../lib/api";
import { touchesEnvs } from "./env";

describe("touchesEnvs", () => {
  const project = {
    config: "sonde.yaml",
    envs: [
      {
        name: "dev",
        default: true,
        secretsFile: "secrets/dev.secrets",
        variables: [{ name: "host", value: "x", type: "string", source: "vars/dev.env", secret: false }],
      },
    ],
  } as EnvProject;

  it("is sonde.yaml, even in a project without one yet", () => {
    expect(touchesEnvs(null, ["imported/a.hurl", "sonde.yaml"])).toBe(true);
    expect(touchesEnvs(null, ["sonde.yml"])).toBe(true);
    expect(touchesEnvs(null, ["imported/sonde.yaml"])).toBe(false);
  });

  it("is a file the environments read", () => {
    expect(touchesEnvs(project, ["vars/dev.env"])).toBe(true);
    expect(touchesEnvs(project, ["secrets/dev.secrets"])).toBe(true);
    expect(touchesEnvs(project, ["api/users.hurl"])).toBe(false);
  });
});
