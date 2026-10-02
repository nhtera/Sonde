// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Auth writes plain file lines, read back from them: Bearer is an
// Authorization header, Basic is [BasicAuth], an API key a header or a
// [Query] row, a client certificate [Options] cert and key. OAuth 2 is a
// login request before this one, capturing the token.

import { useEffect, useState, type ReactNode } from "react";
import { Vars, type ScopeVar } from "../../../lib/api";
import { useEnv } from "../../../state/env";
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

/** A line of file text in parts, colored as the editor colors them. */
type Seg = [kind: "key" | "sec" | "var" | "str" | "plain", text: string];

/** The parts of a value: {{variables}} among plain text. */
function valueSegs(text: string): Seg[] {
  return text.split(/(\{\{[^}]*\}\})/).filter(Boolean).map((t): Seg => [t.startsWith("{{") ? "var" : "plain", t]);
}

function Segs({ segs }: { segs: Seg[] }) {
  return (
    <>
      {segs.map(([kind, text], i) => (
        <span key={i} className={`tk-${kind}`}>
          {text}
        </span>
      ))}
    </>
  );
}

/** What each other type writes, for the table under the fields. */
const others: [string, Seg[]][] = [
  ["Basic", [["sec", "[BasicAuth]"], ["plain", "  "], ["var", "{{user}}"], ["plain", ": "], ["var", "{{password}}"]]],
  ["API key · header", [["key", "X-API-Key"], ["plain", ": "], ["var", "{{api_key}}"]]],
  ["API key · query", [["sec", "[Query]"], ["plain", "  api_key: "], ["var", "{{api_key}}"]]],
  ["Client certificate", [["sec", "[Options]"], ["plain", "  cert: "], ["str", "certs/client.pem"], ["plain", "  key: "], ["str", "certs/client.key"]]],
];

/** The variable a field's {{name}} names, as a run would see it. */
function useVar(file: string, text: string): ScopeVar | undefined {
  const name = /^\{\{\s*([\w.-]+)\s*\}\}$/.exec(text.trim())?.[1];
  const env = useEnv((s) => s.current);
  // Kept with the name it was found for: a stale answer never shows.
  const [found, setFound] = useState<{ name: string; v?: ScopeVar }>({ name: "" });
  useEffect(() => {
    if (!name) return;
    let live = true;
    void Vars.For(file, env).then(
      (l) => live && setFound({ name, v: (l ?? []).find((v) => v.name === name) }),
      () => live && setFound({ name }),
    );
    return () => {
      live = false;
    };
  }, [file, env, name]);
  return name && found.name === name ? found.v : undefined;
}

export function AuthTab({ file, entry }: { file: string; entry: EntryModel }) {
  const n = entry.Index;
  const auth = authOf(entry);
  const vars = varSuggester(file);
  const choose = (kind: AuthKind) => kind !== auth.kind && void formEdit(file, ...removeOps(n, auth), ...addOps(n, kind));
  const row = auth.sec !== undefined && auth.index !== undefined ? rowsOf(entry, auth.sec)[auth.index] : undefined;
  const opt = (key: string) => rowsOf(entry, "options").findIndex((r) => r.Key === key);

  const token = auth.kind === "bearer" ? row!.Value.replace(/^Bearer\s+/, "") : "";
  const tokenVar = useVar(file, token);
  // The lines this auth writes, the auth's own marked.
  let lines: { segs: Seg[]; mark?: boolean }[] = [];
  let detail;
  switch (auth.kind) {
    case "bearer":
      lines = [{ segs: [["key", row!.Key], ["plain", ": "], ...valueSegs(row!.Value)], mark: true }];
      detail = (
        <Field title="Bearer token" about="Sends an Authorization header. Point it at a variable so the token never lands in the file.">
          <span className="auth-label">Token</span>
          <span className="auth-input">
            <SuggestInput label="Token" className="mono" value={token} suggest={vars} onCommit={(v) => void setRow(file, n, "headers", auth.index!, row!.Key, `Bearer ${v.trim()}`)} />
            {tokenVar && <span className="src">{tokenVar.origin || tokenVar.source}</span>}
          </span>
          {tokenVar && <span className="auth-value mono">= {tokenVar.secret ? "***" : JSON.stringify(tokenVar.display)}</span>}
        </Field>
      );
      break;
    case "basic":
      lines = [{ segs: [["sec", "[BasicAuth]"]], mark: true }, { segs: [...valueSegs(row!.Key), ["plain", ": "], ...valueSegs(row!.Value)] }];
      detail = (
        <Field title="Basic" about="Written to [BasicAuth] as user: password.">
          <SuggestInput label="User" className="mono" value={row!.Key} suggest={vars} onCommit={(v) => void setRow(file, n, "basic-auth", 0, v.trim(), row!.Value)} />
          <SuggestInput label="Password" className="mono" value={row!.Value} suggest={vars} onCommit={(v) => void setRow(file, n, "basic-auth", 0, row!.Key, v.trim())} />
        </Field>
      );
      break;
    case "apikey":
      lines =
        auth.sec === "query"
          ? [{ segs: [["sec", "[Query]"]] }, { segs: [["plain", `${row!.Key}: `], ...valueSegs(row!.Value)], mark: true }]
          : [{ segs: [["key", row!.Key], ["plain", ": "], ...valueSegs(row!.Value)], mark: true }];
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
      lines = [
        { segs: [["sec", "[Options]"]] },
        ...(["cert", "key"] as const).filter((k) => opt(k) >= 0).map((k) => ({ segs: [["plain", `${k}: `], ["str", rowsOf(entry, "options")[opt(k)].Value]] as Seg[], mark: true })),
      ];
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
        {lines.length > 0 && (
          <section className="auth-writes" aria-label="Write-back preview">
            <div className="section-note">
              <span>Writes to request {n}</span>
              <span className="sub">same line the Headers tab shows</span>
            </div>
            <div className="code-segs">
              {lines.map((l, i) => (
                <div key={i} className={l.mark ? "wb-mark" : undefined}>
                  <Segs segs={l.segs} />
                </div>
              ))}
            </div>
          </section>
        )}
        <div className="auth-others">
          <span className="auth-label">What the other types write</span>
          <dl>
            {others.map(([name, segs]) => (
              <div key={name}>
                <dt>{name}</dt>
                <dd className="code-segs">
                  <Segs segs={segs} />
                </dd>
              </div>
            ))}
          </dl>
        </div>
      </div>
    </div>
  );
}

function Field({ title, about, children }: { title: string; about: string; children: ReactNode }) {
  return (
    <section className="auth-field">
      <b>{title}</b>
      <p>{about}</p>
      <div className="auth-inputs">{children}</div>
    </section>
  );
}
