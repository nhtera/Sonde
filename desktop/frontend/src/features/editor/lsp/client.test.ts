// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The language client over the app's sessions: one initialize per session,
// and a replaced session gets its own (a reloaded page never shows
// "initialize sent twice").

import { beforeEach, describe, expect, it, vi } from "vitest";

const bus = new Map<string, Set<(d: unknown) => void>>();
const sent = new Map<string, string[]>();
let sessionListener: ((id: string | null) => void) | null = null;

vi.mock("../../../lib/events", () => ({
  on: (topic: string, h: (d: unknown) => void) => {
    if (!bus.has(topic)) bus.set(topic, new Set());
    bus.get(topic)!.add(h);
    return () => bus.get(topic)?.delete(h);
  },
}));
const restartLspSession = vi.fn();
vi.mock("../../../lib/lsp-session", () => ({
  onLspSession: (l: (id: string | null) => void) => {
    sessionListener = l;
    return () => {};
  },
  restartLspSession: () => restartLspSession(),
}));
vi.mock("../../../lib/api", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Lsp: {
    Send: vi.fn(async (id: string, msg: string) => {
      (sent.get(id) ?? sent.set(id, []).get(id)!).push(msg);
      // The server answers initialize.
      const m = JSON.parse(msg) as { id?: number; method: string };
      if (m.method === "initialize") {
        setTimeout(() => bus.get(`lsp:${id}`)?.forEach((h) => h(JSON.stringify({ jsonrpc: "2.0", id: m.id, result: { capabilities: { textDocumentSync: 1 } } }))));
      }
    }),
    Configure: vi.fn(async () => {}),
  },
}));

const { useWorkspace } = await import("../../../state/workspace");
const { useEnv } = await import("../../../state/env");
const { lspClient, fileURI, extraVariables, lspReady } = await import("./client");

const initializes = (id: string) => (sent.get(id) ?? []).filter((m) => JSON.parse(m).method === "initialize").length;

beforeEach(() => sent.clear());

describe("language client", () => {
  it("initializes once per session", async () => {
    useWorkspace.setState({ project: { name: "p", dir: "/work/my project" } });
    const client = lspClient();
    sessionListener!("s1");
    await client.initializing;
    expect(initializes("s1")).toBe(1);

    sessionListener!(null);
    sessionListener!("s2");
    await client.initializing;
    expect(initializes("s2")).toBe(1);
    expect(initializes("s1")).toBe(1);
  });

  it("gives another project its own session, never a second initialize", async () => {
    useWorkspace.setState({ project: { name: "p", dir: "/work/my project" } });
    const first = lspClient();
    sessionListener!("s3");
    await first.initializing;
    expect(lspReady()).toBe(true);

    useWorkspace.setState({ project: { name: "q", dir: "/work/other" } });
    const second = lspClient();
    expect(second).not.toBe(first);
    expect(restartLspSession).toHaveBeenCalledOnce();
    expect(initializes("s3")).toBe(1);
    expect(lspReady()).toBe(false);

    sessionListener!(null);
    sessionListener!("s4");
    await second.initializing;
    expect(initializes("s4")).toBe(1);
    expect(initializes("s3")).toBe(1);
  });

  it("treats the session's overrides as defined, not other files' captures", () => {
    useEnv.setState({ overrides: { count: 2, items: [{ name: "base_url", source: "session", flag: "" }, { name: "token", source: "session", flag: "" }] } });
    expect(extraVariables()).toEqual(["base_url", "token"]);
  });

  it("names files by URI under the project", () => {
    useWorkspace.setState({ project: { name: "p", dir: "/work/my project" } });
    expect(fileURI("api/a b.hurl")).toBe("file:///work/my%20project/api/a%20b.hurl");
  });
});
