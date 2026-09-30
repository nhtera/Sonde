// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Body tab: Pretty (a JSON tree; other text highlighted), Raw (as
// received), Preview (HTML, images, PDF), with search, copy and save. A
// body over 20 MB shows its first megabyte until formatted anyway; over
// 50 MB it is only saved or opened in another app.

import { useCallback, useEffect, useState, type KeyboardEvent } from "react";
import { useKeyLabel } from "../../../../app/keymap/use-keys";
import { appError } from "../../../../lib/api";
import { serverMode } from "../../../../lib/mode";
import type { Body } from "../../../../lib/view";
import { useUI } from "../../../../state/ui";
import { addFromBody } from "../../actions";
import { openExternally, saveBody } from "../../body-actions";
import { LargeBody } from "../../large-body";
import { bodyKind, FORMAT_LIMIT, RAW_PREVIEW, VIEW_LIMIT, type BodyKind } from "../../model";
import { useResults, type BodyMode } from "../../state";
import { fetchBody, hexDump, type BodyText } from "./body-fetch";
import { Preview } from "./preview";
import { PrettyJsonTree } from "./pretty-json-tree";
import { RawView } from "./raw-view";

/** The modes a body kind offers. */
function modesOf(kind: BodyKind): BodyMode[] {
  switch (kind) {
    case "json":
    case "xml":
    case "text":
      return ["pretty", "raw"];
    case "html":
      return ["pretty", "raw", "preview"];
    case "image":
    case "pdf":
      return ["raw", "preview"];
    default:
      return ["raw"];
  }
}

const modeLabel: Record<BodyMode, string> = { pretty: "Pretty", raw: "Raw", preview: "Preview" };

/** The body's text (up to limit bytes), once read. */
function useBodyText(id: string, limit: number, enabled: boolean) {
  const [state, setState] = useState<{ id: string; limit: number; body?: BodyText; error?: string } | null>(null);
  useEffect(() => {
    if (!enabled) return;
    let live = true;
    fetchBody(id, limit).then(
      (body) => live && setState({ id, limit, body }),
      (err) => live && setState({ id, limit, error: appError(err).message }),
    );
    return () => {
      live = false;
    };
  }, [id, limit, enabled]);
  return state && state.id === id && state.limit === limit ? state : null;
}

export function BodyTab({ file, entry, body }: { file: string; entry: number; body: Body }) {
  const kind = bodyKind(body.contentType);
  const [formatted, setFormatted] = useState<string | null>(null); // body id formatted anyway
  const [invalid, setInvalid] = useState<string | null>(null); // body id that is not JSON
  const chosen = useResults((s) => s.bodyMode);
  const saveKeys = useKeyLabel("response.save");
  const [query, setQuery] = useState("");
  // Searched once typing pauses (a large body takes a while to scan).
  const [searched, setSearched] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setSearched(query), 150);
    return () => clearTimeout(t);
  }, [query]);
  const [nav, setNav] = useState({ n: 0, step: 1 });
  const [matches, setMatches] = useState({ current: -1, total: 0 });
  const onMatches = useCallback((current: number, total: number) => setMatches({ current, total }), []);
  const onInvalid = useCallback(() => setInvalid(body.id), [body.id]);

  const tooLarge = body.size > VIEW_LIMIT;
  const large = body.size > FORMAT_LIMIT && formatted !== body.id;
  const modes = modesOf(kind);
  let mode: BodyMode = modes.includes(chosen) ? chosen : modes[0];
  if (large && mode === "pretty") mode = "raw";
  const tree = mode === "pretty" && kind === "json" && invalid !== body.id;
  const needsText = !tooLarge && !tree && mode !== "preview";
  const limit = large ? RAW_PREVIEW : FORMAT_LIMIT;
  const text = useBodyText(body.id, limit, needsText);

  useEffect(() => {
    useResults.setState({ activeBody: { id: body.id, contentType: body.contentType } });
    // Gone with the tab: ⌥⌘S never saves a body no longer shown.
    return () => {
      if (useResults.getState().activeBody?.id === body.id) useResults.setState({ activeBody: null });
    };
  }, [body.id, body.contentType]);

  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") setNav((n) => ({ n: n.n + 1, step: e.shiftKey ? -1 : 1 }));
    if (e.key === "Escape") setQuery("");
  };
  const copy = async () => {
    try {
      const t = await fetchBody(body.id, FORMAT_LIMIT);
      await navigator.clipboard.writeText(kind === "binary" ? hexDump(t.bytes, t.bytes.length) : t.text);
      useUI.getState().toast({ kind: "success", text: "Body copied" });
    } catch (err) {
      useUI.getState().toast({ kind: "error", text: appError(err).message });
    }
  };

  let content;
  if (tooLarge) content = null;
  else if (mode === "preview") content = <Preview id={body.id} kind={kind} onOpenExternally={serverMode ? undefined : () => void openExternally(body.id)} />;
  else if (tree)
    content = (
      <PrettyJsonTree
        bodyId={body.id}
        query={searched}
        nav={nav}
        onMatches={onMatches}
        onInvalid={onInvalid}
        onAssert={(path) => void addFromBody(file, entry, body.id, path)}
        onCapture={(path) => void addFromBody(file, entry, body.id, path, "?")}
      />
    );
  else if (!text) content = <div className="body-note">Loading…</div>;
  else if (text.error) content = <div className="body-note">{text.error}</div>;
  else {
    const binary = kind === "binary" || kind === "image" || kind === "pdf";
    content = (
      <RawView
        text={binary ? hexDump(text.body!.bytes) : text.body!.text}
        kind={mode === "pretty" ? kind : "text"}
        query={searched}
        nav={nav}
        onMatches={onMatches}
      />
    );
  }

  return (
    <div className="body-tab">
      {!tooLarge && (
        <div className="body-bar">
          <div className="segmented" role="group" aria-label="Body view">
            {(["pretty", "raw", "preview"] as BodyMode[]).map((m) => (
              <button
                key={m}
                aria-pressed={mode === m}
                disabled={!modes.includes(m) || (m === "pretty" && large)}
                onClick={() => useResults.getState().setBodyMode(m)}
              >
                {modeLabel[m]}
              </button>
            ))}
          </div>
          {mode !== "preview" && (
            <label className="body-search">
              <span aria-hidden>⌕</span>
              <input value={query} placeholder="Search" aria-label="Search the body" onChange={(e) => setQuery(e.target.value)} onKeyDown={onKey} />
              {query && <span className="count mono">{matches.total ? `${matches.current + 1} of ${matches.total}` : "0 of 0"}</span>}
            </label>
          )}
          <button className="btn-ghost icon" title="Copy the body" aria-label="Copy the body" disabled={large} onClick={() => void copy()}>
            ⧉
          </button>
          <button className="btn-ghost icon" title={`Save response to file ${saveKeys ?? ""}`} aria-label="Save response to file" onClick={() => void saveBody(body)}>
            ⤓
          </button>
        </div>
      )}
      {(large || tooLarge) && (
        <LargeBody
          size={body.size}
          contentType={body.contentType}
          saveKeys={saveKeys}
          onSave={() => void saveBody(body)}
          onOpen={serverMode ? undefined : () => void openExternally(body.id)}
          onFormat={() => {
            setFormatted(body.id);
            useResults.getState().setBodyMode("pretty");
          }}
        />
      )}
      {invalid === body.id && mode === "pretty" && kind === "json" && <p className="body-note">Not valid JSON: showing the text.</p>}
      {content}
    </div>
  );
}
