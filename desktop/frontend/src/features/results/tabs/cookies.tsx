// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Cookies sent with the request (and the earlier request that set each)
// and those the response set, with an assert to add for one.

import type { Entry } from "../../../lib/view";
import { useRegistry } from "../../../app/registry";
import { addAssert, hasCommand, runCommand } from "../actions";
import { sentCookies } from "../model";

export function CookiesTab({ file, entry, entries }: { file: string; entry: Entry; entries: Record<number, Entry> }) {
  useRegistry();
  const sent = sentCookies(entries, entry.index);
  const received = entry.calls?.at(-1)?.response.cookies ?? [];
  const sentNames = new Set(sent.map((c) => c.name));
  const first = received[0];
  const quoted = (s: string) => s.replace(/[\\"]/g, "\\$&");
  const assertText = first ? `cookie "${quoted(first.name)}${first.httponly ? "[HttpOnly]" : ""}" exists` : "";
  return (
    <div className="tab-pad cookies-tab">
      <section className="kv-section">
        <h3>
          Sent with this request <span className="muted">{sent.length}</span>
        </h3>
        {sent.length === 0 ? (
          <p className="body-note">No cookies were sent.</p>
        ) : (
          <dl className="kv mono">
            {sent.map((c, i) => (
              <div key={i}>
                <dt>{c.name}</dt>
                <dd>{c.value}</dd>
                <dd className="muted">{c.setBy ? `set by request ${c.setBy}` : "from the jar"}</dd>
              </div>
            ))}
          </dl>
        )}
      </section>
      <section className="kv-section">
        <h3>
          Received · Set-Cookie <span className="muted">{received.length}</span>
        </h3>
        {received.length === 0 && <p className="body-note">The response set no cookies.</p>}
        {received.map((c, i) => (
          <div key={i} className="cookie-card">
            <div className="cookie-name mono">
              <b>{c.name}</b> = {c.value}
              {!sentNames.has(c.name) && <span className="new-chip">new</span>}
            </div>
            <dl className="cookie-attrs">
              <div><dt>Domain</dt><dd className="mono">{c.domain || "–"}</dd></div>
              <div><dt>Path</dt><dd className="mono">{c.path || "–"}</dd></div>
              <div><dt>Expires</dt><dd className="mono">{c.expires || (c.max_age ? `max-age ${c.max_age}` : "session")}</dd></div>
              <div><dt>SameSite</dt><dd className="mono">{c.same_site || "–"}</dd></div>
              <div><dt>HttpOnly</dt><dd className="mono">{c.httponly ? "✓" : "–"}</dd></div>
              <div><dt>Secure</dt><dd className="mono">{c.secure ? "✓" : "–"}</dd></div>
            </dl>
          </div>
        ))}
      </section>
      {hasCommand("cookies.openJar") && (
        <div className="cookie-jar-row">
          <span>All cookies for this project</span>
          <button className="btn-ghost accent" onClick={() => runCommand("cookies.openJar")}>
            Open cookie jar
          </button>
        </div>
      )}
      {assertText && (
        <button className="assert-helper" onClick={() => void addAssert(file, entry.index, assertText)}>
          <span className="muted">Assert a cookie in the file</span>
          <code className="mono">{assertText}</code>
        </button>
      )}
    </div>
  );
}
