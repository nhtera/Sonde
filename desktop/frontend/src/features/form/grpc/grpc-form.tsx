// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A gRPC request (.sonde): the service and method from the [SondeGrpc]
// files (proto, import-path, protoset; resolved as a run would), which set
// the URL's path, and the JSON message beside its type in the .proto.
// Client- and bidi-streaming methods are listed but cannot be picked:
// Sonde runs unary and server-streaming methods.

import * as Menu from "@radix-ui/react-dropdown-menu";
import { useEffect, useState } from "react";
import { appError, EditSvc, type GrpcMethod, type GrpcService } from "../../../lib/api";
import { jsonBodyLanguage } from "../../../lang";
import { useEnv } from "../../../state/env";
import { useTabs } from "../../../state/tabs";
import { CodeField } from "../code-field";
import { formEdit } from "../edit";
import { KvGrid } from "../kv-grid";
import { rowsOf, type EntryModel } from "../model";
import { Chevron } from "../url-bar";

/** The URL's base and its /service/method path. */
export function splitGrpcURL(url: string): { base: string; service: string; method: string } {
  const m = /^(.*?)\/([\w.]+)\/(\w+)$/.exec(url);
  return m ? { base: m[1], service: m[2], method: m[3] } : { base: url.replace(/\/$/, ""), service: "", method: "" };
}

const kindOf = (m: { ClientStreaming: boolean; ServerStreaming: boolean }) =>
  m.ClientStreaming && m.ServerStreaming ? "bidi stream" : m.ClientStreaming ? "client stream" : m.ServerStreaming ? "server stream" : "unary";

/** The last lookup, shared by the pickers and the message (both ask at
 * once). */
let lookup: { key: string; services: Promise<GrpcService[] | null> } | null = null;
function methodsOf(key: string, ask: () => Promise<GrpcService[] | null>): Promise<GrpcService[] | null> {
  if (lookup?.key !== key) lookup = { key, services: ask() };
  const mine = lookup;
  // A failed lookup is asked again next time.
  mine.services.catch(() => lookup === mine && (lookup = null));
  return mine.services;
}

/** The services the request's [SondeGrpc] files declare (asked again
 * when those rows change, not on every keystroke). */
function useGrpcServices(file: string, entry: EntryModel): { services: GrpcService[] | null; error: string } {
  const n = entry.Index;
  const [state, setState] = useState<{ services: GrpcService[] | null; error: string }>({ services: null, error: "" });
  const env = useEnv((s) => s.current);
  const protoKey = JSON.stringify(entry.Rows?.grpc ?? []);
  useEffect(() => {
    const tab = useTabs.getState().tabs.find((t) => t.path === file);
    if (!tab) return;
    let live = true;
    methodsOf(`${file}|${n}|${env}|${protoKey}`, () => EditSvc.Methods({ file, text: tab.text, version: tab.version }, n, env)).then(
      (s) => live && setState({ services: s ?? [], error: "" }),
      (err) => live && setState((p) => ({ ...p, error: appError(err).message })),
    );
    return () => {
      live = false;
    };
  }, [file, n, env, protoKey]);
  return state;
}

/** The method the URL names, from services. */
function methodOf(services: GrpcService[] | null, url: string): GrpcMethod | undefined {
  const { service, method } = splitGrpcURL(url);
  return services?.find((s) => s.Name === service)?.Methods?.find((m) => m.Name === method);
}

/** Service and method under the URL, then the [SondeGrpc] files. */
export function GrpcPickers({ file, entry }: { file: string; entry: EntryModel }) {
  const n = entry.Index;
  const { services, error } = useGrpcServices(file, entry);
  const [editing, setEditing] = useState(false);
  const { base, service, method } = splitGrpcURL(entry.URL);
  const svc = services?.find((s) => s.Name === service);
  const current = methodOf(services, entry.URL);
  const setPath = (s: string, m: string) => void formEdit(file, { kind: "setURL", entry: n, value: `${base}/${s}/${m}` });
  const files = rowsOf(entry, "grpc").filter((r) => !r.Disabled);
  return (
    <>
      <div className="grpc-pickers">
        <label className="grpc-pick">
          <span className="label">Service</span>
          <select
            aria-label="gRPC service"
            value={service}
            onChange={(e) => {
              const s = services?.find((x) => x.Name === e.target.value);
              const first = s?.Methods?.find((m) => !m.ClientStreaming);
              if (s && first) setPath(s.Name, first.Name);
            }}
          >
            {!svc && <option value={service}>{service || "pick a service"}</option>}
            {services?.map((s) => (
              <option key={s.Name} value={s.Name}>
                {s.Name}
              </option>
            ))}
          </select>
          <Chevron />
        </label>
        <Menu.Root>
          <Menu.Trigger className="grpc-pick grpc-method-trigger" aria-label={`gRPC method: ${method || "none"}`}>
            <span className="label">Method</span>
            <span className="value">{method || "pick a method"}</span>
            {current && <span className="kind">{kindOf(current)}</span>}
            <Chevron />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content className="menu grpc-methods" align="end" sideOffset={4}>
              <Menu.Label className="menu-label mono">{service || "no service"}</Menu.Label>
              <Menu.RadioGroup value={method} onValueChange={(m) => setPath(service, m)}>
                {svc?.Methods?.map((m) => (
                  <Menu.RadioItem key={m.Name} className="menu-item" value={m.Name} disabled={m.ClientStreaming}>
                    <span className="mono">{m.Name}</span>
                    <span className="hint">
                      {kindOf(m)}
                      {m.ClientStreaming ? " · not supported" : ""}
                    </span>
                  </Menu.RadioItem>
                ))}
              </Menu.RadioGroup>
              <div className="menu-note">Sonde runs unary and server-streaming methods.</div>
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      </div>
      <p className="grpc-files">
        {files.length > 0 ? files.map((r) => `${r.Key}: ${r.Value}`).join(" · ") + " · " : ""}
        {files.some((r) => r.Key === "proto") ? "import-path: and protoset: also work" : "proto:, import-path: or protoset:"} · with none of them, Sonde uses server reflection{" "}
        <button className="link" aria-expanded={editing} onClick={() => setEditing(!editing)}>
          {editing ? "Done" : "Edit"}
        </button>
      </p>
      {error && <p className="form-note fail">{error}</p>}
      {editing && (
        <div className="grpc-rows">
          <KvGrid file={file} entry={entry} sec="grpc" keyLabel="SondeGrpc" addLabel="+ proto, import-path or protoset" />
        </div>
      )}
    </>
  );
}

/** Whether a message reads as JSON ({{variables}} standing in for values). */
export function validMessage(body: string): boolean {
  try {
    JSON.parse(body.replace(/\{\{[^}]*\}\}/g, "0") || "{}");
    return true;
  } catch {
    return false;
  }
}

const short = (name: string) => name.slice(name.lastIndexOf(".") + 1);

/** The message, beside its type as the .proto declares it. */
export function GrpcForm({ file, entry }: { file: string; entry: EntryModel }) {
  const n = entry.Index;
  const { services } = useGrpcServices(file, entry);
  const current = methodOf(services, entry.URL);
  const valid = validMessage(entry.Body.trim());
  return (
    <div className="form-tab grpc-form">
      <div className="grpc-message">
        <div className="gq-pane">
          <div className="pane-head">
            <span>{current ? short(current.Input) : "Message"}</span>
            <span className={`state ${valid ? "pass" : "fail"}`}>{valid ? "✓ valid" : "✕ not JSON"}</span>
          </div>
          <CodeField
            label="gRPC message"
            language={jsonBodyLanguage}
            value={entry.Body}
            onCommit={(v) => void formEdit(file, { kind: "setBody", entry: n, value: v.trim() || "{}" })}
            minLines={6}
          />
        </div>
        <div className="gq-pane">
          <div className="pane-head">From the .proto</div>
          {current ? (
            <div className="grpc-schema mono">
              <span>
                <span className="tk-sec">message</span> {short(current.Input)} {"{"}
              </span>
              {current.InputFields?.map((f) => (
                <span key={f.Name}>
                  {"  "}
                  <span className="tk-key">{f.Type}</span> {f.Name} = {f.Number};
                </span>
              ))}
              <span>{"}"}</span>
              <span className="returns">
                returns <b>{short(current.Output)}</b>
              </span>
            </div>
          ) : (
            <p className="grpc-schema muted">Pick a method to see its message.</p>
          )}
        </div>
      </div>
    </div>
  );
}
