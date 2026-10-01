// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The form step: the input (pasted or picked), the kind's options, where
// the files go, and the preview of exactly what is written.

import { useEffect, useState } from "react";
import { appError } from "../../lib/api";
import { serverMode } from "../../lib/mode";
import { useEnv } from "../../state/env";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { pickInput } from "./pick";
import { liftOf, requestFile, useImport } from "./state";

const layouts = {
  postman: [
    { id: "request", title: "One file per request", sub: "Closest to Postman: one request, one tab." },
    { id: "folder", title: "One file per folder", sub: "Requests run top to bottom, so captures carry over." },
  ],
  openapi: [
    { id: "tag", title: "By tag", sub: "A file per tag." },
    { id: "path", title: "By path", sub: "A file per path." },
    { id: "flat", title: "One file", sub: "Every operation in one file." },
  ],
} as const;

const pasteHint: Record<string, string> = {
  curl: "Paste one or more curl commands",
  http: "Paste a .http file",
  postman: "Paste a collection (JSON)",
  opencollection: "Paste a collection.yml",
  openapi: "Paste a spec (YAML or JSON)",
};

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

export function SourceForm() {
  const req = useImport((s) => s.req);
  const names = useImport((s) => s.names);
  const preview = useImport((s) => s.preview);
  const error = useImport((s) => s.error);
  const busy = useImport((s) => s.busy);
  const overwrite = useImport((s) => s.overwrite);
  const envs = useEnv((s) => s.project?.envs ?? []);
  const active = useTabs((s) => s.active);
  const [shown, setShown] = useState(0);
  const update = useImport.getState().update;
  const envIds = req.environments ?? [];
  const lift = liftOf(preview?.candidates ?? [], req.lift, req.env);

  const pick = async (dir = false) => {
    try {
      const input = await pickInput(req.kind, "Import from", dir);
      if (input) update({ input: input.id, text: "" }, { [input.id]: input.name });
    } catch (err) {
      fail(err);
    }
  };
  const addEnv = async () => {
    try {
      const input = await pickInput(req.kind === "postman" ? "postman" : "http", req.kind === "postman" ? "Postman environment" : "Env file");
      if (input) update({ environments: [...envIds, input.id] }, { [input.id]: input.name });
    } catch (err) {
      fail(err);
    }
  };
  const files = preview?.files ?? [];
  const warnings = preview?.warnings ?? [];
  const exists = files.filter((f) => f.exists);
  const file = files[Math.min(shown, files.length - 1)];
  const layout = req.kind === "postman" || req.kind === "openapi" ? layouts[req.kind] : null;
  const lifting = (preview?.candidates?.length ?? 0) > 0;
  return (
    <div className="import-form">
      <div className="import-source">
        {req.input ? (
          <div className="picked">
            <span className="mono">{names[req.input] ?? "the file picked"}</span>
            <button className="btn-ghost" onClick={() => update({ input: "" })}>
              Remove
            </button>
          </div>
        ) : (
          <PasteBox key={req.kind} placeholder={pasteHint[req.kind]} />
        )}
        <div className="row-gap">
          <button className="btn" onClick={() => void pick()}>
            Choose file…
          </button>
          {req.kind === "opencollection" && !serverMode && (
            <button className="btn" onClick={() => void pick(true)}>
              Choose folder…
            </button>
          )}
        </div>
      </div>

      {(req.kind === "postman" || req.kind === "http") && (
        <div className="import-row">
          <span>{req.kind === "postman" ? "Environments" : "Env files"}</span>
          <div className="row-gap wrap">
            {envIds.map((id) => (
              <span key={id} className="chip mono">
                {names[id] ?? id}
                <button aria-label={`Remove ${names[id] ?? id}`} onClick={() => update({ environments: envIds.filter((e) => e !== id) })}>
                  ×
                </button>
              </span>
            ))}
            <button className="btn-ghost accent" onClick={() => void addEnv()}>
              + Add
            </button>
          </div>
        </div>
      )}

      {layout && (
        <div className="import-layouts" role="radiogroup" aria-label="Layout">
          {layout.map((l, i) => {
            const on = (req.group || layout[0].id) === l.id;
            return (
              <label key={l.id} className={`layout-card${on ? " on" : ""}`}>
                <input type="radio" name="layout" checked={on} onChange={() => update({ group: l.id })} />
                <span className="layout-title">
                  <b>{l.title}</b>
                  {i === 0 && <span className="accent small">default</span>}
                </span>
                <span className="muted small">{l.sub}</span>
              </label>
            );
          })}
        </div>
      )}
      {req.kind === "openapi" && (
        <label className="import-row">
          <span>Base URL variable</span>
          <input className="mono" value={req.baseUrlVar} placeholder="base_url" onChange={(e) => update({ baseUrlVar: e.target.value.trim() })} />
        </label>
      )}

      <label className="import-row">
        <span>Into</span>
        <FolderInput key={req.kind} />
      </label>
      <div className="import-row">
        <span>Format</span>
        <div className="segmented" role="group" aria-label="Format">
          {(["hurl", "sonde"] as const).map((x) => (
            <button key={x} aria-pressed={(req.ext || "hurl") === x} onClick={() => update({ ext: x })}>
              .{x}
            </button>
          ))}
        </div>
      </div>

      {lifting && (
        <fieldset className="import-secrets">
          <legend>Secrets: replaced by {"{{name}}"}, values written to the environment&apos;s secrets file</legend>
          {envs.length === 0 && <p className="muted small">The project has no sonde.yaml environments: the values stay in the file.</p>}
          {(preview?.candidates ?? []).map((c) => (
            <label key={c.id} className="check-row">
              <input
                type="checkbox"
                disabled={!req.env}
                checked={lift.includes(c.id)}
                onChange={(e) => useImport.getState().setLift(e.target.checked ? [...lift, c.id] : lift.filter((x) => x !== c.id))}
              />
              <span>
                {c.where} → <span className="mono">{`{{${c.name}}}`}</span>
              </span>
            </label>
          ))}
          <label className="import-row">
            <span>Environment</span>
            <select aria-label="Secrets environment" value={req.env} onChange={(e) => update({ env: e.target.value })}>
              {!req.env && <option value="">{envs.length === 0 ? "none" : "Choose an environment"}</option>}
              {envs.map((e) => (
                <option key={e.name} value={e.name}>
                  {e.name}
                </option>
              ))}
            </select>
          </label>
        </fieldset>
      )}

      {error && <p className="run-error">{error}</p>}
      {preview && (
        <div className="import-preview">
          <div className="section-note">
            <span>
              {preview.counts.requests} request{preview.counts.requests === 1 ? "" : "s"} → {files.length} file{files.length === 1 ? "" : "s"}
              {preview.project === "created" && " · sonde.yaml"}
              {preview.project === "kept" && " · sonde.yaml kept as it is"}
              {warnings.length > 0 && ` · ${warnings.length} note${warnings.length === 1 ? "" : "s"}`}
            </span>
          </div>
          <ul className="import-files" aria-label="Files written">
            {files.map((f, i) => (
              <li key={f.path} className={i === shown ? "on" : ""}>
                <button className="mono" onClick={() => setShown(i)}>
                  {f.path}
                </button>
                {f.secret && <span className="muted small">empty secrets stub</span>}
                {f.exists && (
                  <label className="warn small">
                    <input type="checkbox" checked={overwrite.includes(f.path)} onChange={(e) => useImport.getState().setOverwrite(f.path, e.target.checked)} /> exists: overwrite
                  </label>
                )}
              </li>
            ))}
          </ul>
          {file && (
            <pre className="mono import-text" aria-label="Preview">
              {file.text}
            </pre>
          )}
          {warnings.length > 0 && (
            <ul className="import-notes small">
              {warnings.slice(0, 50).map((w, i) => (
                <li key={i}>{w.message}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      <div className="dialog-actions">
        {exists.length > 0 && (
          <span className="muted small">
            {exists.length - exists.filter((f) => overwrite.includes(f.path)).length} existing file(s) kept
          </span>
        )}
        <button className="btn-ghost" onClick={() => useImport.getState().close()}>
          Cancel
        </button>
        {req.kind === "curl" && requestFile(active) && (
          <button className="btn" disabled={!preview || busy} onClick={() => void useImport.getState().insert(active!)}>
            Insert into {active!.split("/").pop()}
          </button>
        )}
        <button className="btn-primary" disabled={!preview || busy} onClick={() => void useImport.getState().write()}>
          Import
        </button>
      </div>
    </div>
  );
}

/** The pasted text, previewed after a pause in typing (150 ms). */
function PasteBox({ placeholder }: { placeholder: string }) {
  const [text, setText] = useState(() => useImport.getState().req.text);
  useEffect(() => {
    if (text === useImport.getState().req.text) return;
    const t = setTimeout(() => useImport.getState().update({ text, input: "" }), 150);
    return () => clearTimeout(t);
  }, [text]);
  return <textarea className="mono" aria-label="Pasted input" placeholder={placeholder} value={text} onChange={(e) => setText(e.target.value)} rows={5} />;
}

/** The folder written into, previewed after a pause in typing. */
function FolderInput() {
  const [folder, setFolder] = useState(() => useImport.getState().req.folder);
  useEffect(() => {
    if (folder === useImport.getState().req.folder) return;
    const t = setTimeout(() => useImport.getState().update({ folder }), 250);
    return () => clearTimeout(t);
  }, [folder]);
  return <input className="mono" aria-label="Into folder" value={folder} placeholder="the project folder" onChange={(e) => setFolder(e.target.value)} />;
}
