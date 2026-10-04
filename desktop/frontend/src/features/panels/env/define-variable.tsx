// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Define a variable a request uses and nothing sets (an undefined
// {{name}}): its value, and where it goes, a secret of an environment
// (its secrets file, never sonde.yaml), a variable of an environment
// (sonde.yaml), or this session only (--variable). Opened from a run's
// "Undefined variable", the editor's hover and an import's result, or
// from the palette with a name to enter.

import * as Dialog from "@radix-ui/react-dialog";
import { useState } from "react";
import { create } from "zustand";
import { registry } from "../../../app/registry";
import { appError, Envs } from "../../../lib/api";
import { isRequestPath } from "../../../lib/files";
import { useEnv } from "../../../state/env";
import { useTabs } from "../../../state/tabs";
import { useUI } from "../../../state/ui";

export type Where = "secret" | "variable" | "session";

/** The name being defined; "" to enter one, null when closed. */
const useDefine = create<{ name: string | null }>(() => ({ name: null }));

/** Opens the dialog to define name ("": the dialog asks for it). */
export function defineVariable(name = "") {
  useDefine.setState({ name });
}

/** Why name can not name a variable; "" when it can. */
export function nameError(name: string): string {
  if (!name) return "";
  if (!/^[A-Za-z0-9_-]+$/.test(name)) return "Letters, digits, _ and - only";
  if (["newDate", "newUuid", "getEnv"].includes(name)) return `${name} is a function`;
  return "";
}

/** Where a variable named so goes by default: a credential is a secret. */
export function defaultWhere(name: string, hasEnv: boolean): Where {
  if (!hasEnv) return "session";
  return /cookie|token|secret|passw|api[-_]?key|auth|bearer|jwt|credential/i.test(name) ? "secret" : "variable";
}

export function DefineVariableDialog() {
  const name = useDefine((s) => s.name);
  if (name === null) return null;
  return <DefineForm key={name} given={name} />;
}

function DefineForm({ given }: { given: string }) {
  const [typed, setName] = useState(given);
  const name = typed.trim();
  const envs = useEnv((s) => s.project?.envs ?? []);
  const current = useEnv((s) => s.current);
  const [env, setEnv] = useState(current || envs[0]?.name || "");
  // Picked by hand, or the default for the name typed so far.
  const [picked, setWhere] = useState<Where | null>(null);
  const where = picked ?? defaultWhere(name, envs.length > 0);
  const invalid = nameError(name);
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const close = () => useDefine.setState({ name: null });
  const secretsFile = envs.find((e) => e.name === env)?.secretsFile || `secrets/${env}.secrets`;
  const options: [Where, string, string][] = [
    ["secret", `Secret of ${env}`, `${secretsFile} · kept out of sonde.yaml, not shared`],
    ["variable", `Variable of ${env}`, "sonde.yaml · shared with the project"],
    ["session", "This session only", "Used as --variable until you quit; written nowhere"],
  ];
  const save = async () => {
    setBusy(true);
    try {
      if (where === "secret") await Envs.SetSecret(env, name, value);
      else if (where === "variable") await Envs.SetVariable(env, name, value as never);
      else await Envs.SetOverride(env, name, value);
      close();
      const file = useTabs.getState().active;
      useUI.getState().toast({
        kind: "success",
        text: `${name} is set${where === "session" ? " for this session" : ` in ${env}`}`,
        action: file && isRequestPath(file) ? { label: "Run again", run: () => void registry.getCommand("file.run")?.run() } : undefined,
      });
    } catch (err) {
      useUI.getState().toast({ kind: "error", text: appError(err).message });
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog.Root open onOpenChange={(open) => !open && close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog dialog-pad define-var" aria-describedby="define-var-note">
          <Dialog.Title className="dialog-title">
            {given ? (
              <>
                Define <code className="mono">{`{{${given}}}`}</code>
              </>
            ) : (
              "Define a variable"
            )}
          </Dialog.Title>
          <p id="define-var-note" className="dialog-note">
            {given ? "A request uses it, and no environment or earlier capture sets it." : "A value for {{name}} in the requests, before you run them."}
          </p>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (!busy && name && !invalid) void save();
            }}
          >
            {!given && (
              <label className="define-field">
                <span>Name</span>
                <input className="mono" aria-label="Name" autoFocus spellCheck={false} autoComplete="off" value={typed} onChange={(e) => setName(e.target.value)} />
                {invalid && <span className="define-error">{invalid}</span>}
              </label>
            )}
            <label className="define-field">
              <span>Value</span>
              <input
                className="mono"
                type={where === "secret" ? "password" : "text"}
                aria-label="Value"
                autoFocus={!!given}
                spellCheck={false}
                autoComplete="off"
                value={value}
                onChange={(e) => setValue(e.target.value)}
              />
            </label>
            {envs.length > 1 && where !== "session" && (
              <label className="define-field">
                <span>Environment</span>
                <select aria-label="Environment" value={env} onChange={(e) => setEnv(e.target.value)}>
                  {envs.map((e) => (
                    <option key={e.name}>{e.name}</option>
                  ))}
                </select>
              </label>
            )}
            <div className="define-where" role="radiogroup" aria-label="Save as">
              {options.map(([id, title, sub]) => {
                const off = id !== "session" && envs.length === 0;
                return (
                  <label key={id} className={`define-option${where === id ? " on" : ""}${off ? " off" : ""}`}>
                    <input type="radio" name="define-where" checked={where === id} disabled={off} onChange={() => setWhere(id)} />
                    <span>
                      <b>{off ? title.replace(/ of $/, "") : title}</b>
                      <span className="muted small">{off ? "The project has no sonde.yaml environment" : sub}</span>
                    </span>
                  </label>
                );
              })}
            </div>
            <div className="dialog-actions">
              <button type="button" className="btn-ghost" onClick={close}>
                Cancel
              </button>
              <button type="submit" className="btn-primary" disabled={busy || !name || !!invalid}>
                Save
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
