// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { requestFiles } from "./state";
import type { Node } from "../../../lib/api";

describe("requestFiles", () => {
  it("returns empty array for null tree", () => {
    expect(requestFiles(null)).toEqual([]);
  });

  it("extracts request files from flat tree", () => {
    const tree: Node = {
      kind: "folder",
      path: "/",
      name: "root",
      children: [
        { kind: "request", path: "a.hurl", name: "a.hurl", children: [] },
        { kind: "request", path: "b.hurl", name: "b.hurl", children: [] },
      ],
    };
    expect(requestFiles(tree)).toEqual(["a.hurl", "b.hurl"]);
  });

  it("skips non-request nodes", () => {
    const tree: Node = {
      kind: "folder",
      path: "/",
      name: "root",
      children: [
        { kind: "folder", path: "sub", name: "sub", children: [] },
        { kind: "request", path: "a.hurl", name: "a.hurl", children: [] },
        { kind: "text", path: "readme.md", name: "readme.md", children: [] },
      ],
    };
    expect(requestFiles(tree)).toEqual(["a.hurl"]);
  });

  it("walks nested trees in order", () => {
    const tree: Node = {
      kind: "folder",
      path: "/",
      name: "root",
      children: [
        {
          kind: "folder",
          path: "api",
          name: "api",
          children: [
            { kind: "request", path: "api/login.hurl", name: "login.hurl", children: [] },
            { kind: "request", path: "api/users.hurl", name: "users.hurl", children: [] },
          ],
        },
        { kind: "request", path: "smoke.hurl", name: "smoke.hurl", children: [] },
      ],
    };
    expect(requestFiles(tree)).toEqual([
      "api/login.hurl",
      "api/users.hurl",
      "smoke.hurl",
    ]);
  });

  it("handles deeply nested trees", () => {
    const tree: Node = {
      kind: "folder",
      path: "/",
      name: "root",
      children: [
        {
          kind: "folder",
          path: "a",
          name: "a",
          children: [
            {
              kind: "folder",
              path: "a/b",
              name: "b",
              children: [
                { kind: "request", path: "a/b/c.hurl", name: "c.hurl", children: [] },
              ],
            },
          ],
        },
      ],
    };
    expect(requestFiles(tree)).toEqual(["a/b/c.hurl"]);
  });

  it("preserves tree order with multiple levels", () => {
    const tree: Node = {
      kind: "folder",
      path: "/",
      name: "root",
      children: [
        { kind: "request", path: "z.hurl", name: "z.hurl", children: [] },
        { kind: "request", path: "a.hurl", name: "a.hurl", children: [] },
        {
          kind: "folder",
          path: "dir",
          name: "dir",
          children: [
            { kind: "request", path: "dir/m.hurl", name: "m.hurl", children: [] },
            { kind: "request", path: "dir/b.hurl", name: "b.hurl", children: [] },
          ],
        },
      ],
    };
    expect(requestFiles(tree)).toEqual([
      "z.hurl",
      "a.hurl",
      "dir/m.hurl",
      "dir/b.hurl",
    ]);
  });

  it("ignores undefined children", () => {
    const tree: Node = {
      kind: "folder",
      path: "/",
      name: "root",
      children: [
        { kind: "request", path: "a.hurl", name: "a.hurl" },
        { kind: "folder", path: "dir", name: "dir" },
        { kind: "request", path: "b.hurl", name: "b.hurl", children: [] },
      ],
    };
    expect(requestFiles(tree)).toEqual(["a.hurl", "b.hurl"]);
  });
});
