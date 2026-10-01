// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The optional suggestions of a Postman import: per file, each change its
// scripts and settings translate to (an assert, a capture, a login
// request), accepted or rejected on its own. Nothing changes until Apply:
// then only the accepted changes are written.

import * as Dialog from "@radix-ui/react-dialog";
import { useState } from "react";
import { label } from "../../app/keymap/keymap-manager";
import type { ImportChange } from "../../lib/api";
import { hunk } from "./hunk";
import { changeKey, changesOf, fileState, tally, useImport } from "./state";

/** One change: what it is, where, its lines; Accept or Reject it, or
 * undo the decision. */
function ChangeCard({ path, change, before }: { path: string; change: ImportChange; before: string }) {
  const key = changeKey(path, change.index);
  const d = useImport((s) => s.decisions[key]);
  const decide = useImport.getState().decide;
  const h = change.error ? null : hunk(before, change.after);
  return (
    <section className={`change-card${d ? ` ${d}` : ""}`} aria-label={`${change.label} · line ${change.line}`}>
      <header>
        <span>
          <b>{change.label}</b> <span className="muted">· line {change.line}</span>
        </span>
        {change.error ? (
          <span className="muted small">Can&apos;t apply</span>
        ) : d ? (
          <span className="row-gap">
            <button className="btn-ghost" onClick={() => decide(key, null)}>
              Undo
            </button>
            <span className={`decided ${d}`}>{d === "accepted" ? "✓ Accepted" : "Rejected"}</span>
          </span>
        ) : (
          <span className="row-gap">
            <button className="btn" onClick={() => decide(key, "rejected")}>
              Reject
            </button>
            <button className="btn-primary" onClick={() => decide(key, "accepted")}>
              Accept
            </button>
          </span>
        )}
      </header>
      {change.error ? (
        <p className="run-error">{change.error}</p>
      ) : (
        <pre className="change-lines mono" aria-label="Lines">
          {h!.removed.map((l, i) => (
            <span key={`r${i}`} className="del">
              - {l}
            </span>
          ))}
          {h!.added.map((l, i) => (
            <span key={`a${i}`} className="add">
              + {l}
            </span>
          ))}
        </pre>
      )}
    </section>
  );
}

const stateLabel = { accepted: "accepted", rejected: "rejected", mixed: "some accepted", pending: "pending", error: "can't apply" } as const;

export function SuggestionsStep() {
  const suggestions = useImport((s) => s.suggestions);
  const decisions = useImport((s) => s.decisions);
  const busy = useImport((s) => s.busy);
  const applied = useImport((s) => s.applied);
  const [picked, setPicked] = useState(suggestions[0]?.path ?? "");
  const s = suggestions.find((x) => x.path === picked) ?? suggestions[0];
  const { accepted, rejected, pending } = tally(suggestions, decisions);
  const applyKeys = label("$mod+Enter");
  const apply = () => accepted > 0 && !busy && void useImport.getState().apply();
  return (
    <div
      className="suggestions"
      onKeyDown={(e) => {
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault();
          apply();
        }
      }}
    >
      <Dialog.Title className="dialog-title">Suggestions · optional</Dialog.Title>
      <p className="muted small">Nothing has changed yet. Accept or reject each change per file. Rejected scripts stay as # comments.</p>
      <div className="suggestions-body">
        <ul className="suggestion-files" aria-label="Files with suggestions">
          {suggestions.map((x) => {
            const st = applied.includes(x.path) ? "applied" : x.path === s?.path && fileState(x, decisions) === "pending" ? "reviewing" : stateLabel[fileState(x, decisions)];
            return (
              <li key={x.path}>
                <button aria-pressed={x.path === s?.path} onClick={() => setPicked(x.path)}>
                  <span className="mono">{x.path}</span>
                  <span className="muted small">{changesOf(x).length}</span>
                  <span className={`small state-${st.replace(/ .*/, "")}`}>{st}</span>
                </button>
              </li>
            );
          })}
        </ul>
        {s && (
          <section className="suggestion" aria-label={`Suggestions for ${s.path}`}>
            <div className="section-note">
              <span className="mono">{s.path}</span>
              <span className="row-gap">
                <button className="btn" disabled={changesOf(s).length === 0} onClick={() => useImport.getState().decideFile(s.path, "rejected")}>
                  Reject all in file
                </button>
                <button className="btn" disabled={changesOf(s).length === 0} onClick={() => useImport.getState().decideFile(s.path, "accepted")}>
                  Accept all in file
                </button>
              </span>
            </div>
            {s.error && changesOf(s).length === 0 && <p className="run-error">{s.error}</p>}
            <div className="change-cards">
              {(s.changes ?? []).map((c) => (
                <ChangeCard key={c.index} path={s.path} change={c} before={s.before} />
              ))}
            </div>
          </section>
        )}
      </div>
      <div className="dialog-actions">
        <span className="muted small">
          {accepted} accepted · {rejected} rejected · {pending} pending
        </span>
        <button className="btn" onClick={() => useImport.getState().close()}>
          Skip the rest
        </button>
        <button className="btn-primary" disabled={accepted === 0 || busy} onClick={apply}>
          Apply {accepted} accepted{applyKeys && <kbd aria-hidden>{applyKeys}</kbd>}
        </button>
      </div>
    </div>
  );
}
