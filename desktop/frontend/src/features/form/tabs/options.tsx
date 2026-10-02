// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Options as controls, each showing the [Options] lines it writes. Only
// options set differently from the default are written; clearing one
// removes its line. Other options are raw rows under More options.

import { useState, type CSSProperties, type ReactNode } from "react";
import { formEdit, type Op } from "../edit";
import { KvGrid } from "../kv-grid";
import { countOf, rowsOf, type EntryModel } from "../model";
import { SuggestInput } from "../suggest-input";
import { Chevron } from "../url-bar";

const versions = ["http1.0", "http1.1", "http2", "http3"];
/** The options the controls own; the rest are More options. */
export const controlled = new Set(["location", "max-redirs", "connect-timeout", "max-time", "retry", "retry-interval", "delay", "insecure", "proxy", "compressed", ...versions]);
const extras: [string, string][] = [
  ["variable", "name=value"],
  ["skip", "false"],
  ["repeat", "2"],
  ["output", "response.out"],
  ["very-verbose", "true"],
];

/** The ops that make option key read value ("" removes it). The live row
 * is the one changed; a disabled `# key:` row is enabled and set when
 * there is none (and left alone when clearing). */
export function optionOps(entry: EntryModel, key: string, value: string): Op[] {
  const n = entry.Index;
  const rows = rowsOf(entry, "options");
  const live = rows.findIndex((r) => r.Key === key && !r.Disabled);
  if (!value) return live >= 0 ? [{ kind: "removeRow", entry: n, section: "options", index: live }] : [];
  if (live >= 0) return [{ kind: "setRow", entry: n, section: "options", index: live, key, value }];
  const off = rows.findIndex((r) => r.Key === key);
  if (off >= 0) {
    return [
      { kind: "toggleRow", entry: n, section: "options", index: off },
      { kind: "setRow", entry: n, section: "options", index: off, key, value },
    ];
  }
  return [{ kind: "addRow", entry: n, section: "options", key, value }];
}

/** The ops that make the HTTP version v ("" for Auto): the others' live
 * rows go (highest index first), then v's is set, at its index after
 * those removals. */
export function versionOps(entry: EntryModel, v: string): Op[] {
  const rows = rowsOf(entry, "options");
  const gone = rows
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => versions.includes(r.Key) && r.Key !== v && !r.Disabled)
    .map(({ i }) => i)
    .sort((a, b) => b - a);
  const ops: Op[] = gone.map((index) => ({ kind: "removeRow", entry: entry.Index, section: "options", index }));
  if (!v || rows.some((r) => r.Key === v && !r.Disabled && r.Value === "true")) return ops;
  for (const op of optionOps(entry, v, "true")) {
    ops.push(op.index === undefined ? op : { ...op, index: op.index - gone.filter((g) => g < op.index!).length });
  }
  return ops;
}

export function OptionsTab({ file, entry }: { file: string; entry: EntryModel }) {
  const rows = rowsOf(entry, "options");
  const get = (key: string) => rows.find((r) => r.Key === key && !r.Disabled)?.Value ?? "";
  const set = (key: string, value: string) => void formEdit(file, ...optionOps(entry, key, value));
  const version = versions.find((v) => get(v) === "true") ?? "";
  const setVersion = (v: string) => void formEdit(file, ...versionOps(entry, v));
  const writes = (...keys: string[]) =>
    keys
      .filter((k) => get(k))
      .map((k) => `${k}: ${get(k)}`)
      .join(" · ") || "default";
  // A value chip, sized to its text: "max 5", "every 500ms".
  const text = (key: string, label: string, affix: { pre?: string; post?: string; placeholder?: string } = {}) => {
    const v = get(key);
    const placeholder = affix.placeholder ?? "default";
    return (
      <label className="opt-chip" style={{ "--w": `${Math.max((v || placeholder).length, 1)}ch` } as CSSProperties}>
        {v && affix.pre && <span>{affix.pre}</span>}
        <SuggestInput label={label} className="mono" highlight={false} value={v} placeholder={placeholder} onCommit={(n) => set(key, n.trim())} />
        {v && affix.post && <span>{affix.post}</span>}
      </label>
    );
  };
  const toggle = (key: string, label: string) => (
    <input type="checkbox" role="switch" className="switch" aria-label={label} checked={get(key) === "true"} onChange={(e) => set(key, e.target.checked ? "true" : "")} />
  );
  const extra = rows.filter((r) => !controlled.has(r.Key)).length;
  const [more, setMore] = useState(extra > 0);
  // HTTP/1.0 is offered only when the file already asks for it.
  const shown = versions.filter((v) => v !== "http1.0" || version === v);
  return (
    <div className="form-tab options-tab">
      <div className="opt-head">
        <span>Option</span>
        <span>Value</span>
        <span>Writes in [Options]</span>
      </div>
      <Opt name="Follow redirects" hint="Up to a maximum number of hops" writes={writes("location", "max-redirs")}>
        {toggle("location", "Follow redirects")}
        {text("max-redirs", "Maximum redirects", { pre: "max", placeholder: "max" })}
      </Opt>
      <Opt name="Connect timeout" hint="Time to open the connection" writes={writes("connect-timeout")}>
        {text("connect-timeout", "Connect timeout")}
      </Opt>
      <Opt name="Max time" hint="Total time for the whole request" writes={writes("max-time")}>
        {text("max-time", "Max time")}
      </Opt>
      <Opt name="Retry" hint="Retry until asserts pass" writes={writes("retry", "retry-interval")}>
        {text("retry", "Retry times", { post: "times", placeholder: "times" })}
        {text("retry-interval", "Retry interval", { pre: "every", placeholder: "every" })}
      </Opt>
      <Opt name="Delay" hint="Wait before sending" writes={writes("delay")}>
        {text("delay", "Delay", { placeholder: "0 ms" })}
      </Opt>
      <Opt
        name="Skip TLS verification"
        hint="Accept any certificate"
        writes={writes("insecure")}
        warn={get("insecure") === "true"}
        after={
          get("insecure") === "true" && (
            <p className="opt-warn">
              <span aria-hidden>⚠</span>
              <span>Certificates are not checked for this request. Prefer adding a CA file in Settings → Certificates.</span>
            </p>
          )
        }
      >
        {toggle("insecure", "Skip TLS verification")}
      </Opt>
      <Opt name="HTTP version" hint="Negotiated when Auto" writes={writes(...versions)}>
        <div className="segmented opt-seg" role="group" aria-label="HTTP version">
          {["", ...shown].map((v) => (
            <button key={v} aria-pressed={version === v} onClick={() => setVersion(v)}>
              {v ? v.replace("http", "") : "Auto"}
            </button>
          ))}
        </div>
      </Opt>
      <Opt name="Proxy" hint="Overrides Settings → Network" writes={writes("proxy")}>
        {text("proxy", "Proxy", { placeholder: "none" })}
      </Opt>
      <Opt name="Compressed" hint="Ask for gzip/br and decode" writes={writes("compressed")}>
        {toggle("compressed", "Compressed")}
      </Opt>
      <div className="opt-more">
        <div>
          <button className="opt-more-toggle" aria-expanded={more} onClick={() => setMore(!more)}>
            More options
            <Chevron />
          </button>
          <span className="muted">Raw key: value rows for everything else</span>
        </div>
        <div className="opt-chips">
          {extras.map(([k, v]) => (
            <button
              key={k}
              className="opt-add"
              onClick={() => {
                setMore(true);
                void formEdit(file, { kind: "addRow", entry: entry.Index, section: "options", key: k, value: v });
              }}
            >
              + {k}
            </button>
          ))}
        </div>
      </div>
      {more && <KvGrid file={file} entry={entry} sec="options" keyLabel="Option" only={(r) => !controlled.has(r.Key)} addLabel="+ Add option" />}
      <p className="form-note">
        Only changed options are written. This request has {countOf(entry, "options")} line{countOf(entry, "options") === 1 ? "" : "s"} in [Options], matching the tab count.
      </p>
    </div>
  );
}

function Opt({ name, hint, writes, warn, after, children }: { name: string; hint: string; writes: string; warn?: boolean; after?: ReactNode; children: ReactNode }) {
  return (
    <div className="opt-row">
      <div>
        <div className="opt-name">{name}</div>
        <div className="muted">{hint}</div>
      </div>
      <div className="opt-controls">{children}</div>
      <div className={`opt-writes mono${writes === "default" ? " muted" : ""}${warn ? " warn" : ""}`}>{writes}</div>
      {after}
    </div>
  );
}
