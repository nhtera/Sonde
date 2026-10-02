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
  const [adding, setAdding] = useState(false);
  if (!env) return <div className="env-main empty"><p className="muted">No environment to show.</p></div>;
  const sources = [...new Set((env.variables ?? []).map((v) => v.source))];
  const override = (v: Var) => overrides.items?.find((o) => o.name === v.name && o.source === "session");
  const set = (v: Var, text: string) => void Envs.SetVariable(env.name, v.name, rawValue(text, v.type) as never).catch(fail);
  return (
    <div className="env-main">
      <header className="env-head">
        <h1>
          <i className="dot" style={{ background: "var(--pass)" }} /> {env.name}
        </h1>
        <p className="muted">
          {sources.join(" · ")} · {env.variables?.length ?? 0} variables
        </p>
        {env.error && <p className="fail">{env.error}</p>}
      </header>
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
        </tbody>
      </table>
      {adding ? (
        <NewVariable env={env.name} onDone={() => setAdding(false)} />
      ) : (
        <button className="add-row" onClick={() => setAdding(true)}>
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

function NewVariable({ env, onDone }: { env: string; onDone(): void }) {
  const [secret, setSecret] = useState(false);
  return (
    <form
      className="new-variable"
      onSubmit={async (e) => {
        e.preventDefault();
        const f = e.currentTarget.elements;
        const name = (f.namedItem("name") as HTMLInputElement).value.trim();
        const value = (f.namedItem("value") as HTMLInputElement).value;
        if (!name) return onDone();
        try {
          // A secret goes to the secrets file at once, never to sonde.yaml.
          if (secret) await Envs.SetSecret(env, name, value);
          else await Envs.SetVariable(env, name, value as never);
          onDone();
        } catch (err) {
          fail(err);
        }
      }}
    >
      <input className="mono" name="name" aria-label="New variable name" placeholder="name" autoFocus />
      <input className="mono" name="value" aria-label="New variable value" placeholder="value" type={secret ? "password" : "text"} />
      <label className="secret-check">
        <input type="checkbox" role="switch" className="switch" aria-label="Secret" checked={secret} onChange={(e) => setSecret(e.target.checked)} /> Secret
      </label>
      <button className="btn" type="submit">
        Add
      </button>
      <button className="btn-ghost" type="button" onClick={onDone}>
        Cancel
      </button>
      {secret && (
        <p className="secret-note">
          <LockIcon />
          <span>Secret values are written to the environment&apos;s secrets file and never shown again or kept in history. Keep that file in .gitignore.</span>
        </p>
      )}
    </form>
  );
}
