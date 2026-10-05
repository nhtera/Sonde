// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Worker. Every page is a prerendered static asset, and assets are served
// before the Worker runs, so a deployed Worker only ever sees paths that
// have no page: it answers them with the prerendered 404.html, status 404 and
// the site's security headers. TanStack Start renders only during the build
// (prerender requests carry a per-build token) and in development.
import handler from "@tanstack/react-start/server-entry";
import { contentSecurityPolicy, inlineScripts, SECURITY_HEADERS } from "./lib/security-headers";

declare const __SONDE_PRERENDER_TOKEN__: string;
// Set by vite.config.ts on every prerender request; not exported (Workers
// treat named exports as entrypoints).
const PRERENDER_HEADER = "x-sonde-prerender";

interface Env {
  ASSETS?: { fetch(input: Request | URL | string): Promise<Response> };
}

async function sha256Base64(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return btoa(String.fromCharCode(...new Uint8Array(digest)));
}

let notFoundPage: Promise<{ html: string; csp: string }> | undefined;

async function loadNotFound(env: Env, url: string) {
  if (!env.ASSETS) throw new Error("ASSETS binding missing");
  const res = await env.ASSETS.fetch(new URL("/404.html", url));
  const html = await res.text();
  const hashes = await Promise.all(inlineScripts(html).map(sha256Base64));
  return { html, csp: contentSecurityPolicy(hashes) };
}

// Workers module syntax: the runtime calls fetch(request, env, ctx), and
// TanStack's entry takes the same arguments.
const render = handler.fetch as unknown as (request: Request, env: Env, ctx: unknown) => Promise<Response>;

export default {
  async fetch(request: Request, env: Env, ctx: unknown): Promise<Response> {
    if (import.meta.env.DEV || request.headers.get(PRERENDER_HEADER) === __SONDE_PRERENDER_TOKEN__) {
      return render(request, env, ctx);
    }
    notFoundPage ??= loadNotFound(env, request.url).catch((err) => {
      notFoundPage = undefined;
      throw err;
    });
    const { html, csp } = await notFoundPage;
    return new Response(request.method === "HEAD" ? null : html, {
      status: 404,
      headers: { ...SECURITY_HEADERS, "Content-Security-Policy": csp, "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" },
    });
  },
};
