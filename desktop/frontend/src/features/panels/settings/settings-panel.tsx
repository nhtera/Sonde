// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Settings. The app's own (appearance, shortcuts, history) and the ones
// that change a run: those are desktop-only, each shows the command flag
// it maps to, and they count in the overrides chip (Copy as › sonde adds
// them, so CI can reproduce the run).

import { type ReactNode } from "react";
import { create } from "zustand";
import { useKeyLabel } from "../../../app/keymap/use-keys";
import { confirm } from "../../../components/ask";
import { appError, Dialogs, History, Settings as SettingsSvc, Update, type SettingsValue } from "../../../lib/api";
import { updatesOn, windowLook } from "../../../lib/mode";
import { useEnv } from "../../../state/env";
import { useSettings } from "../../../state/settings";
import { useUI } from "../../../state/ui";
import { SuggestInput } from "../../form/suggest-input";
import { ThemeSettings } from "./theme-settings";
import { useCookieJar } from "../cookies/cookie-jar";
import { lastResult, privacyLine, updateCheckNote } from "../../update/texts";
import { useUpdate } from "../../../state/update";

const sections = [
  { id: "general", title: "General" },
  { id: "editor", title: "Editor" },
  { id: "keyboard", title: "Keyboard" },
  { id: "network", title: "Network" },
  { id: "tls", title: "Certificates" },
  { id: "privacy", title: "History & privacy" },
];

const fail = (err: unknown) => useUI.getState().toast({ kind: "error", text: appError(err).message });

/** The section shown: General shows every card, the others theirs. */
const useSection = create<{ id: string }>(() => ({ id: "general" }));

/** Shows a section of the settings ("cookies" is under Network). */
export function goToSection(id: string) {
  useSection.setState({ id: id === "cookies" ? "network" : sections.some((s) => s.id === id) ? id : "general" });
}

export function SettingsSide() {
  const shown = useSection((s) => s.id);
  return (
    <div className="panel-body">
      <div className="panel-head">
        <h2>Settings</h2>
      </div>
      <nav className="settings-nav" aria-label="Settings sections">
        {sections.map((s) => (
          <button key={s.id} aria-current={shown === s.id ? "true" : undefined} onClick={() => goToSection(s.id)}>
            {s.title}
          </button>
        ))}
      </nav>
      <p className="side-note small">App settings live in the app&apos;s data. Project options live in sonde.yaml.</p>
    </div>
  );
}

function Row({ name, flag, children }: { name: string; flag?: string; children: ReactNode }) {
  return (
    <div className="set-row">
      <div>
        <div>{name}</div>
        {flag && <div className="flag small">Desktop only · {flag}</div>}
      </div>
      <div className="set-control">{children}</div>
    </div>
  );
}

function Card({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section className="set-card" id={`settings-${id}`} aria-label={title}>
      <h2>{title}</h2>
      {children}
    </section>
  );
}

const shortcuts: [string, string][] = [
  ["request.send", "Send request at cursor"],
  ["request.runTo", "Run requests 1 to cursor"],
  ["file.run", "Run whole file"],
  ["response.save", "Save response to file"],
  ["editor.toggleView", "Toggle Text / Form"],
  ["palette.open", "Command palette"],
];

function Shortcut({ id, title }: { id: string; title: string }) {
  const keys = useKeyLabel(id);
  return (
    <div className="set-row">
      <div>{title}</div>
      <kbd>{keys || "none"}</kbd>
    </div>
  );
}

export function SettingsMain() {
  const v = useSettings((s) => s.value);
  const overrides = useEnv((s) => s.overrides.count);
  const sendKeys = useKeyLabel("request.send");
  const section = useSection((s) => s.id);
  const all = section === "general";
  const show = (id: string) => all || section === id;
  if (!v) return null;
  const save = (next: SettingsValue) => void useSettings.getState().save(next).catch(fail);
  const pickTLS = async (kind: "cacert" | "cert" | "key") => {
    try {
      const handle = await Dialogs.OpenFile(kind === "cacert" ? "CA bundle" : kind === "cert" ? "Client certificate" : "Client key", "", "");
      if (!handle) return;
      const next = await SettingsSvc.SetTLSFile(kind, handle);
      useSettings.setState({ value: next });
    } catch (err) {
      fail(err);
    }
  };
  // Set keeps the TLS files: only SetTLSFile changes them ("" clears).
  const clearTLS = async (kind: "cacert" | "cert" | "key") => {
    try {
      useSettings.setState({ value: await SettingsSvc.SetTLSFile(kind, "") });
    } catch (err) {
      fail(err);
    }
  };
  const tlsRow = (kind: "cacert" | "cert" | "key", name: string, flag: string) => (
    <Row name={name} flag={flag}>
      <span className="mono path">{v.tls[kind] || "none"}</span>
      {windowLook && (
        <button className="btn" onClick={() => void pickTLS(kind)}>
          Choose…
        </button>
      )}
      {v.tls[kind] && (
        <button className="btn-ghost" onClick={() => void clearTLS(kind)}>
          Clear
        </button>
      )}
    </Row>
  );
  // The editor's rows: under Appearance in General, their own card under
  // Editor.
  const editorRows = (
    <>
      <Row name="Editor font size">
        <div className="segmented" role="group" aria-label="Editor font size">
          {[12, 13, 14, 15].map((n) => (
            <button key={n} aria-pressed={v.appearance.codeFontSize === n} onClick={() => save({ ...v, appearance: { ...v.appearance, codeFontSize: n } })}>
              {n}
            </button>
          ))}
        </div>
      </Row>
      <Row name="Code ligatures">
        <span className="muted small mono">{v.appearance.ligatures ? "on · == may join" : "off · == stays =="}</span>
        <input
          type="checkbox"
          role="switch"
          className="switch"
          aria-label="Code ligatures"
          checked={v.appearance.ligatures}
          onChange={(e) => save({ ...v, appearance: { ...v.appearance, ligatures: e.target.checked } })}
        />
      </Row>
    </>
  );
  return (
    <div className="settings-main">
      <div className="set-banner" role="note">
        {overrides > 0 && <span className="overrides-chip">{overrides} override{overrides === 1 ? "" : "s"}</span>}
        Settings that change a run are desktop-only. They show as the overrides chip and are added as flags to Copy as › sonde, so CI can reproduce the run.
      </div>
      <div className="set-grid">
        {all && (
          <Card id="general" title="Appearance">
            <ThemeSettings prefs={v.appearance} />
            <Row name="UI font size">
              <div className="segmented" role="group" aria-label="UI font size">
                {[12, 13, 14].map((n) => (
                  <button key={n} aria-pressed={v.appearance.uiFontSize === n} onClick={() => save({ ...v, appearance: { ...v.appearance, uiFontSize: n } })}>
                    {n}
                  </button>
                ))}
              </div>
            </Row>
            {editorRows}
          </Card>
        )}
        {section === "editor" && (
          <Card id="editor" title="Editor">
            {editorRows}
          </Card>
        )}
        {show("keyboard") && (
          <Card id="keyboard" title="Keyboard shortcuts">
            {shortcuts.map(([id, title]) => (
              <Shortcut key={id} id={id} title={title} />
            ))}
            <p className="muted small">
              {sendKeys || "⌘↵"} sends the request at the cursor, as in other API clients.{" "}
              <button className="btn-ghost accent inline" onClick={() => useUI.getState().setShortcutsOpen(true)}>
                All shortcuts, and change them
              </button>
            </p>
          </Card>
        )}
        {show("network") && (
          <Card id="network" title="Network">
            <Row name="Proxy" flag="--proxy">
              <SuggestInput label="Proxy" className="mono" value={v.network.proxy} placeholder="none" onCommit={(p) => save({ ...v, network: { ...v.network, proxy: p.trim() } })} />
            </Row>
            <Row name="Default connect timeout" flag="--connect-timeout when set">
              <SuggestInput label="Connect timeout" className="mono" value={v.network.connectTimeout} placeholder="none" onCommit={(t) => save({ ...v, network: { ...v.network, connectTimeout: t.trim() } })} />
            </Row>
            <Row name="Default retries" flag="--retry when set">
              <SuggestInput
                label="Retries"
                className="mono"
                value={String(v.network.retry || "")}
                placeholder="0"
                onCommit={(t) => save({ ...v, network: { ...v.network, retry: Math.max(0, Number(t) || 0) } })}
              />
            </Row>
            <Row name="Keep cookies between runs" flag="-b and -c with the kept jar">
              <input
                type="checkbox"
                role="switch"
                className="switch"
                aria-label="Keep cookies between runs"
                checked={v.cookies.keep}
                onChange={(e) => save({ ...v, cookies: { keep: e.target.checked } })}
              />
            </Row>
            <p className="muted small">Like the CLI, each run starts with an empty jar unless you keep cookies.</p>
            <button className="btn-ghost accent" onClick={() => useCookieJar.getState().setOpen(true)}>
              Manage the cookie jar
            </button>
          </Card>
        )}
        {show("tls") && (
          <Card id="tls" title="TLS & certificates">
            <div className="set-row">
              <div>
                <div>Verify certificates</div>
                <div className={`small ${v.tls.skipVerify ? "flag" : "muted"}`}>
                  {v.tls.skipVerify ? "Off for every run · --insecure" : "Per request, [Options] insecure: true turns this off"}
                </div>
              </div>
              <div className="set-control">
                <input
                  type="checkbox"
                  role="switch"
                  className="switch"
                  aria-label="Verify certificates"
                  checked={!v.tls.skipVerify}
                  onChange={(e) => save({ ...v, tls: { ...v.tls, skipVerify: !e.target.checked } })}
                />
              </div>
            </div>
            {tlsRow("cacert", "Extra CA bundle", "--cacert")}
            {tlsRow("cert", "Client certificate", "--cert when set")}
            {tlsRow("key", "Client key", "--key when set")}
          </Card>
        )}
        {show("privacy") && (
          <Card id="privacy" title="History & privacy">
            <Row name="Save run history">
              <input
                type="checkbox"
                role="switch"
                className="switch"
                aria-label="Save run history"
                checked={v.history.enabled}
                onChange={(e) => save({ ...v, history: { ...v.history, enabled: e.target.checked } })}
              />
            </Row>
            <Row name="Keep history for">
              <div className="segmented" role="group" aria-label="Keep history for">
                {[
                  ["7d", "7 days"],
                  ["30d", "30 days"],
                  ["forever", "Forever"],
                ].map(([id, label]) => (
                  <button key={id} aria-pressed={v.history.retention === id} onClick={() => save({ ...v, history: { ...v.history, retention: id } })}>
                    {label}
                  </button>
                ))}
              </div>
            </Row>
            <p className="muted small">Kept in the app&apos;s data, never in the project. Authorization, Cookie and Set-Cookie values, captured tokens and declared secrets are saved as ***.</p>
            {updatesOn && <UpdateRows />}
            <div className="set-row">
              <div>
                <i className="dot" /> {privacyLine(updatesOn)}
              </div>
              <button
                className="btn danger"
                onClick={async () => {
                  if (await confirm({ title: "Clear the history?", message: "Every run and send kept for this project is deleted.", submit: "Clear" })) {
                    await History.Clear().catch(fail);
                  }
                }}
              >
                Clear history
              </button>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}

/** The update check's settings (the window app): the switch and what it
 * sends, Check now with the last check, and a skipped version. */
function UpdateRows() {
  const v = useSettings((st) => st.value);
  const result = useUpdate((st) => lastResult(st.status));
  if (!v) return null;
  const u = v.updates;
  const last = u.lastCheck ? new Date(u.lastCheck) : null;
  return (
    <>
      <div className="set-row">
        <div>
          <div>Check for updates</div>
          <div className="muted small">{updateCheckNote}</div>
        </div>
        <div className="set-control">
          <input
            type="checkbox"
            role="switch"
            className="switch"
            aria-label="Check for updates"
            checked={u.check}
            onChange={(e) => void useSettings.getState().save({ ...v, updates: { ...u, check: e.target.checked } }).catch(fail)}
          />
        </div>
      </div>
      <Row name="Updates">
        <span className="muted small">
          {last && !isNaN(last.getTime()) ? `Last checked ${last.toLocaleString()}` : "Not checked yet"}
          {result && ` · ${result}`}
        </span>
        {u.skipped && (
          <button className="btn" onClick={() => void Update.ClearSkip().catch(fail)}>
            Show skipped updates again
          </button>
        )}
        <button className="btn" onClick={() => void Update.Check().catch(fail)}>
          Check now
        </button>
      </Row>
    </>
  );
}
