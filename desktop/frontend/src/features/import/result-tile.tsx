// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// What an import wrote, in honest counts, and what it could not carry over
// (grouped by kind). A Postman import offers its suggestions next.

import * as Dialog from "@radix-ui/react-dialog";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { label } from "../../app/keymap/keymap-manager";
import type { ImportWrote as Wrote } from "../../lib/api";
import { useImport } from "./state";

const notes: Record<string, string> = {
  script: "Scripts kept as # comments (Sonde does not run scripts)",
  "unsupported-auth": "Auth settings not converted",
  "unsupported-body": "Bodies not converted",
  "unsupported-option": "Options not converted",
  "dynamic-variable": "Dynamic variables to set by hand",
  secret: "Values to move to a secret",
  unsupported: "Other items not converted",
  environments: "Environments not in the app's sonde.yaml",
};

/** The warnings of an import by kind: [kind, count, first message]. */
export function grouped(warnings: { kind: string; message: string }[]): [string, number, string][] {
  const m = new Map<string, [number, string]>();
  for (const w of warnings) {
    const g = m.get(w.kind);
    m.set(w.kind, g ? [g[0] + 1, g[1]] : [1, w.message]);
  }
  return [...m].map(([k, [n, first]]) => [k, n, first]);
}

/** What a kind is called in a title. */
export const kindTitle: Record<string, string> = { curl: "curl", postman: "Postman", opencollection: "Bruno", http: ".http", openapi: "OpenAPI" };

/** What a line became, colored as the editor colors it: an HTTP line, a
 * request line with its {{variables}}, or a comment. */
function WroteTo({ w }: { w: Wrote }) {
  if (w.kind === "comment") return <span className="mono to com">{w.to}</span>;
  const status = /^HTTP (\d+)$/.exec(w.to);
  if (status) {
    return (
      <span className="mono to">
        <span className="t-http">HTTP</span> <span className="t-num">{status[1]}</span>
      </span>
    );
  }
  const req = /^([A-Z]+) (.*)$/.exec(w.to);
  if (!req) return <span className="mono to">{w.to}</span>;
  return (
    <span className="mono to">
      <span className={`m-${req[1]}`}>{req[1]}</span>{" "}
      {req[2].split(/(\{\{[^}]*\}\})/).map((part, i) => (part.startsWith("{{") ? <span key={i} className="t-var">{part}</span> : part))}
    </span>
  );
}

/** A card: what needs a look, why, and what to do about it. */
function WarnCard({ title, sub, action }: { title: string; sub: string; action?: { label: string; run(): void } }) {
  return (
    <div className="warn-card" role="note">
      <span className="warn-icon" aria-hidden>
        ⚠
      </span>
      <span className="warn-text">
        <b>{title}</b>
        <span className="muted small">{sub}</span>
      </span>
      {action && (
        <button className="btn" onClick={action.run}>
          {action.label}
        </button>
      )}
    </div>
  );
}

export function ImportResult() {
  const written = useImport((s) => s.written)!;
  const preview = useImport((s) => s.preview);
  const req = useImport((s) => s.req);
  const suggestions = useImport((s) => s.suggestions);
  const c = written.counts;
  const files = written.files ?? [];
  const kept = written.kept ?? [];
  const secrets = written.secrets ?? [];
  const open = () => {
    const first = files.find((f) => /\.(hurl|sonde)$/.test(f));
    if (first) void useTabs.getState().open(first);
    useUI.getState().setPanel("files");
    useImport.getState().close();
  };
  const openEnvs = () => {
    useUI.getState().setPanel("env");
    useImport.getState().close();
  };
  const changes = suggestions.reduce((n, s) => n + (s.changes?.filter((x) => !x.error).length ?? 0), 0);
  const review = changes > 0 ? { label: "Review suggestions", run: () => useImport.getState().review() } : undefined;
  const tiles: [number, string][] = [
    [c.requests, `request${c.requests === 1 ? "" : "s"} → ${c.files} file${c.files === 1 ? "" : "s"}`],
    [c.environments, `environment${c.environments === 1 ? "" : "s"}`],
    [c.statusChecks, `status checks → HTTP lines · ${c.pathVariables} path variable${c.pathVariables === 1 ? "" : "s"} → {{name}}`],
    [c.secretStubs, "secret stubs to fill in"],
  ];
  const name = preview?.name;
  const layout = req.kind === "postman" ? (req.group === "folder" ? "One file per folder" : "One file per request") : "";
  const into = req.folder ? `${req.folder.replace(/\/$/, "")}/` : "the project";
  const applyKeys = label("$mod+Enter");
  return (
    <div
      className="import-result"
      onKeyDown={(e) => {
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault();
          open();
        }
      }}
    >
      <div className="import-head">
        <Dialog.Title className="dialog-title">
          {name ? `Imported \u201c${name}\u201d from ${kindTitle[req.kind]}` : `Imported from ${kindTitle[req.kind] ?? req.kind}`}
        </Dialog.Title>
        <p>
          {layout ? `${layout} → ${into}` : `${files.length} file${files.length === 1 ? "" : "s"} written into ${into}`}
          {kept.length > 0 && `, ${kept.length} existing kept`}
          {secrets.length > 0 && ` · ${secrets.join(", ")} in the secrets file`}. Nothing was sent.
        </p>
      </div>
      <div className="import-tiles" aria-label="Import counts">
        {tiles.map(([n, label]) => (
          <div key={label} className="tile">
            <b>{n}</b>
            <span>{label}</span>
          </div>
        ))}
      </div>
      <div className="warn-cards">
        {c.scripts > 0 && (
          <WarnCard
            title={`${c.scripts} script${c.scripts === 1 ? "" : "s"} kept as # comments`}
            sub="Above each request. Sonde does not run scripts; asserts and captures can be suggested."
            action={review}
          />
        )}
        {grouped(preview?.warnings ?? [])
          .filter(([k]) => k !== "script")
          .map(([k, n, first]) => (
            <WarnCard key={k} title={`${notes[k] ?? k} (${n})`} sub={first} />
          ))}
        {c.secretStubs > 0 && (
          <WarnCard
            title={`${c.secretStubs} secret${c.secretStubs === 1 ? "" : "s"} need${c.secretStubs === 1 ? "s" : ""} values`}
            sub="Name-only stubs in the secrets files; the values are empty. Fill them in Environments."
            action={{ label: "Open Environments", run: openEnvs }}
          />
        )}
      </div>
      {(written.wrote?.length ?? 0) > 0 && (
        <section className="import-wrote" aria-label="What the importer wrote">
          <div className="wrote-head">What the importer wrote</div>
          {written.wrote!.map((w, i) => (
            <div key={i} className="wrote-row">
              <span className="mono from">{w.from}</span>
              <span className="arrow">→</span>
              <WroteTo w={w} />
              <span className={`kind ${w.kind}`}>{w.kind}</span>
            </div>
          ))}
        </section>
      )}
      <div className="dialog-actions">
        <span className="foot-note">{changes > 0 && "Suggestions for asserts, captures and a login request are optional, shown change by change."}</span>
        {changes > 0 && (
          <button className="btn" onClick={() => useImport.getState().review()}>
            Review {changes} suggestion{changes === 1 ? "" : "s"}
          </button>
        )}
        <button className="btn-primary" onClick={open}>
          Open the files{applyKeys && <kbd aria-hidden>{applyKeys}</kbd>}
        </button>
      </div>
    </div>
  );
}
