// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A request that got no response: why, in plain words, and what to try
// (start the mock, retry, raise max-time, open the TLS settings). A
// canceled run is said so, without error styling.

import { useEffect, useMemo, useState } from "react";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { useRuns } from "../../state/run";
import type { EntryError } from "../../lib/view";
import { useRegistry } from "../../app/registry";
import { hasCommand, mockSpec, retry, runCommand, setMaxTime, startMock } from "./actions";
import { cardOf } from "./model";

/** "127.0.0.1:34129 did not accept the connection." from a refused
 * connection's message, when it names the host and port. */
export function refusedText(message: string): string | undefined {
  const m = /connect to (\S+) port (\d+)/.exec(message);
  return m ? `${m[1]}:${m[2]} did not accept the connection.` : undefined;
}

/** "stg.shop.dev: no such host" from a failed lookup's message. */
export function unresolvedText(message: string): string | undefined {
  const m = /resolve host:? (\S+)/i.exec(message);
  return m ? `${m[1]}: no such host` : undefined;
}

// Failures before the request left: a timeout may come after it was sent.
const unsent = new Set(["connect", "resolve", "tls", "host-denied"]);

export function ErrorCard({ file, entry, err }: { file: string; entry: number; err: EntryError }) {
  useRegistry();
  const card = cardOf(err);
  const retryKeys = useKeyLabel("request.send");
  // What the connection tried, as the run logged it.
  const ms = useRuns((s) => s.runs[file]?.entries[entry]?.time);
  // What the connection tried: the address, the error, how long.
  const tried = useMemo(() => {
    const at = /connect to (\S+) port (\d+)/.exec(err.message);
    return [
      at && `Trying ${at[1]}:${at[2]}...`,
      ...err.message.split("\n").filter(Boolean),
      `Failed${ms !== undefined ? ` after ${ms} ms` : ""}${unsent.has(err.transport ?? "") ? " · no request sent" : ""}`,
    ].filter(Boolean) as string[];
  }, [err.message, err.transport, ms]);
  const [spec, setSpec] = useState("");
  useEffect(() => {
    if (err.transport !== "connect") return;
    let live = true;
    void mockSpec().then((s) => live && setSpec(s));
    return () => {
      live = false;
    };
  }, [err.transport]);
  if (!card) return null;
  const retryButton = (
    <button className="btn" onClick={() => retry(file, entry)}>
      Retry
      {retryKeys && <kbd>{retryKeys}</kbd>}
    </button>
  );
  return (
    <div className="tab-pad">
      <section className={`error-card ${card.neutral ? "neutral" : ""}`} role={card.neutral ? "status" : "alert"}>
        <div className="error-title">
          <b>{card.title}</b>
          <span className="code-chip mono">{card.code}</span>
        </div>
        <p className="error-message mono">
          {(err.transport === "connect" && refusedText(err.message)) || (err.transport === "resolve" && unresolvedText(err.message)) || err.message || err.description}
        </p>
        <p className="error-hint">{card.hint}</p>
        <div className="error-actions">
          {err.transport === "connect" && spec && (
            <button className="btn primary-soft" onClick={() => void startMock(file, entry)}>
              Start mock on :4010
            </button>
          )}
          {(err.transport === "connect" || err.transport === "resolve") && hasCommand("env.pick") && (
            <button className="btn" onClick={() => runCommand("env.pick")}>
              Switch environment
            </button>
          )}
          {err.transport === "resolve" && hasCommand("env.open") && (
            <button className="btn" onClick={() => runCommand("env.open")}>
              Edit environment
            </button>
          )}
          {err.transport === "tls" && hasCommand("settings.open") && (
            <button className="btn" onClick={() => runCommand("settings.open", "tls")}>
              Settings › TLS
            </button>
          )}
          {err.transport === "timeout" && (
            <button className="btn" onClick={() => void setMaxTime(file, entry, "30s")}>
              Set max-time: 30s
            </button>
          )}
          {err.transport !== "host-denied" && retryButton}
        </div>
      </section>
      {!card.neutral && (
        <pre className="error-trace mono" aria-label="What was tried">
          {tried.map((l, i) => (
            // The time taken differs run to run.
            <span key={i} data-volatile={i === tried.length - 1 || undefined}>{`* ${l}\n`}</span>
          ))}
        </pre>
      )}
    </div>
  );
}
