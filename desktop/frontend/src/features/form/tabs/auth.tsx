// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Auth writes plain file lines, read back from them: Bearer is an
// Authorization header, Basic is [BasicAuth], an API key a header or a
// [Query] row, a client certificate [Options] cert and key. OAuth 2 is a
// login request before this one, capturing the token.

import { formEdit, setRow, type Op } from "../edit";
import { authOf, escapeFilename, rowsOf, useForm, type Auth, type AuthKind, type EntryModel } from "../model";
import { SuggestInput, varSuggester } from "../suggest-input";

const kinds: { kind: AuthKind; label: string; where: string }[] = [
  { kind: "none", label: "None", where: "" },
  { kind: "bearer", label: "Bearer token", where: "header" },
  { kind: "basic", label: "Basic", where: "[BasicAuth]" },
  { kind: "apikey", label: "API key", where: "header or query" },
  { kind: "cert", label: "Client certificate", where: "cert + key" },
];

/** The ops that remove an auth's lines. */
function removeOps(n: number, a: Auth): Op[] {
  switch (a.kind) {
    case "bearer":
    case "apikey":
      return [{ kind: "removeRow", entry: n, section: a.sec, index: a.index }];
    case "basic":
      return [{ kind: "removeSection", entry: n, section: "basic-auth" }];
    case "cert":
      return [...(a.rows ?? [])].sort((x, y) => y - x).map((index) => ({ kind: "removeRow", entry: n, section: "options", index }));
  }
  return [];
}

/** The ops that write a new auth of kind. */
function addOps(n: number, kind: AuthKind): Op[] {
  switch (kind) {
    case "bearer":
      return [{ kind: "addRow", entry: n, section: "headers", key: "Authorization", value: "Bearer {{token}}" }];
    case "basic":
      return [{ kind: "addRow", entry: n, section: "basic-auth", key: "{{user}}", value: "{{password}}" }];
    case "apikey":
      return [{ kind: "addRow", entry: n, section: "headers", key: "X-API-Key", value: "{{api_key}}" }];
    case "cert":
      return [
        { kind: "addRow", entry: n, section: "options", key: "cert", value: "certs/client.pem" },
        { kind: "addRow", entry: n, section: "options", key: "key", value: "certs/client.key" },
      ];
  }
  return [];
}

export function AuthTab({ file, entry }: { file: string; entry: EntryModel }) {
  const n = entry.Index;
  const auth = authOf(entry);
  const vars = varSuggester(file);
  const choose = (kind: AuthKind) => kind !== auth.kind && void formEdit(file, ...removeOps(n, auth), ...addOps(n, kind));
  const row = auth.sec !== undefined && auth.index !== undefined ? rowsOf(entry, auth.sec)[auth.index] : undefined;
  const opt = (key: string) => rowsOf(entry, "options").findIndex((r) => r.Key === key);

  let detail;
  switch (auth.kind) {
    case "bearer":
      detail = (
        <Field title="Bearer token" about="Sends an Authorization header. Point it at a variable so the token never lands in the file.">
          <SuggestInput label="Token" className="mono" value={row!.Value.replace(/^Bearer\s+/, "")} suggest={vars} onCommit={(v) => void setRow(file, n, "headers", auth.index!, row!.Key, `Bearer ${v.trim()}`)} />
        </Field>
      );
      break;
    case "basic":
      detail = (
        <Field title="Basic" about="Written to [BasicAuth] as user: password.">
          <SuggestInput label="User" className="mono" value={row!.Key} suggest={vars} onCommit={(v) => void setRow(file, n, "basic-auth", 0, v.trim(), row!.Value)} />
          <SuggestInput label="Password" className="mono" value={row!.Value} suggest={vars} onCommit={(v) => void setRow(file, n, "basic-auth", 0, row!.Key, v.trim())} />
        </Field>
      );
      break;
    case "apikey":
      detail = (
        <Field title="API key" about="A header, or a [Query] row.">
          <SuggestInput label="Name" className="mono" value={row!.Key} onCommit={(v) => void setRow(file, n, auth.sec!, auth.index!, v.trim(), row!.Value)} />
          <SuggestInput label="Key" className="mono" value={row!.Value} suggest={vars} onCommit={(v) => void setRow(file, n, auth.sec!, auth.index!, row!.Key, v.trim())} />
          <div className="segmented" role="group" aria-label="Sent in">
            {(["headers", "query"] as const).map((sec) => (
              <button
                key={sec}
                aria-pressed={auth.sec === sec}
                onClick={() =>
                  sec !== auth.sec &&
                  void formEdit(file, { kind: "removeRow", entry: n, section: auth.sec, index: auth.index }, { kind: "addRow", entry: n, section: sec, key: row!.Key, value: row!.Value })
                }
              >
                {sec === "headers" ? "Header" : "Query"}
              </button>
            ))}
          </div>
        </Field>
      );
      break;
    case "cert":
      detail = (
        <Field title="Client certificate" about="Paths in the project; the key's contents are never read here.">
          {(["cert", "key"] as const).map((k) => (
            <SuggestInput
              key={k}
              label={k === "cert" ? "Certificate" : "Key"}
              className="mono"
              value={(rowsOf(entry, "options")[opt(k)]?.Value ?? "").replace(/\\(.)/g, "$1")}
              // A file option: its path written as a file name.
              onCommit={(v) => void escapeFilename(v.trim()).then((p) => setRow(file, n, "options", opt(k), k, p))}
            />
          ))}
        </Field>
      );
      break;
    default:
      detail = <p className="form-note">No credentials are sent. Pick a type to write its lines.</p>;
  }

  return (
    <div className="form-tab auth-tab">
      <div className="auth-kinds" role="radiogroup" aria-label="Auth type">
        {kinds.map((k) => (
          <label key={k.kind} className={`auth-kind${auth.kind === k.kind ? " on" : ""}`}>
            <input type="radio" name={`auth-${n}`} checked={auth.kind === k.kind} onChange={() => choose(k.kind)} />
            <span>{k.label}</span>
            <span className="where mono">{k.where}</span>
          </label>
        ))}
        <div className="oauth-card">
          <b>OAuth 2</b>
          <p>In Sonde, OAuth 2 is a login request plus a capture. The token lives in a variable, not a hidden store.</p>
          <button
            className="btn"
            onClick={() => void formEdit(file, { kind: "addLogin", entry: n, key: "token" }).then((ok) => ok && useForm.getState().select(file, n + 1))}
          >
            + Add login request
          </button>
        </div>
      </div>
      <div className="auth-detail">
        {detail}
        <div className="auth-others">
          <div className="section-note">
            <span>What the other types write</span>
          </div>
          <dl className="mono">
            <dt>Basic</dt>
            <dd>[BasicAuth] {"{{user}}: {{password}}"}</dd>
            <dt>API key · header</dt>
            <dd>X-API-Key: {"{{api_key}}"}</dd>
            <dt>API key · query</dt>
            <dd>[Query] api_key: {"{{api_key}}"}</dd>
            <dt>Client certificate</dt>
            <dd>[Options] cert: certs/client.pem key: certs/client.key</dd>
          </dl>
        </div>
      </div>
    </div>
  );
}

function Field({ title, about, children }: { title: string; about: string; children: React.ReactNode }) {
  return (
    <section className="auth-field">
      <b>{title}</b>
      <p>{about}</p>
      <div className="auth-inputs">{children}</div>
    </section>
  );
}
