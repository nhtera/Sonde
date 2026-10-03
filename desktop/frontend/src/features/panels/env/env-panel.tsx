// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Environments: the list (the side), and the environment shown with its
// variables to edit (the main area). Each edit is written to where the
// variable comes from (sonde.yaml, a variables file, a secrets file); a
// session override is used by this window's runs only (--variable).

import { LockIcon } from "../../../components/icons";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { useState } from "react";
import { create } from "zustand";
import { appError, Envs, type Var } from "../../../lib/api";
import { useEnv } from "../../../state/env";
import { useTabs } from "../../../state/tabs";
import { useUI } from "../../../state/ui";
import { ask, confirm } from "../../../components/ask";
import { SuggestInput } from "../../form/suggest-input";
import { label } from "../../../app/keymap/keymap-manager";

/** The environment the panel shows (not the one runs use). */
const useShown = create<{ env: string; set(env: string): void }>((set) => ({ env: "", set: (env) => set({ env }) }));

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

/** The JSON a typed value is written as: the variable's type kept. */
export function rawValue(text: string, type: string): unknown {
  if (type === "number" && text.trim() !== "" && Number.isFinite(Number(text))) return Number(text);
  if (type === "boolean" && (text === "true" || text === "false")) return text === "true";
  if (type === "null" && text === "null") return null;
  return text;
}

/** The precedence of a variable's value, highest first. */
export const precedence: [string, string][] = [
  ["Captures from earlier requests", ""],
  ["Entry [Options] variable", ""],
  ["--variable", "CLI · session overrides"],
  ["Data row", "--data"],
  ["--variables-file", "CLI"],
  ["SONDE_VARIABLE_* env vars", "shell"],
  ["sonde.yaml environment", "variables, then variables_files · secrets files here too"],
];

function shownEnv(): string {
  const { project, current } = useEnv.getState();
  const env = useShown.getState().env;
  return project?.envs?.some((e) => e.name === env) ? env : current;
}

export function EnvSide() {
  const project = useEnv((s) => s.project);
  const current = useEnv((s) => s.current);
  const shown = useShown((s) => s.env) || current;
  const env = project?.envs?.find((e) => e.name === shown);
  return (
    <div className="panel-body">
      <div className="panel-head">
        <h2>Environments</h2>
        {project?.config && (
          <button className="btn-ghost accent" onClick={() => {
              // The editor shows in the Files panel (Environments has its own main view).
              useUI.getState().setPanel("files");
              void useTabs.getState().open(project.config);
            }}>
            Edit {project.config}
          </button>
        )}
      </div>
      <ul className="env-list" aria-label="Environments">
        {(project?.envs ?? []).map((e) => (
          <li key={e.name}>
            <button aria-pressed={e.name === shown} onClick={() => useShown.getState().set(e.name)}>
              <i className="dot" style={{ background: e.name === current ? "var(--pass)" : "var(--faint)" }} />
              <span>{e.name}</span>
              {e.default && <span className="muted small">default</span>}
            </button>
          </li>
        ))}
        {(project?.envs ?? []).length === 0 && <li className="muted">No sonde.yaml environments.</li>}
      </ul>
      {env && (
        <div className="panel-section">
          <h3>Variables in {env.name}</h3>
          <ul className="var-summary">
            {(env.variables ?? []).map((v) => (
              <li key={v.name}>
                <span className="mono name">{v.name}</span>
                <span className="muted small">{v.source}</span>
                <span className="mono value">{v.secret ? "*** · locked" : v.value}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="panel-section">
        <h3>Precedence, highest first</h3>
        <ol className="precedence">
          {precedence.map(([name, note]) => (
            <li key={name}>
              {name}
              {note && <span className="muted small"> {note}</span>}
            </li>
          ))}
        </ol>
      </div>
    </div>
  );
}

export function EnvMain() {
  const project = useEnv((s) => s.project);
  const overrides = useEnv((s) => s.overrides);
  useShown((s) => s.env);
  const name = shownEnv();
  const env = project?.envs?.find((e) => e.name === name);
  // The variable being added: a change to save (Save ⌘S) or discard.
  const [draft, setDraft] = useState<Draft | null>(null);
  if (!env) return <div className="env-main empty"><p className="muted">No environment to show.</p></div>;
  const sources = [...new Set((env.variables ?? []).map((v) => v.source))];
  const override = (v: Var) => overrides.items?.find((o) => o.name === v.name && o.source === "session");
  const set = (v: Var, text: string) => void Envs.SetVariable(env.name, v.name, rawValue(text, v.type) as never).catch(fail);
  const save = async () => {
    if (!draft) return;
    const n = draft.name.trim();
    if (!n) return setDraft(null);
    try {
      // A secret goes to the secrets file at once, never to sonde.yaml.
      if (draft.secret) await Envs.SetSecret(env.name, n, draft.value);
      else await Envs.SetVariable(env.name, n, draft.value as never);
      setDraft(null);
    } catch (err) {
      fail(err);
    }
  };
  return (
    <div
      className="env-main"
      onKeyDown={(e) => {
        if (!draft || e.nativeEvent.isComposing) return;
        if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
          // ⌘S saves the variable here, not an editor tab.
          e.preventDefault();
          e.stopPropagation();
          void save();
        } else if (e.key === "Escape") setDraft(null);
      }}
    >
      <header className="env-head">
        <div className="env-title">
          <h1>
            <i className="dot" style={{ background: "var(--pass)" }} /> {env.name}
          </h1>
          <p className="muted">
            {sources.join(" · ")} · {env.variables?.length ?? 0} variables
          </p>
          {env.error && <p className="fail">{env.error}</p>}
        </div>
        {draft && (
          <div className="env-save">
            <button className="btn" onClick={() => setDraft(null)}>
              Discard
            </button>
            <button className="btn-primary" onClick={() => void save()}>
              Save<kbd aria-hidden>{label("$mod+KeyS")}</kbd>
            </button>
          </div>
        )}
      </header>
      <div className="env-box">
        <table className="env-table" aria-label={`Variables in ${env.name}`}>
          <thead>
            <tr>
              <th>Name</th>
              <th>Value</th>
              <th>Session override</th>
              <th>Source</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(env.variables ?? []).map((v) => {
              const o = override(v);
              return (
                <tr key={v.name}>
                  <td className="mono name">{v.name}</td>
                  <td>
                    {v.secret ? (
                      <SuggestInput label={`${v.name} value`} className="mono" value="" placeholder="🔒 *** · type to replace" onCommit={(t) => t && set(v, t)} />
                    ) : (
                      <SuggestInput label={`${v.name} value`} className="mono" value={v.value} onCommit={(t) => set(v, t)} />
                    )}
                  </td>
                  <td>
                    {o ? (
                      <span className="override-chip mono" title={o.flag}>
                        {v.secret ? "***" : (o.origin ?? "set")}
                        <button aria-label={`Remove the override of ${v.name}`} onClick={() => void Envs.RemoveOverride(v.name)}>
                          ×
                        </button>
                      </span>
                    ) : (
                      <span className="muted">–</span>
                    )}
                  </td>
                  <td className="muted small">{v.source}</td>
                  <td>
                    <VarMenu env={env.name} v={v} />
                  </td>
                </tr>
              );
            })}
            {draft && <NewVariable draft={draft} secretsFile={env.secretsFile} onChange={setDraft} onSave={() => void save()} />}
          </tbody>
        </table>
      </div>
      {!draft && (
        <button className="add-row" onClick={() => setDraft({ name: "", value: "", secret: false })}>
          + Add variable
        </button>
      )}
      <p className="form-note">
        <span className="override-chip mono">override</span> Session overrides are used by runs in this window (as --variable): they are never written to a file, and go when you quit.
      </p>
    </div>
  );
}

function VarMenu({ env, v }: { env: string; v: Var }) {
  const overrideIt = async () => {
    const text = await ask({ title: `Override ${v.name} for this session`, label: "Value", submit: "Override" });
    if (text !== null) await Envs.SetOverride(env, v.name, text).catch(fail);
  };
  const remove = async () => {
    if (await confirm({ title: `Delete ${v.name}?`, message: `It is removed from ${v.source}.`, submit: "Delete" })) {
      await Envs.RemoveVariable(env, v.name).catch(fail);
    }
  };
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button className="btn-ghost" aria-label={`${v.name} actions`}>
          ⋯
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="menu env-menu" align="end" sideOffset={4}>
          {!v.secret && (
            <Menu.Item className="menu-item" onSelect={() => setTimeout(() => (document.querySelector(`[aria-label="${CSS.escape(v.name)} value"]`) as HTMLInputElement | null)?.focus())}>
              Edit value
              <span className="hint">↵</span>
            </Menu.Item>
          )}
          <Menu.Item className="menu-item tall" onSelect={() => void overrideIt()}>
            <b>Override for this session</b>
            <span className="sub">Used by runs in this window. Not written to any file.</span>
          </Menu.Item>
          {!v.secret && (
            <Menu.Item className="menu-item tall" onSelect={() => void Envs.MarkSecret(env, v.name).catch(fail)}>
              <b>Mark as secret</b>
              <span className="sub">Moves it to the environment's secrets file (keep that file in .gitignore). Shown as *** from then on.</span>
            </Menu.Item>
          )}
          <Menu.Item className="menu-item" onSelect={() => void navigator.clipboard.writeText(v.name)}>
            Copy name
          </Menu.Item>
          <Menu.Separator className="menu-sep" />
          <Menu.Item className="menu-item danger" onSelect={() => void remove()}>
            Delete
          </Menu.Item>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** A variable being added, not saved yet. */
interface Draft {
  name: string;
  value: string;
  secret: boolean;
}

/** The variable being added: a row of the table, each field in its own
 * column, then the secret note. ↵ or Save adds it; Esc or Discard drops
 * it. */
function NewVariable({ draft, secretsFile, onChange, onSave }: { draft: Draft; secretsFile: string; onChange(d: Draft): void; onSave(): void }) {
  const { secret } = draft;
  return (
    <>
      <tr className="new-variable" onKeyDown={(e) => e.key === "Enter" && !e.nativeEvent.isComposing && onSave()}>
        <td>
          <input
            className="mono"
            name="name"
            aria-label="New variable name"
            placeholder="name"
            autoFocus
            spellCheck={false}
            value={draft.name}
            onChange={(e) => onChange({ ...draft, name: e.target.value })}
          />
        </td>
        <td>
          <input
            className={`mono ${secret ? "secret" : ""}`}
            name="value"
            aria-label="New variable value"
            placeholder="value"
            type={secret ? "password" : "text"}
            spellCheck={false}
            value={draft.value}
            onChange={(e) => onChange({ ...draft, value: e.target.value })}
          />
        </td>
        <td>
          <label className="secret-check">
            <input type="checkbox" role="switch" className="switch" aria-label="Secret" checked={secret} onChange={(e) => onChange({ ...draft, secret: e.target.checked })} /> Secret
          </label>
        </td>
        <td>{secret && secretsFile && <span className="secret-to">→ {secretsFile}</span>}</td>
        <td />
      </tr>
      {secret && (
        <tr className="new-variable-foot">
          <td colSpan={5}>
            <p className="secret-note">
              <LockIcon />
              <span>Secret values are written to {secretsFile || "the environment's secrets file"} and never shown again or kept in history. Keep that file in .gitignore.</span>
            </p>
          </td>
        </tr>
      )}
    </>
  );
}
