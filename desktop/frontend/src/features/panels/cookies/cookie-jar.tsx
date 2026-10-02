// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The cookie jar kept between runs (Settings › Keep cookies), by domain:
// names, paths, expiry and flags; values are never shown. Delete one,
// clear a domain or every jar, and keep cookies or not.

import * as Dialog from "@radix-ui/react-dialog";
import { useEffect, useState } from "react";
import { create } from "zustand";
import { confirm } from "../../../components/ask";
import { appError, Jar, type CookieJar } from "../../../lib/api";
import { useSettings } from "../../../state/settings";
import { useUI } from "../../../state/ui";

export const useCookieJar = create<{ open: boolean; setOpen(open: boolean): void }>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
}));

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

/** "session", "in 2 days", "expired" for an expiry (Unix seconds, 0). */
export function expiresText(expires: number, now = Date.now()): string {
  if (!expires) return "session";
  const days = Math.round((expires * 1000 - now) / 86_400_000);
  if (days < 0) return "expired";
  if (days === 0) return "today";
  return `in ${days} day${days === 1 ? "" : "s"}`;
}

export function CookieJarDialog() {
  const open = useCookieJar((s) => s.open);
  const keep = useSettings((s) => s.value?.cookies.keep ?? false);
  const [jars, setJars] = useState<CookieJar[]>([]);
  const [picked, setPicked] = useState("");
  const load = () => void Jar.List().then((l) => setJars(l ?? []), fail);
  useEffect(() => {
    if (open) load();
  }, [open]);
  const domains = [...new Set(jars.flatMap((j) => (j.cookies ?? []).map((c) => c.domain)))].sort();
  const domain = domains.includes(picked) ? picked : domains[0];
  const rows = jars.flatMap((j) => (j.cookies ?? []).filter((c) => c.domain === domain).map((c) => ({ ...c, file: j.file })));
  const save = (k: boolean) => {
    const v = useSettings.getState().value;
    if (v) void useSettings.getState().save({ ...v, cookies: { ...v.cookies, keep: k } }).catch(fail);
  };
  const count = (d: string) => jars.flatMap((j) => j.cookies ?? []).filter((c) => c.domain === d).length;
  return (
    <Dialog.Root open={open} onOpenChange={(o) => useCookieJar.getState().setOpen(o)}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog cookie-jar" aria-describedby={undefined}>
          <div className="dialog-head">
            <Dialog.Title className="dialog-title">Cookie jar</Dialog.Title>
            <p>Cookies kept between runs. Like the CLI, each run starts with an empty jar unless you keep cookies. Values are not shown.</p>
          </div>
          <div className="jar-body">
            <ul className="jar-domains" aria-label="Domains">
              {domains.map((d) => (
                <li key={d}>
                  <button aria-pressed={d === domain} onClick={() => setPicked(d)}>
                    <span className="mono">{d}</span>
                    <span className="n">{count(d)}</span>
                  </button>
                </li>
              ))}
              {domains.length === 0 && <li className="muted small">The jar is empty.</li>}
            </ul>
            {domain && (
              <div className="jar-main">
                <div className="jar-head">
                  <b className="mono">{domain}</b>
                  <button
                    className="jar-clear"
                    onClick={async () => {
                      if (await confirm({ title: `Clear the cookies of ${domain}?`, message: "They are deleted from the kept jars.", submit: "Clear" })) {
                        await Promise.all(rows.map((c) => Jar.Delete(c.file, c.domain, c.path, c.name))).then(load, fail);
                      }
                    }}
                  >
                    Clear {domain}
                  </button>
                </div>
                <table className="jar-table" aria-label={`Cookies of ${domain}`}>
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Value</th>
                      <th>Path</th>
                      <th>Expires</th>
                      <th>Flags</th>
                      <th>File</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((c) => (
                      <tr key={`${c.file}|${c.path}|${c.name}`}>
                        <td className="mono name">{c.name}</td>
                        <td className="mono secret">***</td>
                        <td className="mono">{c.path}</td>
                        <td>{expiresText(c.expires)}</td>
                        <td className="flags">{[c.httpOnly && "HttpOnly", c.secure && "Secure"].filter(Boolean).join(" · ") || "–"}</td>
                        <td className="mono file">{c.file}</td>
                        <td>
                          <button
                            className="kv-remove"
                            aria-label={`Delete ${c.name}`}
                            onClick={() => void Jar.Delete(c.file, c.domain, c.path, c.name).then(load, fail)}
                          >
                            ×
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <p className="jar-note">Values are never shown here, Secure and session cookies included. Kept per file, in the app&apos;s data.</p>
              </div>
            )}
          </div>
          <div className="jar-foot">
            <button
              className="jar-clear"
              disabled={jars.length === 0}
              onClick={async () => {
                if (await confirm({ title: "Clear all cookies?", message: "Every file's kept jar is deleted.", submit: "Clear" })) {
                  await Promise.all(jars.map((j) => Jar.Clear(j.file))).then(load, fail);
                }
              }}
            >
              Clear all cookies
            </button>
            <span style={{ flex: 1 }} />
            <label className="jar-keep">
              <input type="checkbox" role="switch" className="switch" aria-label="Keep cookies between runs" checked={keep} onChange={(e) => save(e.target.checked)} />
              <span>
                <b>Keep cookies between runs</b> · off by default. When on, the overrides chip shows and Copy as › sonde adds -b.
              </span>
            </label>
          </div>
          <Dialog.Close className="dialog-esc">Esc</Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
