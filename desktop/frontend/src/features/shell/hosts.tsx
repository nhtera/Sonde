// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The editor and results hosts: they render what features registered, and
// a plain fallback until then.

import { useMemo } from "react";
import { registry, useRegistry } from "../../app/registry";
import { RunSummary } from "../../components/run/run-summary";
import { firstChangedLine, StaleBanner } from "../../components/run/stale-banner";
import { useRuns } from "../../state/run";
import { useTabs } from "../../state/tabs";
import { useUI } from "../../state/ui";
import { useWorkspace } from "../../state/workspace";
import { FileEmpty } from "./empty-states";

export function EditorHost({ file }: { file: string }) {
  useRegistry();
  const view = useUI((s) => s.editorView);
  const text = useTabs((s) => s.tabs.find((t) => t.path === file)?.text ?? "");
  const editors = registry.editors();
  const editor = editors.find((e) => e.id === view) ?? editors[0];
  if (editor) return <editor.render file={file} />;
  return (
    <pre className="editor-fallback mono" aria-label="File text">
      {text}
    </pre>
  );
}

export function ResultsHost({ file }: { file: string }) {
  useRegistry();
  const results = registry.getResults();
  const index = useWorkspace((s) => s.index);
  const run = useRuns((s) => s.runs[file]);
  // The edited line only (a number): the results do not re-render on
  // every keystroke.
  const changed = useTabs((s) => {
    const text = s.tabs.find((t) => t.path === file)?.text;
    return run && !run.running && text !== undefined ? firstChangedLine(run.source, text) : 0;
  });
  const requests = useMemo(() => index.filter((r) => r.file === file), [index, file]);
  if (results) return <results.render file={file} />;
  if (!run) return <FileEmpty file={file} hasRequests={requests.length > 0} />;
  return (
    <div className="results-body">
      <RunSummary
        file={file}
        run={run}
        requests={requests}
        notice={changed > 0 && <StaleBanner line={changed} onRun={() => void useRuns.getState().run(file)} />}
      />
    </div>
  );
}
