// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import type { Node } from "../../lib/api";
import { dataFiles } from "./data-picker";

describe("data-picker", () => {
  it("dataFiles: returns empty array when tree is null", () => {
    expect(dataFiles(null)).toEqual([]);
  });

  it("dataFiles: finds all data files in the tree, sorted", () => {
    const tree: Node = {
      kind: "dir",
      path: ".",
      name: ".",
      children: [
        { kind: "file", path: "checkout.hurl", name: "checkout.hurl", children: null },
        { kind: "data", path: "data/users.csv", name: "users.csv", children: null },
        {
          kind: "dir",
          path: "tests",
          name: "tests",
          children: [
            { kind: "data", path: "tests/login.json", name: "login.json", children: null },
            { kind: "file", path: "tests/auth.hurl", name: "auth.hurl", children: null },
          ],
        },
        { kind: "data", path: "data/products.csv", name: "products.csv", children: null },
      ],
    };

    const result = dataFiles(tree);

    expect(result).toEqual(["data/products.csv", "data/users.csv", "tests/login.json"]);
  });

  it("dataFiles: ignores non-data files", () => {
    const tree: Node = {
      kind: "dir",
      path: ".",
      name: ".",
      children: [
        { kind: "file", path: "a.hurl", name: "a.hurl", children: null },
        { kind: "file", path: "b.txt", name: "b.txt", children: null },
      ],
    };

    expect(dataFiles(tree)).toEqual([]);
  });

  it("dataFiles: handles deeply nested data files", () => {
    const tree: Node = {
      kind: "dir",
      path: ".",
      name: ".",
      children: [
        {
          kind: "dir",
          path: "a",
          name: "a",
          children: [
            {
              kind: "dir",
              path: "a/b",
              name: "b",
              children: [{ kind: "data", path: "a/b/deep.csv", name: "deep.csv", children: null }],
            },
          ],
        },
      ],
    };

    expect(dataFiles(tree)).toEqual(["a/b/deep.csv"]);
  });
});
