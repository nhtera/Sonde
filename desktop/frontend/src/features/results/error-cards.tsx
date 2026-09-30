// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A request that got no response: why, in plain words, and what to try
// (start the mock, retry, raise max-time, open the TLS settings). A
// canceled run is said so, without error styling.

import { useEffect, useState } from "react";
import type { EntryError } from "../../lib/view";
import { useRegistry } from "../../app/registry";
import { hasCommand, mockSpec, retry, runCommand, setMaxTime, startMock } from "./actions";
import { cardOf } from "./model";

export function ErrorCard({ file, entry, err }: { file: string; entry: number; err: EntryError }) {
  useRegistry();
  const card = cardOf(err);
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
    </button>
  );
  return (
    <div className="tab-pad">
      <section className={`error-card ${card.neutral ? "neutral" : ""}`} role={card.neutral ? "status" : "alert"}>
        <div className="error-title">
          <b>{card.title}</b>
          <span className="code-chip mono">{card.code}</span>
        </div>
        <p className="error-message mono">{err.message || err.description}</p>
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
    </div>
  );
}
