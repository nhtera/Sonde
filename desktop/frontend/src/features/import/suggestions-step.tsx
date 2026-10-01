// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The optional suggestions of a Postman import: per file, the diff of the
// asserts, captures and login request its scripts translate to. Nothing
// changes until Apply: then only the accepted files are written.

import * as Dialog from "@radix-ui/react-dialog";
import { unifiedMergeView } from "@codemirror/merge";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { useEffect, useRef, useState } from "react";
import { sondeLanguage } from "../../lang";
import { editorHighlight, editorTheme } from "../editor/theme";
import { tally, useImport } from "./state";

/** after against before, read-only, changed lines marked. */
export function FileDiff({ before, after }: { before: string; after: string }) {
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: after,
        extensions: [
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          sondeLanguage(),
          editorTheme,
          editorHighlight,
          unifiedMergeView({ original: before, mergeControls: false, gutter: true }),
          EditorView.contentAttributes.of({ "aria-label": "Suggested changes" }),
        ],
      }),
    });
    return () => v.destroy();
  }, [before, after]);
  return <div className="file-diff" ref={host} />;
}

export function SuggestionsStep() {
  const suggestions = useImport((s) => s.suggestions);
  const decisions = useImport((s) => s.decisions);
  const busy = useImport((s) => s.busy);
  const applied = useImport((s) => s.applied);
  const [picked, setPicked] = useState(suggestions[0]?.path ?? "");
  const s = suggestions.find((x) => x.path === picked) ?? suggestions[0];
  const { accepted, rejected, pending } = tally(suggestions, decisions);
  const decide = useImport.getState().decide;
  return (
    <div className="suggestions">
      <Dialog.Title className="dialog-title">Suggestions · optional</Dialog.Title>
      <p className="muted small">Nothing has changed yet. Accept or reject each file. Rejected scripts stay as # comments.</p>
      <div className="suggestions-body">
        <ul className="suggestion-files" aria-label="Files with suggestions">
          {suggestions.map((x) => (
            <li key={x.path}>
              <button aria-pressed={x.path === s?.path} onClick={() => setPicked(x.path)}>
                <span className="mono">{x.path}</span>
                <span className={`small state-${applied.includes(x.path) ? "accepted" : (decisions[x.path] ?? (x.error ? "error" : "pending"))}`}>
                  {applied.includes(x.path) ? "applied" : (decisions[x.path] ?? (x.error ? "can't apply" : "pending"))}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {s && (
          <section className="suggestion" aria-label={`Suggestions for ${s.path}`}>
            <div className="section-note">
              <span className="mono">{s.path}</span>
              <span>
                <button className="btn-ghost" aria-pressed={decisions[s.path] === "rejected"} onClick={() => decide(s.path, decisions[s.path] === "rejected" ? null : "rejected")}>
                  Reject
                </button>
                <button className="btn" disabled={!!s.error} aria-pressed={decisions[s.path] === "accepted"} onClick={() => decide(s.path, decisions[s.path] === "accepted" ? null : "accepted")}>
                  {decisions[s.path] === "accepted" ? "✓ Accepted" : "Accept"}
                </button>
              </span>
            </div>
            <p className="muted small">{(s.labels ?? []).join(" · ")}</p>
            {s.error ? <p className="run-error">{s.error}</p> : <FileDiff before={s.before} after={s.after} />}
          </section>
        )}
      </div>
      <div className="dialog-actions">
        <span className="muted small">
          {accepted} accepted · {rejected} rejected · {pending} pending
        </span>
        <button className="btn-ghost" onClick={() => useImport.getState().close()}>
          Skip the rest
        </button>
        <button className="btn-primary" disabled={accepted === 0 || busy} onClick={() => void useImport.getState().apply()}>
          Apply {accepted} accepted
        </button>
      </div>
    </div>
  );
}
