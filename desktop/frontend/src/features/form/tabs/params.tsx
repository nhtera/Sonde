// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";
import { formEdit } from "../edit";
import { KvGrid } from "../kv-grid";
import type { EntryModel } from "../model";
import { SuggestInput } from "../suggest-input";

const noop = () => {};

/** Query parameters, written to [Query] (the URL stays readable: the
 * write-back preview says so). Params the URL holds itself (a pasted
 * or imported URL) are listed first, as sent: edited in the URL bar, or
 * moved to [Query] rows in one change. */
export function ParamsTab({ file, entry }: { file: string; entry: EntryModel }) {
  const inURL = entry.URLQuery ?? [];
  // A second click would find no query string left.
  const [moving, setMoving] = useState(false);
  const move = async () => {
    setMoving(true);
    try {
      await formEdit(file, { kind: "moveURLQuery", entry: entry.Index });
    } finally {
      setMoving(false);
    }
  };
  return (
    <div className="form-tab">
      {inURL.length > 0 && (
        <div className="kv-grid url-query" role="table" aria-label="URL query params">
          <div className="kv-head url-query-head" role="row">
            <span />
            <span role="columnheader">In the URL</span>
            <span className="url-query-action">
              {entry.URLQueryKept ? (
                <span className="url-query-kept" title={`Kept in the URL: ${entry.URLQueryKept}`}>
                  edit in the URL bar · {entry.URLQueryKept}
                </span>
              ) : (
                <button className="btn-ghost accent" disabled={moving} onClick={() => void move()}>
                  Move to [Query]
                </button>
              )}
            </span>
            <span />
          </div>
          {inURL.map((p, i) => (
            <div key={`${i}:${p.Key}`} className="kv-row managed" role="row">
              <span />
              <SuggestInput label={`URL param ${i + 1}`} className="mono" value={p.Key} onCommit={noop} readOnly />
              <SuggestInput label={`URL param ${i + 1} value`} className="mono" highlight="vars" value={p.Value} onCommit={noop} readOnly />
              <span />
            </div>
          ))}
        </div>
      )}
      <KvGrid file={file} entry={entry} sec="query" />
    </div>
  );
}
