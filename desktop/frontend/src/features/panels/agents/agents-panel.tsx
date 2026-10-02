// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// AI agents: the configuration that starts `sonde mcp` on this project in
// an agent (Claude Code, Cursor, VS Code). Read-only unless requests are
// allowed, and then to the hosts listed only.

import { useEffect, useState } from "react";
import { Agents, appError, type AgentSnippet } from "../../../lib/api";
import { useUI } from "../../../state/ui";

const clients = [
  { id: "claude", label: "Claude Code" },
  { id: "cursor", label: "Cursor" },
  { id: "vscode", label: "VS Code" },
];

export function AgentsPanel() {
  const [client, setClient] = useState("claude");
  const [scope, setScope] = useState("user");
  const [allowRun, setAllowRun] = useState(false);
  const [hosts, setHosts] = useState<string[]>(["localhost"]);
  const [draft, setDraft] = useState("");
  const [snippet, setSnippet] = useState<AgentSnippet | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    Agents.Snippet({ client, scope, allowRun, hosts }).then(
      (s) => {
        if (!live) return;
        setSnippet(s);
        setError("");
      },
      (err) => live && setError(appError(err).message),
    );
    return () => {
      live = false;
    };
  }, [client, scope, allowRun, hosts]);
  const addHost = () => {
    const h = draft.trim();
    if (h && !hosts.includes(h)) setHosts([...hosts, h]);
    setDraft("");
  };
  const label = client === "claude" ? "Claude Code" : clients.find((c) => c.id === client)!.label;
  return (
    <div className="panel-body">
      <div className="panel-head">
        <h2>AI agents</h2>
      </div>
      <p className="muted small">Agents use the same files through sonde mcp. Nothing leaves this machine unless you allow it.</p>
      <div className="segmented full" role="group" aria-label="Agent">
        {clients.map((c) => (
          <button key={c.id} aria-pressed={client === c.id} onClick={() => setClient(c.id)}>
            {c.label}
          </button>
        ))}
      </div>
      <div className="segmented full" role="group" aria-label="Scope">
        <button aria-pressed={scope === "user"} onClick={() => setScope("user")}>
          This machine
        </button>
        <button aria-pressed={scope === "project"} onClick={() => setScope("project")}>
          In the repo
        </button>
      </div>
      <section className="card">
        <label className="toggle-line">
          <span>
            <b>Allow the agent to send requests</b>
            <span className="muted small">Off means list and check only.</span>
          </span>
          <input type="checkbox" role="switch" className="switch" aria-label="Allow the agent to send requests" checked={allowRun} onChange={(e) => setAllowRun(e.target.checked)} />
        </label>
        {allowRun && (
          <>
            <div className="muted small">Allowed hosts</div>
            <div className="host-chips">
              {hosts.map((h) => (
                <span key={h} className="host-chip mono">
                  {h}
                  <button aria-label={`Remove ${h}`} onClick={() => setHosts(hosts.filter((x) => x !== h))}>
                    ×
                  </button>
                </span>
              ))}
              <input
                className="mono"
                aria-label="Add a host"
                placeholder="+ host"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && addHost()}
                onBlur={addHost}
              />
            </div>
          </>
        )}
      </section>
      {error ? (
        <p className="form-note fail">{error}</p>
      ) : (
        snippet && (
          <>
            <div className="section-note">
              <span>
                Add to {label} · {snippet.where}
              </span>
              <button className="btn-ghost accent" onClick={() => void navigator.clipboard.writeText(snippet.text).then(() => useUI.getState().toast({ kind: "success", text: "Copied" }))}>
                Copy
              </button>
            </div>
            <pre className="snippet mono" aria-label="Agent configuration">
              {snippet.text}
            </pre>
            <div className="panel-section">
              <h3>Tools the agent gets</h3>
              <ul className="tool-list">
                {(snippet.tools ?? []).map((t) => (
                  <li key={t.name}>
                    <span className="mono accent">{t.name}</span>
                    <span className="muted small">{t.title}</span>
                  </li>
                ))}
              </ul>
            </div>
          </>
        )
      )}
    </div>
  );
}
