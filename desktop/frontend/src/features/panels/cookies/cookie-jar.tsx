// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The cookie jar kept between runs (Settings › Keep cookies), per file and
// domain: names, paths, expiry and flags; values are never shown. Delete
// one, or clear a file's jar.

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
  return (
    <Dialog.Root open={open} onOpenChange={(o) => useCookieJar.getState().setOpen(o)}>
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content className="dialog dialog-pad cookie-jar" aria-describedby={undefined}>
          <Dialog.Title className="dialog-title">Cookie jar</Dialog.Title>
          <p className="muted small">
            Cookies kept between runs. Like the CLI, each run starts with an empty jar unless you keep cookies{keep ? "" : " (off)"}. Values are not shown.
          </p>
          <div className="jar-body">
            <ul className="jar-domains" aria-label="Domains">
              {domains.map((d) => (
                <li key={d}>
                  <button aria-pressed={d === domain} onClick={() => setPicked(d)}>
                    <span className="mono">{d}</span>
                    <span className="muted small">{jars.flatMap((j) => j.cookies ?? []).filter((c) => c.domain === d).length}</span>
                  </button>
                </li>
              ))}
              {domains.length === 0 && <li className="muted small">The jar is empty.</li>}
            </ul>
            {domain && (
              <table className="jar-table" aria-label={`Cookies of ${domain}`}>
                <thead>
                  <tr>
                    <th>Name</th>
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
                      <td className="mono">{c.path}</td>
                      <td>{expiresText(c.expires)}</td>
                      <td className="small">{[c.httpOnly && "HttpOnly", c.secure && "Secure"].filter(Boolean).join(" · ") || "–"}</td>
                      <td className="mono small">{c.file}</td>
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
            )}
          </div>
          <div className="jar-foot">
            {jars.map((j) => (
              <div key={j.file} className="jar-file small">
                <span className="mono">{j.file}</span>
                <span className="muted mono">{j.path}</span>
                <button className="btn-ghost" onClick={() => void navigator.clipboard.writeText(j.path)}>
                  Copy path
                </button>
                <button
                  className="btn-ghost danger"
                  onClick={async () => {
                    if (await confirm({ title: `Clear the jar of ${j.file}?`, message: "Its kept cookies are deleted.", submit: "Clear" })) {
                      await Jar.Clear(j.file).then(load, fail);
                    }
                  }}
                >
                  Clear
                </button>
              </div>
            ))}
          </div>
          <Dialog.Close className="btn-ghost close">Esc</Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
