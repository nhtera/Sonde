// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The cookie jar kept between runs (Settings › Keep cookies), by domain:
// names, paths, expiry and flags; values are never shown. Delete one,
// clear a domain or every jar, add a cookie or replace a value (written,
// never read back), and keep cookies or not.

import * as Dialog from "@radix-ui/react-dialog";
import { useEffect, useMemo, useState } from "react";
import { create } from "zustand";
import { confirm } from "../../../components/ask";
import { appError, Jar, type CookieJar } from "../../../lib/api";
import { useSettings } from "../../../state/settings";
import { useUI } from "../../../state/ui";
import { useWorkspace } from "../../../state/workspace";
import { OverridesChip } from "../../shell/title-bar";

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

/** A cookie row of the picked domain, with the file whose jar holds it. */
type Row = NonNullable<CookieJar["cookies"]>[number] & { file: string };

const keyOf = (c: Row) => `${c.file}|${c.path}|${c.name}`;

export function CookieJarDialog() {
  const open = useCookieJar((s) => s.open);
  const keep = useSettings((s) => s.value?.cookies.keep ?? false);
  const [jars, setJars] = useState<CookieJar[]>([]);
  const [picked, setPicked] = useState("");
  // The row whose value is being replaced, or a cookie being added.
  const [editing, setEditing] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const load = () => void Jar.List().then((l) => setJars(l ?? []), fail);
  useEffect(() => {
    if (open) load();
  }, [open]);
  const domains = [...new Set(jars.flatMap((j) => (j.cookies ?? []).map((c) => c.domain)))].sort();
  const domain = domains.includes(picked) ? picked : domains[0];
  const rows: Row[] = jars.flatMap((j) => (j.cookies ?? []).filter((c) => c.domain === domain).map((c) => ({ ...c, file: j.file })));
  const save = (k: boolean) => {
    const v = useSettings.getState().value;
    if (v) void useSettings.getState().save({ ...v, cookies: { ...v.cookies, keep: k } }).catch(fail);
  };
  const count = (d: string) => jars.flatMap((j) => j.cookies ?? []).filter((c) => c.domain === d).length;
  // A new value: written to the jar, never read back.
  const setValue = (c: Row, value: string) =>
    void Jar.Set(c.file, { domain: c.domain, path: c.path, name: c.name, value, expires: c.expires, secure: c.secure, httpOnly: c.httpOnly }).then(() => {
      setEditing(null);
      load();
    }, fail);
  const edited = rows.find((c) => keyOf(c) === editing);
  // A cookie can go to any request file's jar.
  const index = useWorkspace((s) => s.index);
  const files = useMemo(() => [...new Set([...jars.map((j) => j.file), ...index.map((r) => r.file)])].sort(), [jars, index]);
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(o) => {
        useCookieJar.getState().setOpen(o);
        setEditing(null);
        setAdding(false);
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="scrim" />
        <Dialog.Content
          className="dialog cookie-jar"
          aria-describedby={undefined}
          onEscapeKeyDown={(e) => {
            // Esc ends the edit first.
            if (editing || adding) {
              e.preventDefault();
              setEditing(null);
              setAdding(false);
            }
          }}
        >
          <div className="dialog-head">
            <Dialog.Title className="dialog-title">Cookie jar</Dialog.Title>
            <p>Cookies kept between runs. Like the CLI, each run starts with an empty jar unless you keep cookies. Values are not shown.</p>
          </div>
          <div className="jar-body">
            <div className="jar-side">
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
              <button
                className="jar-clear all"
                disabled={jars.length === 0}
                onClick={async () => {
                  if (await confirm({ title: "Clear all cookies?", message: "Every file's kept jar is deleted.", submit: "Clear" })) {
                    await Promise.all(jars.map((j) => Jar.Clear(j.file))).then(load, fail);
                  }
                }}
              >
                Clear all cookies
              </button>
            </div>
            <div className="jar-main">
              <div className="jar-head">
                <b className="mono">{domain ?? "No cookies kept"}</b>
                <button className="btn" disabled={adding} onClick={() => setAdding(true)}>
                  + Add cookie
                </button>
                {domain && (
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
                )}
              </div>
              {(domain || adding) && (
                <>
                  <table className="jar-table" aria-label={domain ? `Cookies of ${domain}` : "New cookie"}>
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
                        <tr key={keyOf(c)} className={keyOf(c) === editing ? "editing" : undefined}>
                          <td className="mono name">{c.name}</td>
                          <td className="mono secret">
                            {keyOf(c) === editing ? (
                              <input
                                className="jar-input mono"
                                aria-label={`New value of ${c.name}`}
                                placeholder="new value"
                                autoFocus
                                spellCheck={false}
                                onKeyDown={(e) => {
                                  // An empty value would wipe the cookie's: Esc cancels instead.
                                  if (e.key === "Enter" && !e.nativeEvent.isComposing && e.currentTarget.value) setValue(c, e.currentTarget.value);
                                }}
                              />
                            ) : (
                              "***"
                            )}
                          </td>
                          <td className="mono">{c.path}</td>
                          <td>{expiresText(c.expires)}</td>
                          <td className="flags">{[c.httpOnly && "HttpOnly", c.secure && "Secure"].filter(Boolean).join(" · ") || "–"}</td>
                          <td className="mono file">{c.file}</td>
                          <td className="jar-actions">
                            <button className="kv-remove" aria-label={`Edit ${c.name}`} title="Replace the value" onClick={() => setEditing(keyOf(c))}>
                              ✎
                            </button>
                            <button className="kv-remove" aria-label={`Delete ${c.name}`} onClick={() => void Jar.Delete(c.file, c.domain, c.path, c.name).then(load, fail)}>
                              ×
                            </button>
                          </td>
                        </tr>
                      ))}
                      {adding && (
                        <NewCookie
                          domain={domain ?? ""}
                          files={files}
                          file={rows[0]?.file}
                          onDone={(added) => {
                            setAdding(false);
                            if (added) load();
                          }}
                        />
                      )}
                    </tbody>
                  </table>
                  <p className="jar-note">
                    {edited ? (
                      <>
                        Editing <span className="mono">{edited.name}</span> · ↵ to save, Esc to cancel. Values are never shown here, so type the new one.
                      </>
                    ) : adding ? (
                      "A new cookie · ↵ to add, Esc to cancel. A session cookie, without flags."
                    ) : (
                      "Values are never shown here, Secure and session cookies included. Kept per file, in the app's data."
                    )}
                  </p>
                </>
              )}
              <div className="jar-keep-row">
                <label className="jar-keep">
                  <input type="checkbox" role="switch" className="switch" aria-label="Keep cookies between runs" checked={keep} onChange={(e) => save(e.target.checked)} />
                  <span>
                    <b>Keep cookies between runs</b> · off by default. When on, the overrides chip shows and Copy as › sonde adds -b.
                  </span>
                </label>
                {keep && <OverridesChip />}
              </div>
            </div>
          </div>
          <Dialog.Close className="dialog-esc">Esc</Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** A cookie being added to a file's jar: name, value and path, in the
 * domain picked. ↵ adds it. */
function NewCookie({ domain, files, file, onDone }: { domain: string; files: string[]; file?: string; onDone(added: boolean): void }) {
  const [to, setTo] = useState(file ?? files[0] ?? "");
  const add = (row: HTMLTableRowElement) => {
    const v = (n: string) => (row.querySelector(`[name="${n}"]`) as HTMLInputElement).value.trim();
    if (!v("name") || !v("domain") || !to) return;
    void Jar.Set(to, { domain: v("domain"), path: v("path") || "/", name: v("name"), value: v("value"), expires: 0, secure: false, httpOnly: false }).then(() => onDone(true), fail);
  };
  return (
    <tr
      className="editing"
      onKeyDown={(e) => {
        // Enter adds, but not while composing text or picking the file.
        if (e.key === "Enter" && !e.nativeEvent.isComposing && !(e.target instanceof HTMLSelectElement)) add(e.currentTarget);
      }}
    >
      <td>
        <input className="jar-input mono" name="name" aria-label="New cookie name" placeholder="name" autoFocus spellCheck={false} />
        {/* The domain picked, or another. */}
        <input className="jar-input mono domain" name="domain" aria-label="New cookie domain" placeholder="domain" defaultValue={domain} spellCheck={false} />
      </td>
      <td>
        <input className="jar-input mono" name="value" aria-label="New cookie value" placeholder="value" spellCheck={false} />
      </td>
      <td>
        <input className="jar-input mono" name="path" aria-label="New cookie path" defaultValue="/" spellCheck={false} />
      </td>
      <td>session</td>
      <td className="flags">–</td>
      <td className="file">
        <select className="jar-input mono" aria-label="Jar of file" value={to} onChange={(e) => setTo(e.target.value)}>
          {files.map((f) => (
            <option key={f}>{f}</option>
          ))}
        </select>
      </td>
      <td />
    </tr>
  );
}
