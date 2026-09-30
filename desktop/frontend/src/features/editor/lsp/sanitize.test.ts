// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { sanitizeHTML } from "./sanitize";

describe("hover HTML", () => {
  it("keeps formatting and drops scripts, handlers and unsafe links", () => {
    const out = sanitizeHTML('<p onclick="x()"><code>a</code><script>alert(1)</script><img src=x onerror=y><a href="javascript:z">l</a><a href="https://x.dev">d</a></p>');
    expect(out).toBe('<p><code>a</code>alert(1)<a>l</a><a href="https://x.dev" target="_blank" rel="noreferrer noopener">d</a></p>');
  });
});
