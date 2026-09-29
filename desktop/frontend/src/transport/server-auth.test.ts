// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it, vi } from "vitest";
import { isServerMode, needsToken, SESSION_PATH, TOKEN_HEADER, withToken } from "./server-auth";

const origin = "http://127.0.0.1:7780";

function fakeFetch(session: { status: number; token?: string }) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), origin);
    if (url.pathname === SESSION_PATH) {
      return new Response(JSON.stringify({ token: session.token }), { status: session.status });
    }
    const headers = new Headers(init?.headers);
    return new Response(headers.get(TOKEN_HEADER) ?? "none");
  });
}

describe("server-auth", () => {
  it("detects server mode", () => {
    expect(isServerMode({ protocol: "http:", hostname: "127.0.0.1" })).toBe(true);
    expect(isServerMode({ protocol: "http:", hostname: "localhost" })).toBe(true);
    expect(isServerMode({ protocol: "wails:", hostname: "wails.localhost" })).toBe(false);
    expect(isServerMode({ protocol: "http:", hostname: "wails.localhost" })).toBe(false);
  });

  it("classifies paths like the server", () => {
    expect(needsToken("/wails/runtime")).toBe(true);
    expect(needsToken("/wails/stream/poll")).toBe(true);
    expect(needsToken("/_sonde/events")).toBe(true);
    expect(needsToken(SESSION_PATH)).toBe(false);
    expect(needsToken("/_sonde/body/abc")).toBe(false);
    expect(needsToken("/wails/runtime.js")).toBe(false);
    expect(needsToken("/assets/index.js")).toBe(false);
  });

  it("adds the token to runtime calls only, fetching it once", async () => {
    const base = fakeFetch({ status: 200, token: "t0k" });
    const f = withToken(base as unknown as typeof fetch, origin);
    expect(await (await f("/wails/runtime", { method: "POST" })).text()).toBe("t0k");
    expect(await (await f(`${origin}/_sonde/events`)).text()).toBe("t0k");
    expect(await (await f("/assets/index.js")).text()).toBe("none");
    expect(await (await f("http://127.0.0.1:9999/wails/runtime")).text()).toBe("none");
    expect(base.mock.calls.filter(([u]) => String(u) === SESSION_PATH)).toHaveLength(1);
  });

  it("sends no token when the session is refused, and retries later", async () => {
    const base = fakeFetch({ status: 401 });
    const f = withToken(base as unknown as typeof fetch, origin);
    expect(await (await f("/wails/runtime")).text()).toBe("none");
    await f("/wails/runtime");
    expect(base.mock.calls.filter(([u]) => String(u) === SESSION_PATH)).toHaveLength(2);
  });
});
