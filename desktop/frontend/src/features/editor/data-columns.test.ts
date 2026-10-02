// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { dataColumns } from "./data-columns";

describe("dataColumns", () => {
  it("reads a CSV header", () => {
    expect(dataColumns("d.csv", "name, email\nada,a@b.c\n")).toEqual(["name", "email"]);
    expect(dataColumns("d.csv", '"full, name",email\r\nx,y')).toEqual(["full, name", "email"]);
    expect(dataColumns("d.csv", "")).toEqual([]);
  });
  it("reads a JSON array's first object", () => {
    expect(dataColumns("d.json", '[{"user": "ada", "pass": "x"}, {"other": 1}]')).toEqual(["pass", "user"]);
    expect(dataColumns("d.json", '{"user": "ada"}')).toEqual([]);
    expect(dataColumns("d.json", "not json")).toEqual([]);
  });
});
