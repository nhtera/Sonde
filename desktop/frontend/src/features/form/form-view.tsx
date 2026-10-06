// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Form view (⌘E): the same file as Text, one request at a time. It
// shares the tab's editor (one text, one undo history), so switching views
// is instant; every change is an edit Go makes on that text.

import { useEffect } from "react";
import { useUI } from "../../state/ui";
import { openView } from "../editor/views";
import { formEdit } from "./edit";
import { GrpcForm, GrpcPickers } from "./grpc/grpc-form";
import { authOf, bodyKindOf, countOf, shownEntry, useFileModel, useForm, type EntryModel } from "./model";
import { RequestStrip } from "./request-strip";
import { AssertsTab } from "./tabs/asserts";
import { AuthTab } from "./tabs/auth";
import { BodyTab } from "./tabs/body/body-tab";
import { CapturesTab } from "./tabs/captures";
import { HeadersTab } from "./tabs/headers";
import { OptionsTab } from "./tabs/options";
import { ParamsTab } from "./tabs/params";
import { UrlBar } from "./url-bar";
import { WriteBackPreview, type WriteBackFocus } from "./write-back-preview";
import { useTabs } from "../../state/tabs";
import "./form.css";

const authLabel = { none: "", bearer: "Bearer", basic: "Basic", apikey: "API key", cert: "Cert" };
const bodyLabel = { none: "", "form-data": "form-data", urlencoded: "urlencoded", json: "JSON", xml: "XML", text: "Text", binary: "binary", graphql: "GraphQL", other: "block" };

/** What the request holds that the form has no tab for (edited in Text). */
function gaps(e: EntryModel): string[] {
  const out: string[] = [];
  const cookies = countOf(e, "cookies");
  if (cookies) out.push(`[Cookies] ${cookies} row${cookies === 1 ? "" : "s"}`);
  const headers = countOf(e, "response-headers");
  if (headers) out.push(`${headers} expected response header${headers === 1 ? "" : "s"}`);
  return out;
}

/** The tabs of an entry, with their counts or kind. */
function tabsOf(e: EntryModel, grpc: boolean): { id: string; title: string; note?: string }[] {
  const n = (x: number) => (x ? String(x) : undefined);
  if (grpc) {
    return [
      { id: "message", title: "Message" },
      { id: "headers", title: "Metadata", note: n(countOf(e, "headers")) },
      { id: "captures", title: "Captures", note: n(countOf(e, "captures")) },
      { id: "asserts", title: "Asserts", note: n(countOf(e, "asserts")) },
      { id: "options", title: "Options", note: n(countOf(e, "options")) },
    ];
  }
  return [
    { id: "params", title: "Params", note: n(countOf(e, "query") + (e.URLQuery?.length ?? 0)) },
    { id: "auth", title: "Auth", note: authLabel[authOf(e).kind] || undefined },
    { id: "headers", title: "Headers", note: n(countOf(e, "headers")) },
    { id: "body", title: "Body", note: bodyLabel[bodyKindOf(e)] || undefined },
    { id: "captures", title: "Captures", note: n(countOf(e, "captures")) },
    { id: "asserts", title: "Asserts", note: n(countOf(e, "asserts")) },
    { id: "options", title: "Options", note: n(countOf(e, "options")) },
  ];
}

// The marks are kept here: a new function would remake the preview.
const marks = {
  query: (l: string) => /^\[(Query|QueryStringParams)\]\s*$/.test(l),
  fileRow: (l: string) => /^[^#\s][^:]*:\s*file,/.test(l),
  form: (l: string) => /^\[(Form|FormParams)\]\s*$/.test(l),
  grpc: (l: string) => /^\[SondeGrpc\]\s*$/.test(l),
};

/** What the write-back preview shows under a tab; null: none (Auth
 * shows its own lines; Headers and Options show theirs in the rows). */
export function writeBackFocus(tab: string, entry: EntryModel, file: string): WriteBackFocus | null {
  const request = `Writes to request ${entry.Index}`;
  switch (tab) {
    case "params":
      return { title: request, sub: "the URL stays readable; params live in [Query]", part: "request", mark: marks.query };
    case "auth":
    case "headers":
    case "options":
      return null;
    case "body":
      switch (bodyKindOf(entry)) {
        case "form-data":
          return { sub: "unchecked rows become # comments", part: "request", mark: marks.fileRow };
        case "urlencoded":
          return { sub: "unchecked rows become # comments", part: "request", mark: marks.form };
        case "graphql":
          return null;
        default:
          return { part: "request" };
      }
    case "message":
      return { title: `Writes to ${file}`, sub: "method is always POST", mark: marks.grpc };
    case "captures":
    case "asserts":
      return { part: "response" };
  }
  return {};
}

export function FormView({ file }: { file: string }) {
  // The Text editor holds the tab's text and undo history, shown or not.
  useEffect(() => void openView(file), [file]);
  const model = useFileModel(file);
  const picked = useForm((s) => s.entry[file]);
  const tab = useForm((s) => s.tab);
  const cursor = useUI((s) => s.cursor);
  const text = useTabs((s) => s.tabs.find((t) => t.path === file)?.text ?? "");

  if (!model) return <div className="form-view"><p className="form-note">Reading the file…</p></div>;
  const entries = model.entries;
  if (entries.length === 0) {
    return (
      <div className="form-view">
        {model.invalid ? (
          <p className="form-invalid">The file does not read: {model.invalid}. Fix it in Text (⌘E).</p>
        ) : (
          <div className="form-empty">
            <p>No requests yet.</p>
            <button className="btn" onClick={() => void formEdit(file, { kind: "addEntry", key: "GET", value: "{{base_url}}/" })}>
              + Request
            </button>
          </div>
        )}
      </div>
    );
  }
  const entry = shownEntry(model, picked, text, cursor?.line)!;
  const grpc = file.endsWith(".sonde") && !!entry.Rows?.grpc;
  const tabs = tabsOf(entry, grpc);
  const current = tabs.some((t) => t.id === tab) ? tab : tabs[0].id;
  const focus = writeBackFocus(current, entry, file);
  let content;
  switch (current) {
    case "message":
      content = <GrpcForm file={file} entry={entry} />;
      break;
    case "params":
      content = <ParamsTab file={file} entry={entry} />;
      break;
    case "auth":
      content = <AuthTab file={file} entry={entry} />;
      break;
    case "headers":
      content = <HeadersTab file={file} entry={entry} />;
      break;
    case "body":
      content = <BodyTab file={file} entry={entry} />;
      break;
    case "captures":
      content = <CapturesTab file={file} entry={entry} />;
      break;
    case "asserts":
      content = <AssertsTab file={file} entry={entry} />;
      break;
    case "options":
      // Its More options open state belongs to the request.
      content = <OptionsTab key={entry.Index} file={file} entry={entry} />;
      break;
  }
  return (
    <div className="form-view" aria-label={`${file} form`}>
      <RequestStrip file={file} entries={entries} current={entry.Index} />
      {model.invalid && <p className="form-invalid">The text does not read right now ({model.invalid}): the form shows it as it last read.</p>}
      <UrlBar file={file} entry={entry} grpcHint={grpc} />
      {grpc && <GrpcPickers file={file} entry={entry} />}
      <div className="form-tabs" role="tablist" aria-label="Request parts">
        {tabs.map((t) => (
          <button key={t.id} role="tab" aria-selected={t.id === current} onClick={() => useForm.getState().setTab(t.id)}>
            {t.title}
            {t.note && <span className="n">{t.note}</span>}
          </button>
        ))}
      </div>
      {gaps(entry).length > 0 && <p className="form-note">Also in this request, edited in Text (⌘E): {gaps(entry).join(" · ")}.</p>}
      <div role="tabpanel" aria-label={tabs.find((t) => t.id === current)?.title}>
        {content}
      </div>
      {focus && <WriteBackPreview file={file} entry={entry} count={entries.length} version={model.version} focus={focus} />}
    </div>
  );
}
