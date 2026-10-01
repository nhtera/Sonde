// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import { rawValue } from "./env-panel";

describe("rawValue", () => {
  describe("number type", () => {
    it("parses numeric strings", () => {
      expect(rawValue("42", "number")).toBe(42);
      expect(rawValue("3.14", "number")).toBe(3.14);
      expect(rawValue("-100", "number")).toBe(-100);
      expect(rawValue("0", "number")).toBe(0);
    });

    it("returns string for non-numeric input", () => {
      expect(rawValue("not a number", "number")).toBe("not a number");
      expect(rawValue("12abc", "number")).toBe("12abc");
    });

    it("returns string for empty or whitespace", () => {
      expect(rawValue("", "number")).toBe("");
      expect(rawValue("   ", "number")).toBe("   ");
    });

    it("handles Infinity and NaN", () => {
      expect(rawValue("Infinity", "number")).toBe("Infinity");
      expect(rawValue("NaN", "number")).toBe("NaN");
    });
  });

  describe("boolean type", () => {
    it("parses 'true' and 'false'", () => {
      expect(rawValue("true", "boolean")).toBe(true);
      expect(rawValue("false", "boolean")).toBe(false);
    });

    it("returns string for other values", () => {
      expect(rawValue("True", "boolean")).toBe("True");
      expect(rawValue("FALSE", "boolean")).toBe("FALSE");
      expect(rawValue("yes", "boolean")).toBe("yes");
      expect(rawValue("1", "boolean")).toBe("1");
    });
  });

  describe("null type", () => {
    it("parses 'null' exactly", () => {
      expect(rawValue("null", "null")).toBe(null);
    });

    it("returns string for non-null values", () => {
      expect(rawValue("Null", "null")).toBe("Null");
      expect(rawValue("NULL", "null")).toBe("NULL");
      expect(rawValue("", "null")).toBe("");
    });
  });

  describe("string type and unknown types", () => {
    it("returns string unchanged", () => {
      expect(rawValue("hello", "string")).toBe("hello");
      expect(rawValue("123", "string")).toBe("123");
      expect(rawValue("", "string")).toBe("");
    });

    it("defaults to string for unknown types", () => {
      expect(rawValue("value", "unknown")).toBe("value");
      expect(rawValue("data", "custom")).toBe("data");
    });
  });
});
