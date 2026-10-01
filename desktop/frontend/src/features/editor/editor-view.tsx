// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The Text view: shows the tab's editor (views.ts keeps one per tab); an
// empty file says how to start (type, paste a curl command, import).

import { useEffect, useRef } from "react";
import { registry } from "../../app/registry";
import { label } from "../../app/keymap/keymap-manager";
import { useKeyLabel } from "../../app/keymap/use-keys";
import { useTabs } from "../../state/tabs";
import { pasteAsCurl } from "./paste-curl";
import { openView } from "./views";

export function TextEditor({ file }: { file: string }) {
  const host = useRef<HTMLDivElement>(null);
  const empty = useTabs((s) => (s.tabs.find((t) => t.path === file)?.text ?? "x").trim() === "");
  useEffect(() => {
    const view = openView(file);
    host.current?.replaceChildren(view.dom);
    view.requestMeasure();
    view.focus();
  }, [file]);
  return (
    <>
      <div className="text-editor" ref={host} />
      {empty && <EmptyFile />}
    </>
  );
}

/** What to do with an empty file: type a request, paste a curl command
 * (the Import dialog converts it), or import another client's files. */
function EmptyFile() {
  const importKeys = useKeyLabel("import.open");
  const pasteCurl = async () => {
    const text = await navigator.clipboard?.readText?.().catch(() => "");
    await pasteAsCurl(text ?? "");
  };
  return (
    <div className="editor-empty" role="note" aria-label="Empty file">
      <b>No requests yet</b>
      <p>Type a method and URL, paste a curl command, or import from another client. Sonde writes plain .hurl you can commit.</p>
      <div className="row-gap">
        <button className="btn primary-soft" onClick={() => void pasteCurl()}>
          Paste curl<kbd>{label("$mod+KeyV")}</kbd>
        </button>
        <button className="btn" onClick={() => void registry.getCommand("import.open")?.run()}>
          Import…{importKeys && <kbd>{importKeys}</kbd>}
        </button>
      </div>
      <pre className="editor-empty-sample mono" aria-label="Example">
        <span className="m">GET</span> {"{{base_url}}"}/health{"\n"}
        <span className="h">HTTP</span> 200
      </pre>
    </div>
  );
}

