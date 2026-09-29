// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Server mode: the app is served to a browser on 127.0.0.1 and every
// runtime call must carry the per-launch token. The page reads the token
// once from the session endpoint (its cookie proves it opened the launch
// link) and keeps it in memory only; this module adds it to the page's own
// runtime and app requests. The desktop window is served from its own
// scheme and needs none of this.

export const TOKEN_HEADER = "X-Sonde-Token";
export const SESSION_PATH = "/_sonde/session";

/** Whether the page is served by server mode (or the test harness). */
export function isServerMode(loc: Pick<Location, "protocol" | "hostname">): boolean {
  return loc.protocol === "http:" && (loc.hostname === "127.0.0.1" || loc.hostname === "localhost");
}

/** Whether a same-origin path needs the token. */
export function needsToken(path: string): boolean {
  return (
    path === "/wails/runtime" ||
    path.startsWith("/wails/stream/") ||
    path.startsWith("/wails/eventpayload/") ||
    (path.startsWith("/_sonde/") && path !== SESSION_PATH && !path.startsWith("/_sonde/body/"))
  );
}

/**
 * Returns a fetch that adds the token to same-origin requests that need
 * it. The token is fetched once, on first need; if the session endpoint
 * refuses, requests go out without it (and the server refuses them).
 */
export function withToken(base: typeof fetch, origin: string): typeof fetch {
  let token: Promise<string | undefined> | undefined;
  const load = async (): Promise<string | undefined> => {
    try {
      const res = await base(SESSION_PATH, { credentials: "same-origin", cache: "no-store" });
      if (!res.ok) return undefined;
      const body = (await res.json()) as { token?: unknown };
      return typeof body.token === "string" ? body.token : undefined;
    } catch {
      return undefined;
    }
  };
  return async (input, init) => {
    const url = new URL(input instanceof Request ? input.url : String(input), origin);
    if (url.origin !== origin || !needsToken(url.pathname)) {
      return base(input, init);
    }
    token ??= load();
    const value = await token;
    if (value === undefined) {
      token = undefined; // try again on the next call
      return base(input, init);
    }
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
    headers.set(TOKEN_HEADER, value);
    return base(input, { ...init, headers });
  };
}

/** Installs the token fetch on window when the page is in server mode. */
export function install(win: Window & typeof globalThis = window): void {
  if (isServerMode(win.location)) {
    win.fetch = withToken(win.fetch.bind(win), win.location.origin);
  }
}

install();
