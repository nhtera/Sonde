// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// A gRPC request (.sonde): [SondeGrpc] (proto, import-path, protoset), the
// service and method from those files (resolved as a run would), which
// set the URL's path, and the JSON message. Client- and bidi-streaming
// methods are listed but cannot be picked: Sonde runs unary and
// server-streaming methods.

import * as Menu from "@radix-ui/react-dropdown-menu";
import { useEffect, useState } from "react";
import { appError, EditSvc, type GrpcService } from "../../../lib/api";
import { jsonBodyLanguage } from "../../../lang";
import { useEnv } from "../../../state/env";
import { useTabs } from "../../../state/tabs";
import { CodeField } from "../code-field";
import { formEdit } from "../edit";
import { KvGrid } from "../kv-grid";
import type { EntryModel } from "../model";

/** The URL's base and its /service/method path. */
export function splitGrpcURL(url: string): { base: string; service: string; method: string } {
  const m = /^(.*?)\/([\w.]+)\/(\w+)$/.exec(url);
  return m ? { base: m[1], service: m[2], method: m[3] } : { base: url.replace(/\/$/, ""), service: "", method: "" };
}

const kindOf = (m: { ClientStreaming: boolean; ServerStreaming: boolean }) =>
  m.ClientStreaming && m.ServerStreaming ? "bidi stream" : m.ClientStreaming ? "client stream" : m.ServerStreaming ? "server stream" : "unary";

export function GrpcForm({ file, entry }: { file: string; entry: EntryModel }) {
  const n = entry.Index;
  const [services, setServices] = useState<GrpcService[] | null>(null);
  const [error, setError] = useState("");
  const env = useEnv((s) => s.current);
  const protoKey = JSON.stringify(entry.Rows?.grpc ?? []);
  useEffect(() => {
    const tab = useTabs.getState().tabs.find((t) => t.path === file);
    if (!tab) return;
    let live = true;
    EditSvc.Methods({ file, text: tab.text, version: tab.version }, n, env).then(
      (s) => {
        if (!live) return;
        setServices(s ?? []);
        setError("");
      },
      (err) => live && setError(appError(err).message),
    );
    return () => {
      live = false;
    };
    // Asked again when the proto rows change, not on every keystroke.
     
  }, [file, n, env, protoKey]);
  const { base, service, method } = splitGrpcURL(entry.URL);
  const svc = services?.find((s) => s.Name === service);
  const current = svc?.Methods?.find((m) => m.Name === method);
  const setPath = (s: string, m: string) => void formEdit(file, { kind: "setURL", entry: n, value: `${base}/${s}/${m}` });
  return (
    <div className="form-tab grpc-form">
      <p className="form-note flush">The URL's path is set by the service and method; the method is always POST.</p>
      <div className="grpc-pickers">
        <label>
          <span>Service</span>
          <select aria-label="gRPC service" value={service} onChange={(e) => {
            const s = services?.find((x) => x.Name === e.target.value);
            const first = s?.Methods?.find((m) => !m.ClientStreaming);
            if (s && first) setPath(s.Name, first.Name);
          }}>
            {!svc && <option value={service}>{service || "pick a service"}</option>}
            {services?.map((s) => (
              <option key={s.Name} value={s.Name}>
                {s.Name}
              </option>
            ))}
          </select>
        </label>
        <div className="grpc-method">
          <span>Method</span>
          <Menu.Root>
            <Menu.Trigger className="grpc-method-trigger" aria-label={`gRPC method: ${method || "none"}`}>
              <span className="mono">{method || "pick a method"}</span>
              {current && <span className="muted small">{kindOf(current)}</span>}
              <span aria-hidden>▾</span>
            </Menu.Trigger>
            <Menu.Portal>
              <Menu.Content className="menu grpc-methods" align="start" sideOffset={4}>
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
      </div>
      {error && <p className="form-note fail">{error}</p>}
      {services && services.length === 0 && !error && <p className="form-note">[SondeGrpc] names no proto file: the server's reflection is used at run time.</p>}
      <KvGrid file={file} entry={entry} sec="grpc" keyLabel="SondeGrpc" addLabel="+ proto, import-path or protoset" />
      <div className="pane-head">
        Message <span className="muted">{current?.Input ?? "JSON"}</span>
      </div>
      <CodeField
        label="gRPC message"
        language={jsonBodyLanguage}
        value={entry.Body}
        onCommit={(v) => void formEdit(file, { kind: "setBody", entry: n, value: v.trim() || "{}" })}
        minLines={6}
      />
    </div>
  );
}
