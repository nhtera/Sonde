// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The request body: none, form-data ([Multipart]), x-www-form-urlencoded
// ([Form]), raw JSON, XML or text, binary (a `file,path;` body) or
// GraphQL (a ```graphql body with its variables). Changing the kind is one
// edit: the old kind's lines go, the new kind's come.

import { useState } from "react";
import { graphqlLanguage, jsonBodyLanguage, xmlBodyLanguage } from "../../../../lang";
import { CodeField } from "../../code-field";
import { formEdit, setRow, type Op } from "../../edit";
import { KvGrid } from "../../kv-grid";
import { bodyKindOf, fileRef, fileValue, graphqlBody, graphqlParts, multilineText, rowsOf, type BodyKind, type EntryModel, type ModelRow } from "../../model";
import { SuggestInput } from "../../suggest-input";
import { OutsideNotice, SelectFile, type Outside } from "./file-picker";

const kinds: { kind: BodyKind; label: string }[] = [
  { kind: "none", label: "none" },
  { kind: "form-data", label: "form-data" },
  { kind: "urlencoded", label: "x-www-form-urlencoded" },
  { kind: "json", label: "raw" },
  { kind: "binary", label: "binary" },
  { kind: "graphql", label: "GraphQL" },
];

/** raw's kinds, picked beside it. */
const raws: { kind: BodyKind; label: string }[] = [
  { kind: "json", label: "JSON" },
  { kind: "xml", label: "XML" },
  { kind: "text", label: "Text" },
];
const isRaw = (k: BodyKind) => raws.some((r) => r.kind === k);

/** The first body of each kind. */
const starters: Partial<Record<BodyKind, string>> = {
  json: "{\n}",
  xml: "<request/>",
  text: "```\ntext\n```",
  binary: "file,data.bin;",
  graphql: graphqlBody("query {\n  __typename\n}", ""),
};

/** The Content-Type each body kind sends. */
const contentTypes: Partial<Record<BodyKind, RegExp>> = {
  json: /^application\/(.+\+)?json\b/i,
  graphql: /^application\/json\b/i,
  xml: /^(application|text)\/(.+\+)?xml\b/i,
  urlencoded: /^application\/x-www-form-urlencoded\b/i,
  "form-data": /^multipart\/form-data\b/i,
  text: /^text\/plain\b/i,
};

/** The ops that change entry's body from its kind to kind. A
 * Content-Type header that described the old kind goes too: Sonde sends
 * the new kind's. */
export function bodyKindOps(entry: EntryModel, kind: BodyKind): Op[] {
  const n = entry.Index;
  const from = bodyKindOf(entry);
  if (from === kind) return [];
  const ops: Op[] = [];
  const stale = contentTypes[from];
  const ct = rowsOf(entry, "headers").findIndex((r) => !r.Disabled && /^content-type$/i.test(r.Key) && !!stale?.test(r.Value));
  if (ct >= 0) ops.push({ kind: "removeRow", entry: n, section: "headers", index: ct });
  if (rowsOf(entry, "multipart").length || hasSection(entry, "multipart")) ops.push({ kind: "removeSection", entry: n, section: "multipart" });
  if (rowsOf(entry, "form").length || hasSection(entry, "form")) ops.push({ kind: "removeSection", entry: n, section: "form" });
  if (entry.HasBody) ops.push({ kind: "setBody", entry: n, value: "" });
  if (kind === "form-data") ops.push({ kind: "addRow", entry: n, section: "multipart", key: "field", value: "value" });
  else if (kind === "urlencoded") ops.push({ kind: "addRow", entry: n, section: "form", key: "field", value: "value" });
  else if (starters[kind]) ops.push({ kind: "setBody", entry: n, value: starters[kind] });
  return ops;
}

const hasSection = (e: EntryModel, sec: string) => !!(e.Rows as Record<string, unknown> | null)?.[sec];

export function BodyTab({ file, entry }: { file: string; entry: EntryModel }) {
  const kind = bodyKindOf(entry);
  const n = entry.Index;
  const setBody = (value: string) => void formEdit(file, { kind: "setBody", entry: n, value });
  let content;
  switch (kind) {
    case "form-data":
      content = <FormData file={file} entry={entry} />;
      break;
    case "urlencoded":
      content = <KvGrid file={file} entry={entry} sec="form" />;
      break;
    case "json":
      content = <CodeField label="JSON body" language={jsonBodyLanguage} value={entry.Body} onCommit={(v) => setBody(v.trim() || "{}")} minLines={8} />;
      break;
    case "xml":
      content = <CodeField label="XML body" language={xmlBodyLanguage} value={entry.Body} onCommit={(v) => setBody(v.trim() || "<request/>")} minLines={8} />;
      break;
    case "text":
      content = <CodeField label="Text body" value={multilineText(entry.Body)} onCommit={(v) => setBody("```\n" + v.replace(/\n+$/, "") + "\n```")} minLines={8} />;
      break;
    case "binary":
      content = <BinaryBody file={file} entry={entry} />;
      break;
    case "graphql": {
      const { query, variables } = graphqlParts(entry.Body);
      content = (
        <>
          <div className="graphql-title">Query and variables</div>
          <div className="graphql-body">
            <div className="gq-pane">
              <div className="pane-head">Query</div>
              <CodeField label="GraphQL query" language={graphqlLanguage} value={query} onCommit={(q) => setBody(graphqlBody(q, variables))} minLines={18} />
            </div>
            <div className="gq-pane">
              <div className="pane-head">
                Variables <span className="kind">JSON</span>
              </div>
              <CodeField label="GraphQL variables" language={jsonBodyLanguage} value={variables} onCommit={(v) => setBody(graphqlBody(query, v))} minLines={14} />
              <p className="gq-note">
                Stored in the file as a <code>```graphql</code> body with a <code>variables</code> block.
              </p>
            </div>
          </div>
        </>
      );
      break;
    }
    case "other":
      content = (
        <>
          <p className="form-note">This body is a {entry.Body.trim().split("\n")[0]} block: edit it in Text (⌘E) to keep it as written.</p>
          <pre className="body-readonly mono">{entry.Body}</pre>
        </>
      );
      break;
    default:
      content = <p className="form-note">This request sends no body.</p>;
  }
  return (
    <div className="form-tab body-tab">
      <div className="body-kinds" role="radiogroup" aria-label="Body type">
        {kinds.map((k) => {
          const raw = k.kind === "json";
          const on = raw ? isRaw(kind) : kind === k.kind;
          return (
            <label key={k.kind} className={`body-kind${on ? " on" : ""}`}>
              <input type="radio" name={`body-${n}`} aria-label={k.label} checked={on} onChange={() => void formEdit(file, ...bodyKindOps(entry, k.kind))} />
              {k.label}
              {raw && (
                <span className="raw-pick">
                  <select
                    className="raw-kind"
                    aria-label="Raw body type"
                    value={isRaw(kind) ? kind : "json"}
                    onChange={(e) => void formEdit(file, ...bodyKindOps(entry, e.target.value as BodyKind))}
                  >
                    {raws.map((r) => (
                      <option key={r.kind} value={r.kind}>
                        {r.label}
                      </option>
                    ))}
                  </select>
                  <span aria-hidden>▾</span>
                </span>
              )}
            </label>
          );
        })}
      </div>
      {content}
    </div>
  );
}

function FileIcon() {
  return (
    <svg className="file-icon" width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.1" aria-hidden>
      <path d="M3 1.5h4l2.5 2.5v6.5H3z" />
      <path d="M7 1.5V4h2.5" />
    </svg>
  );
}

/** [Multipart] rows: text values, or files with a content type. */
function FormData({ file, entry }: { file: string; entry: EntryModel }) {
  const [outside, setOutside] = useState<{ row: number; o: Outside } | null>(null);
  const n = entry.Index;
  const write = (r: ModelRow, i: number, value: string) => void setRow(file, n, "multipart", i, r.Key, value);
  return (
    <>
      <KvGrid
        file={file}
        entry={entry}
        sec="multipart"
        addLabel="+ Add row"
        valueHead={
          <span className="multipart-head">
            <span>Type</span>
            <span>Value</span>
            <span>Content type</span>
          </span>
        }
        renderValue={(r, i) => {
          const f = fileRef(r.Value);
          const asFile = async (path: string, ct = f?.type ?? "") => write(r, i, await fileValue(path, ct));
          return (
            <span className="multipart-value">
              <span className="segmented" role="group" aria-label={`${r.Key} type`}>
                <button aria-pressed={!f} onClick={() => f && write(r, i, "value")}>
                  Text
                </button>
                <button aria-pressed={!!f} onClick={() => !f && void asFile("data.txt")}>
                  File
                </button>
              </span>
              {f ? (
                <>
                  <span className="file-field">
                    <FileIcon />
                    <SuggestInput label={`${r.Key} file`} className="mono" value={f.path} onCommit={(p) => p.trim() && void asFile(p.trim())} />
                  </span>
                  <SelectFile onPath={(p) => void asFile(p)} onOutside={(o) => setOutside({ row: i, o })} />
                  <SuggestInput label={`${r.Key} content type`} className="mono ct" highlight={false} value={f.type} onCommit={(ct) => void asFile(f.path, ct.trim())} />
                </>
              ) : (
                <>
                  <SuggestInput label={`${r.Key} value`} className="mono" highlight="vars" value={r.Value} onCommit={(v) => write(r, i, v)} />
                  {/* A text field has no content type: its cell stays empty. */}
                  <span className="ct ct-none" aria-hidden />
                </>
              )}
            </span>
          );
        }}
      />
      {outside && (
        <OutsideNotice
          outside={outside.o}
          onDismiss={() => setOutside(null)}
          onCopied={(path) => {
            const r = rowsOf(entry, "multipart")[outside.row];
            setOutside(null);
            if (r) void fileValue(path, fileRef(r.Value)?.type ?? "").then((v) => write(r, outside.row, v));
          }}
        />
      )}
      <p className="form-note info">
        <span className="i" aria-hidden>
          i
        </span>
        <span>
          Paths are relative to the project folder. The CLI resolves them from each file's own folder and rejects .., so Copy as › sonde adds <code>--file-root .</code>
        </span>
      </p>
    </>
  );
}

/** A `file,path;` body. */
function BinaryBody({ file, entry }: { file: string; entry: EntryModel }) {
  const [outside, setOutside] = useState<Outside | null>(null);
  const path = fileRef(entry.Body)?.path;
  const set = (p: string) => void fileValue(p).then((value) => formEdit(file, { kind: "setBody", entry: entry.Index, value }));
  if (path === undefined) return <p className="form-note">The body is inline bytes (hex or base64): edit it in Text (⌘E).</p>;
  return (
    <div className="binary-body">
      <SuggestInput label="Body file" className="mono" value={path} onCommit={(p) => p.trim() && set(p.trim())} />
      <SelectFile onPath={set} onOutside={setOutside} />
      {outside && (
        <OutsideNotice
          outside={outside}
          onDismiss={() => setOutside(null)}
          onCopied={(p) => {
            setOutside(null);
            set(p);
          }}
        />
      )}
    </div>
  );
}
