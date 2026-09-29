// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Spikes: each returns numbers for desktop/README.md §Spikes.

import { Call, type CancellablePromise } from "@wailsio/runtime";

const svc = "main.HarnessService";
const call = <T>(method: string, ...args: unknown[]) => Call.ByName(`${svc}.${method}`, ...args) as CancellablePromise<T>;
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

export async function runSpike(name: string): Promise<unknown> {
  switch (name) {
    case "cancel":
      return cancelCall();
    case "bodies":
      return { call: await cancelCall(), ...(await bodies(50)) };
    default:
      throw new Error(`unknown spike ${name}`);
  }
}

/** Cancelling a call from JS reaches the Go method's context. */
export async function cancelCall(): Promise<{ state: string; ms: number }> {
  const id = `w${Date.now()}`;
  const p = call<void>("Wait", id);
  p.catch(() => undefined);
  await sleep(200);
  const t0 = performance.now();
  p.cancel();
  let state = "";
  for (let i = 0; i < 50 && state !== "canceled"; i++) {
    await sleep(20);
    state = await call<string>("WaitState", id);
  }
  return { state, ms: Math.round(performance.now() - t0) };
}

interface BodyState {
  written: number;
  finished: boolean;
  aborted: boolean;
}

/** A body of mb MiB fetched by URL and parsed in a worker; a fetch cancelled mid-stream; the sandbox policy. */
export async function bodies(mb: number) {
  const size = mb << 20;

  const jsonId = await call<string>("NewBody", size, "json");
  let t0 = performance.now();
  const res = await fetch(`/_sonde/body/${jsonId}`);
  const buf = await res.arrayBuffer();
  const fetchMs = performance.now() - t0;
  t0 = performance.now();
  const worker = new Worker(new URL("./parse-worker.ts", import.meta.url), { type: "module" });
  const parsed = await new Promise<{ items: number; ms: number }>((resolve, reject) => {
    worker.onmessage = (e) => resolve(e.data as { items: number; ms: number });
    worker.onerror = (e) => reject(new Error(e.message));
    worker.postMessage(buf, [buf]);
  });
  worker.terminate();
  const parseTotalMs = performance.now() - t0;

  const binId = await call<string>("NewBody", size, "slow");
  const ctl = new AbortController();
  const stream = await fetch(`/_sonde/body/${binId}`, { signal: ctl.signal });
  const reader = stream.body!.getReader();
  let read = 0;
  while (read < 1 << 20) {
    const { value, done } = await reader.read();
    if (done) break;
    read += value.byteLength;
  }
  ctl.abort();
  let state: BodyState = { written: 0, finished: false, aborted: false };
  for (let i = 0; i < 100 && !state.aborted && !state.finished; i++) {
    await sleep(20);
    state = await call<BodyState>("BodyState", binId);
  }

  return {
    mb,
    fetchMs: Math.round(fetchMs),
    workerParseMs: parsed.ms,
    parseTotalMs: Math.round(parseTotalMs),
    items: parsed.items,
    cancel: { readBeforeAbort: read, served: state.written, total: size, aborted: state.aborted, finished: state.finished },
    sandbox: await sandboxHonored(),
  };
}

/**
 * A body page served with CSP sandbox must not run its script, whether
 * framed (the desktop window allows it; server mode denies all framing)
 * or opened as a page. The control page, served without the policy, shows
 * whether each way can run a script at all.
 */
async function sandboxHonored() {
  return { sandboxed: await scriptRuns("html"), control: await scriptRuns("control") };
}

async function scriptRuns(kind: string): Promise<{ iframe: boolean; popupOpened: boolean; popup: boolean }> {
  const url = `/_sonde/body/${await call<string>("NewBody", 0, kind)}`;
  let ran = false;
  const onMessage = (e: MessageEvent) => {
    if (e.data === "body-script-ran") ran = true;
  };
  window.addEventListener("message", onMessage);
  try {
    const frame = document.createElement("iframe");
    frame.src = url;
    document.body.appendChild(frame);
    await sleep(1000);
    frame.remove();
    const iframe = ran;
    ran = false;
    const popup = window.open(url, "_blank");
    await sleep(1000);
    popup?.close();
    return { iframe, popupOpened: popup !== null, popup: ran };
  } finally {
    window.removeEventListener("message", onMessage);
  }
}
